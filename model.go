package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"
	"github.com/muesli/reflow/wordwrap"
	"github.com/sahilm/fuzzy"
)

type pane int

const (
	treePane pane = iota
	previewPane
)

// Layer represents an architectural layer in the swimlane view
type Layer struct {
	ID   string // "ui", "feature", "data", "integration", "misc"
	Name string // "UI Layer", "Feature Layer", etc.
}

// ContextGroup represents a named group of files
type ContextGroup struct {
	Name        string
	Description string
	Files       []string // Relative paths
	Layer       string   // Layer ID this group belongs to
	Parent      string   // Parent group name (for hierarchy)
	Contains    []string // Child group names (inverse of Parent)
	Tags        []string // Cross-cutting concern tags
}

type model struct {
	rootPath       string
	entries        []entry
	cursor         int
	activePane     pane
	tree           viewport.Model // Scrollable tree pane
	preview        viewport.Model
	previewContent string
	previewPath    string
	loading        bool
	width          int
	height         int
	ready          bool
	lastClickTime  time.Time
	lastClickIndex int

	// Pane resizing
	splitRatio    float64 // 0.2 to 0.8, left pane width ratio
	draggingSplit bool    // True when dragging the divider

	// Fuzzy finder
	searching     bool
	searchInput   textinput.Model
	searchResults []searchResult
	searchCursor  int
	allFiles      []string // Flat list of all file paths for searching

	// Context groups
	layers        []Layer                    // Ordered list of layers
	layerGroups   map[string][]ContextGroup  // Layer ID -> groups in that layer
	contextGroups []ContextGroup             // All groups (flat list)
	fileToGroups  map[string][]string        // Maps file path to group names
	showingGroups bool
	layerCursor   int // Which layer (row) is selected in swimlane
	groupCursor   int // Which group within layer (column) is selected

	// File watcher
	watcher *fsnotify.Watcher
}

type searchResult struct {
	path         string
	displayName  string
	matchedIndex int // Index into allFiles
}

// Message sent when file content is loaded
type fileLoadedMsg struct {
	path    string
	content string
}

// Message sent when filesystem changes
type fsEventMsg struct{}

// Message to continue watching after an event
type watchNextMsg struct{}

type entry struct {
	name     string
	path     string
	isDir    bool
	depth    int
	expanded bool
	children []entry
}

func initialModel(rootPath string) model {
	absPath, _ := filepath.Abs(rootPath)
	entries := loadDirectory(absPath, 0)

	// Set up search input
	ti := textinput.New()
	ti.Placeholder = "Search files..."
	ti.CharLimit = 100
	ti.Width = 40

	// Collect all files for searching
	allFiles := collectAllFiles(absPath)

	// Load context groups
	layers, layerGroups, groups, fileToGroups := loadContextGroups(absPath)

	// Set up file watcher
	watcher, _ := fsnotify.NewWatcher()
	if watcher != nil {
		// Watch root and all subdirectories
		filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				// Skip hidden and common ignore dirs
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
				watcher.Add(path)
			}
			return nil
		})
	}

	return model{
		rootPath:      absPath,
		entries:       entries,
		cursor:        0,
		activePane:    treePane,
		splitRatio:    0.5, // Start with equal split
		searchInput:   ti,
		allFiles:      allFiles,
		layers:        layers,
		layerGroups:   layerGroups,
		contextGroups: groups,
		fileToGroups:  fileToGroups,
		watcher:       watcher,
	}
}

// loadContextGroups parses .context-groups.md and returns layers, groups, and mappings
// If the file doesn't exist, it creates a default template
func loadContextGroups(rootPath string) ([]Layer, map[string][]ContextGroup, []ContextGroup, map[string][]string) {
	layers := []Layer{}
	layerGroups := make(map[string][]ContextGroup)
	groups := []ContextGroup{}
	fileToGroups := make(map[string][]string)

	groupsFile := filepath.Join(rootPath, ".context-groups.md")
	content, err := os.ReadFile(groupsFile)
	if err != nil {
		// Create default template if file doesn't exist
		if os.IsNotExist(err) {
			createDefaultContextGroupsFile(groupsFile)
		}
		return layers, layerGroups, groups, fileToGroups
	}

	lines := strings.Split(string(content), "\n")
	var currentGroup *ContextGroup
	var descLines []string
	parsingLayers := false
	passedSeparator := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect layers: block start
		if trimmed == "layers:" {
			parsingLayers = true
			continue
		}

		// Parse layer definitions (  - id: Name)
		if parsingLayers && strings.HasPrefix(trimmed, "- ") {
			layerDef := strings.TrimPrefix(trimmed, "- ")
			parts := strings.SplitN(layerDef, ":", 2)
			if len(parts) == 2 {
				layers = append(layers, Layer{
					ID:   strings.TrimSpace(parts[0]),
					Name: strings.TrimSpace(parts[1]),
				})
			}
			continue
		}

		// End of layers block (separator or new section)
		if parsingLayers && (trimmed == "---" || strings.HasPrefix(trimmed, "#")) {
			parsingLayers = false
			if trimmed == "---" {
				passedSeparator = true
				continue
			}
		}

		// New group heading (## name)
		if strings.HasPrefix(trimmed, "## ") {
			// Save previous group if exists
			if currentGroup != nil {
				currentGroup.Description = strings.TrimSpace(strings.Join(descLines, " "))
				groups = append(groups, *currentGroup)
			}
			// Start new group
			groupName := strings.TrimPrefix(trimmed, "## ")
			currentGroup = &ContextGroup{Name: groupName}
			descLines = nil
			continue
		}

		// Skip if no current group
		if currentGroup == nil {
			continue
		}

		// Parse metadata fields (layer:, parent:, tags:, contains:)
		if strings.HasPrefix(trimmed, "layer:") {
			currentGroup.Layer = strings.TrimSpace(strings.TrimPrefix(trimmed, "layer:"))
			continue
		}
		if strings.HasPrefix(trimmed, "parent:") {
			currentGroup.Parent = strings.TrimSpace(strings.TrimPrefix(trimmed, "parent:"))
			continue
		}
		if strings.HasPrefix(trimmed, "tags:") {
			tagsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "tags:"))
			currentGroup.Tags = parseListField(tagsStr)
			continue
		}
		if strings.HasPrefix(trimmed, "contains:") {
			containsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "contains:"))
			currentGroup.Contains = parseListField(containsStr)
			continue
		}

		// File entry (- path/to/file)
		if strings.HasPrefix(trimmed, "- ") {
			filePath := strings.TrimPrefix(trimmed, "- ")
			filePath = strings.TrimSpace(filePath)
			if filePath != "" {
				currentGroup.Files = append(currentGroup.Files, filePath)
				// Add to reverse mapping
				fileToGroups[filePath] = append(fileToGroups[filePath], currentGroup.Name)
			}
			continue
		}

		// Description text (non-empty, non-heading, non-file, non-metadata lines)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "---") {
			descLines = append(descLines, trimmed)
		}
	}

	// Save last group
	if currentGroup != nil {
		currentGroup.Description = strings.TrimSpace(strings.Join(descLines, " "))
		groups = append(groups, *currentGroup)
	}

	// Build layerGroups map, defaulting to "misc" layer
	miscLayerNeeded := false
	for _, g := range groups {
		layerID := g.Layer
		if layerID == "" {
			layerID = "misc"
			miscLayerNeeded = true
		}
		layerGroups[layerID] = append(layerGroups[layerID], g)
	}

	// Add misc layer if needed and not already defined
	if miscLayerNeeded {
		hasMisc := false
		for _, l := range layers {
			if l.ID == "misc" {
				hasMisc = true
				break
			}
		}
		if !hasMisc {
			layers = append(layers, Layer{ID: "misc", Name: "Miscellaneous"})
		}
	}

	// Ignore passedSeparator warning
	_ = passedSeparator

	return layers, layerGroups, groups, fileToGroups
}

// parseListField parses "[item1, item2]" or "item1, item2" into a slice
func parseListField(s string) []string {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	result := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// createDefaultContextGroupsFile creates a starter template for new projects
func createDefaultContextGroupsFile(path string) {
	template := `# Context Groups

Organize your codebase into logical groups for AI-assisted development.
This file is used by contexTUI to provide quick context loading for Claude and other AI tools.

## Format

` + "```markdown" + `
## group-name
layer: <layer-id>        # Which architectural layer (ui, feature, data, integration)
parent: <parent-group>   # Optional: nest under another group
tags: [tag1, tag2]       # Optional: cross-cutting concerns

Description of what this group contains and when to use it.

- path/to/file1.ts
- path/to/file2.tsx
` + "```" + `

## Usage

**In contexTUI:**
- Files show ` + "`[group-name]`" + ` badges in the tree view
- Press ` + "`g`" + ` to open the swimlane view (groups organized by layer)
- Navigate: ` + "`h/l`" + ` switch layers, ` + "`j/k`" + ` switch groups
- Press ` + "`enter`" + ` or ` + "`c`" + ` to copy all files as @filepath references

**For AI Agents:**
When working on a feature, copy the relevant context group to provide Claude with
the necessary file context. Groups are designed to be self-contained units of
related functionality.

**Git:** This file should be committed to your repository so team members share
the same context group definitions.

---

layers:
  - ui: UI Layer
  - feature: Feature Layer
  - data: Data Layer
  - integration: Integration Layer

---

## example
layer: feature
tags: [starter]

This is an example context group. Replace this with your own groups.
Add files that are commonly edited together or provide context for a feature.

- README.md
`
	os.WriteFile(path, []byte(template), 0644)
}

// Recursively collect all file paths
func collectAllFiles(root string) []string {
	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden files/dirs and common ignores (except .context-groups.md)
		name := info.Name()
		if strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			// Always show .context-groups.md - it's part of contexTUI workflow
			if name != ".context-groups.md" {
				return nil
			}
		}
		if name == "node_modules" || name == "vendor" || name == "__pycache__" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			// Store relative path for display
			relPath, _ := filepath.Rel(root, path)
			files = append(files, relPath)
		}
		return nil
	})
	return files
}

func loadDirectory(path string, depth int) []entry {
	var entries []entry

	files, err := os.ReadDir(path)
	if err != nil {
		return entries
	}

	for _, f := range files {
		// Skip hidden files and common ignores (except .context-groups.md)
		if strings.HasPrefix(f.Name(), ".") && f.Name() != ".context-groups.md" {
			continue
		}
		if f.Name() == "node_modules" || f.Name() == "vendor" || f.Name() == "__pycache__" {
			continue
		}

		e := entry{
			name:  f.Name(),
			path:  filepath.Join(path, f.Name()),
			isDir: f.IsDir(),
			depth: depth,
		}
		entries = append(entries, e)
	}

	return entries
}

func (m model) Init() tea.Cmd {
	return m.waitForFsEvent()
}

// waitForFsEvent returns a command that waits for the next filesystem event
func (m model) waitForFsEvent() tea.Cmd {
	if m.watcher == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case _, ok := <-m.watcher.Events:
			if !ok {
				return nil
			}
			// Debounce: wait a bit for rapid changes to settle
			time.Sleep(100 * time.Millisecond)
			// Drain any additional events that came in
			for {
				select {
				case <-m.watcher.Events:
				default:
					return fsEventMsg{}
				}
			}
		case <-m.watcher.Errors:
			return nil
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle search mode separately
	if m.searching {
		return m.updateSearch(msg)
	}

	// Handle groups panel mode
	if m.showingGroups {
		return m.updateGroups(msg)
	}

	switch msg := msg.(type) {
	case fsEventMsg:
		// Reload everything on filesystem change
		m.entries = loadDirectory(m.rootPath, 0)
		m.allFiles = collectAllFiles(m.rootPath)
		m.layers, m.layerGroups, m.contextGroups, m.fileToGroups = loadContextGroups(m.rootPath)
		// Refresh tree content
		if m.ready {
			m.tree.SetContent(m.renderTree())
		}
		// Continue watching for next event
		return m, m.waitForFsEvent()

	case fileLoadedMsg:
		// Only update if this is still the file we're waiting for
		if msg.path == m.previewPath {
			m.loading = false
			m.preview.SetContent(msg.content)
			m.preview.GotoTop()
		}
		return m, nil

	case tea.MouseMsg:
		divX := m.dividerX()

		// Handle divider dragging
		if m.draggingSplit {
			if msg.Action == tea.MouseActionRelease {
				m.draggingSplit = false
			} else if msg.Action == tea.MouseActionMotion {
				// Update split ratio based on mouse X position
				newRatio := float64(msg.X) / float64(m.width)
				if newRatio < 0.2 {
					newRatio = 0.2
				} else if newRatio > 0.8 {
					newRatio = 0.8
				}
				m.splitRatio = newRatio
				// Update viewport widths
				m.tree.Width = m.leftPaneWidth() - 2
				m.preview.Width = m.rightPaneWidth() - 2
				m.tree.SetContent(m.renderTree())
			}
			return m, nil
		}

		// Check if clicking on divider (within 2 pixels)
		nearDivider := msg.X >= divX-2 && msg.X <= divX+2

		if msg.Button == tea.MouseButtonLeft && nearDivider {
			m.draggingSplit = true
			return m, nil
		}

		// Auto-switch pane based on mouse position relative to divider
		if msg.X < divX {
			m.activePane = treePane
		} else {
			m.activePane = previewPane
		}

		if msg.Button == tea.MouseButtonWheelUp {
			if m.activePane == treePane {
				m.tree.LineUp(3)
			} else {
				m.preview.LineUp(3)
			}
		} else if msg.Button == tea.MouseButtonWheelDown {
			if m.activePane == treePane {
				m.tree.LineDown(3)
			} else {
				m.preview.LineDown(3)
			}
		} else if msg.Button == tea.MouseButtonLeft && m.activePane == treePane {
			// Click in tree pane - calculate which entry was clicked
			// Account for header (1 line) + border (1 line) + viewport scroll
			headerOffset := 2
			clickedLine := msg.Y - headerOffset
			clickedIndex := clickedLine + m.tree.YOffset

			flat := m.flatEntries()
			if clickedIndex >= 0 && clickedIndex < len(flat) {
				now := time.Now()
				isDoubleClick := clickedIndex == m.lastClickIndex &&
					now.Sub(m.lastClickTime) < 400*time.Millisecond

				if isDoubleClick {
					// Double-click: toggle directory or just select file
					e := flat[clickedIndex]
					if e.isDir {
						m.cursor = clickedIndex
						m = m.toggleExpand(e.path)
						m.tree.SetContent(m.renderTree())
					}
					m.lastClickTime = time.Time{} // Reset to prevent triple-click
				} else {
					// Single click: move cursor and update preview
					m.cursor = clickedIndex
					m.tree.SetContent(m.renderTree())
					var cmd tea.Cmd
					m, cmd = m.updatePreview()
					cmds = append(cmds, cmd)
				}

				m.lastClickIndex = clickedIndex
				m.lastClickTime = now
			}
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "tab":
			if m.activePane == treePane {
				m.activePane = previewPane
			} else {
				m.activePane = treePane
			}

		case "j", "down":
			if m.activePane == treePane {
				flat := m.flatEntries()
				if m.cursor < len(flat)-1 {
					m.cursor++
					m.tree.SetContent(m.renderTree())
					// Auto-scroll to keep cursor visible
					if m.cursor >= m.tree.YOffset+m.tree.Height {
						m.tree.LineDown(1)
					}
				}
			} else {
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				cmds = append(cmds, cmd)
			}

		case "k", "up":
			if m.activePane == treePane {
				if m.cursor > 0 {
					m.cursor--
					m.tree.SetContent(m.renderTree())
					// Auto-scroll to keep cursor visible
					if m.cursor < m.tree.YOffset {
						m.tree.LineUp(1)
					}
				}
			} else {
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				cmds = append(cmds, cmd)
			}

		case "enter", "l":
			if m.activePane == treePane {
				flat := m.flatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.isDir {
						m = m.toggleExpand(e.path)
						m.tree.SetContent(m.renderTree())
					}
				}
			}

		case "h":
			if m.activePane == treePane {
				flat := m.flatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.isDir {
						m = m.collapse(e.path)
						m.tree.SetContent(m.renderTree())
					}
				}
			}

		case "right":
			// Resize: right arrow increases tree pane
			if m.splitRatio < 0.8 {
				m.splitRatio += 0.05
				m.tree.Width = m.leftPaneWidth() - 2
				m.preview.Width = m.rightPaneWidth() - 2
				m.tree.SetContent(m.renderTree())
			}

		case "left":
			// Resize: left arrow decreases tree pane (increases preview)
			if m.splitRatio > 0.2 {
				m.splitRatio -= 0.05
				m.tree.Width = m.leftPaneWidth() - 2
				m.preview.Width = m.rightPaneWidth() - 2
				m.tree.SetContent(m.renderTree())
			}

		case "c":
			// Copy selected file to clipboard
			flat := m.flatEntries()
			if m.cursor < len(flat) {
				e := flat[m.cursor]
				if !e.isDir {
					copyToClipboard(e.path)
				}
			}

		case "/":
			// Enter search mode
			m.searching = true
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			m.searchResults = nil
			m.searchCursor = 0
			return m, textinput.Blink

		case "g":
			// Show groups panel
			if len(m.contextGroups) > 0 {
				m.showingGroups = true
				m.groupCursor = 0
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Use dynamic pane widths based on splitRatio
		paneHeight := m.height - 4
		treeWidth := m.leftPaneWidth() - 2   // subtract padding
		previewWidth := m.rightPaneWidth() - 2

		if !m.ready {
			m.tree = viewport.New(treeWidth, paneHeight)
			m.tree.SetContent(m.renderTree())
			m.preview = viewport.New(previewWidth, paneHeight)
			m.preview.SetContent("Select a file to preview")
			m.ready = true
		} else {
			m.tree.Width = treeWidth
			m.tree.Height = paneHeight
			m.tree.SetContent(m.renderTree())
			m.preview.Width = previewWidth
			m.preview.Height = paneHeight
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) flatEntries() []entry {
	return flattenEntries(m.entries)
}

// leftPaneWidth returns the width of the left (tree) pane
func (m model) leftPaneWidth() int {
	// Total usable width minus borders and gap
	usable := m.width - 4 // 2 for each pane's border
	return int(float64(usable) * m.splitRatio)
}

// rightPaneWidth returns the width of the right (preview) pane
func (m model) rightPaneWidth() int {
	usable := m.width - 4
	return usable - m.leftPaneWidth()
}

// dividerX returns the X position of the divider between panes
func (m model) dividerX() int {
	return m.leftPaneWidth() + 2 // +2 for left pane border
}

func flattenEntries(entries []entry) []entry {
	var flat []entry
	for _, e := range entries {
		flat = append(flat, e)
		if e.isDir && e.expanded {
			flat = append(flat, flattenEntries(e.children)...)
		}
	}
	return flat
}

func (m model) toggleExpand(path string) model {
	m.entries = toggleExpandRecursive(m.entries, path)
	return m
}

func toggleExpandRecursive(entries []entry, path string) []entry {
	for i, e := range entries {
		if e.path == path && e.isDir {
			if e.expanded {
				entries[i].expanded = false
				entries[i].children = nil
			} else {
				entries[i].expanded = true
				entries[i].children = loadDirectory(path, e.depth+1)
			}
			return entries
		}
		if e.expanded && len(e.children) > 0 {
			entries[i].children = toggleExpandRecursive(e.children, path)
		}
	}
	return entries
}

func (m model) collapse(path string) model {
	m.entries = collapseRecursive(m.entries, path)
	return m
}

func collapseRecursive(entries []entry, path string) []entry {
	for i, e := range entries {
		if e.path == path && e.isDir && e.expanded {
			entries[i].expanded = false
			entries[i].children = nil
			return entries
		}
		if e.expanded && len(e.children) > 0 {
			entries[i].children = collapseRecursive(e.children, path)
		}
	}
	return entries
}

func (m model) updatePreview() (model, tea.Cmd) {
	flat := m.flatEntries()
	if m.cursor >= len(flat) {
		return m, nil
	}

	e := flat[m.cursor]
	if e.isDir {
		m.preview.SetContent("Directory: " + e.name)
		m.loading = false
		return m, nil
	}

	// Set loading state and trigger async load
	m.loading = true
	m.previewPath = e.path
	m.preview.SetContent("Loading...")

	// Return command that loads file content
	previewWidth := m.preview.Width
	fileName := e.name
	return m, func() tea.Msg {
		content, err := os.ReadFile(e.path)
		if err != nil {
			return fileLoadedMsg{path: e.path, content: "Error: " + err.Error()}
		}

		// Render markdown files with glamour
		if strings.HasSuffix(fileName, ".md") {
			renderer, _ := glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(previewWidth),
			)
			rendered, err := renderer.Render(string(content))
			if err == nil {
				return fileLoadedMsg{path: e.path, content: rendered}
			}
		}

		// Syntax highlight code files with chroma
		highlighted := highlightCode(string(content), fileName, previewWidth)
		return fileLoadedMsg{path: e.path, content: highlighted}
	}
}

// highlightCode uses chroma to syntax highlight code based on filename
func highlightCode(code, filename string, maxWidth int) string {
	// Skip highlighting for certain file types that don't benefit from it
	skipExtensions := []string{".sum", ".lock", ".txt", ".log", ".csv", ".json"}
	for _, ext := range skipExtensions {
		if strings.HasSuffix(filename, ext) {
			return wrapLines(code, maxWidth)
		}
	}

	var buf bytes.Buffer

	// Use filename to detect language, "terminal256" formatter for terminal colors
	err := quick.Highlight(&buf, code, filename, "terminal256", "monokai")
	if err != nil {
		// Fall back to plain text if highlighting fails
		return wrapLines(code, maxWidth)
	}

	// Word wrap highlighted output
	return wrapLines(buf.String(), maxWidth)
}

// wrapLines wraps text at word boundaries to fit within maxWidth
// Uses ANSI-aware wrapping to handle syntax highlighting escape codes
func wrapLines(content string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	// Leave buffer for padding and border
	maxWidth = maxWidth - 4

	// Use ANSI-aware word wrapping from muesli/reflow
	return wordwrap.String(content, maxWidth)
}

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel search
			m.searching = false
			m.searchInput.Blur()
			return m, nil

		case "enter":
			// Select the current result
			if len(m.searchResults) > 0 && m.searchCursor < len(m.searchResults) {
				result := m.searchResults[m.searchCursor]
				m.searching = false
				m.searchInput.Blur()
				// Navigate to the file
				m = m.navigateToFile(result.path)
				var cmd tea.Cmd
				m, cmd = m.updatePreview()
				return m, cmd
			}
			m.searching = false
			return m, nil

		case "up", "ctrl+p":
			if m.searchCursor > 0 {
				m.searchCursor--
			}
			return m, nil

		case "down", "ctrl+n":
			if m.searchCursor < len(m.searchResults)-1 {
				m.searchCursor++
			}
			return m, nil
		}
	}

	// Update the text input
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	cmds = append(cmds, cmd)

	// Update search results based on current query
	query := m.searchInput.Value()
	if query == "" {
		m.searchResults = nil
	} else {
		matches := fuzzy.Find(query, m.allFiles)
		m.searchResults = make([]searchResult, 0, len(matches))
		for _, match := range matches {
			if len(m.searchResults) >= 10 { // Limit results
				break
			}
			m.searchResults = append(m.searchResults, searchResult{
				path:        m.allFiles[match.Index],
				displayName: m.allFiles[match.Index],
			})
		}
		// Reset cursor if it's out of bounds
		if m.searchCursor >= len(m.searchResults) {
			m.searchCursor = 0
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) updateGroups(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "g":
			m.showingGroups = false
			return m, nil

		case "up", "k":
			// Move up within current layer
			if m.groupCursor > 0 {
				m.groupCursor--
			}
			return m, nil

		case "down", "j":
			// Move down within current layer
			maxGroups := m.getGroupCountForCurrentLayer()
			if m.groupCursor < maxGroups-1 {
				m.groupCursor++
			}
			return m, nil

		case "left", "h":
			// Move to previous layer
			if len(m.layers) > 0 && m.layerCursor > 0 {
				m.layerCursor--
				// Reset group cursor, clamping to available groups
				maxGroups := m.getGroupCountForCurrentLayer()
				if m.groupCursor >= maxGroups {
					m.groupCursor = maxGroups - 1
				}
				if m.groupCursor < 0 {
					m.groupCursor = 0
				}
			}
			return m, nil

		case "right", "l":
			// Move to next layer
			if len(m.layers) > 0 && m.layerCursor < len(m.layers)-1 {
				m.layerCursor++
				// Reset group cursor, clamping to available groups
				maxGroups := m.getGroupCountForCurrentLayer()
				if m.groupCursor >= maxGroups {
					m.groupCursor = maxGroups - 1
				}
				if m.groupCursor < 0 {
					m.groupCursor = 0
				}
			}
			return m, nil

		case "enter", "c":
			// Copy the selected group to clipboard
			selectedGroup := m.getSelectedGroup()
			if selectedGroup != nil {
				copyGroupToClipboard(m.rootPath, *selectedGroup)
				m.showingGroups = false
			}
			return m, nil
		}
	}
	return m, nil
}

// getGroupCountForCurrentLayer returns the number of groups in the current layer
func (m model) getGroupCountForCurrentLayer() int {
	if len(m.layers) == 0 {
		return len(m.contextGroups)
	}
	if m.layerCursor >= len(m.layers) {
		return 0
	}
	layer := m.layers[m.layerCursor]
	return len(m.getOrderedGroupsForLayer(layer.ID))
}

// Navigate to a file by expanding parent directories and moving cursor
func (m model) navigateToFile(relPath string) model {
	parts := strings.Split(relPath, string(filepath.Separator))
	currentPath := m.rootPath

	// Expand each directory in the path
	for i := 0; i < len(parts)-1; i++ {
		currentPath = filepath.Join(currentPath, parts[i])
		m.entries = expandPath(m.entries, currentPath)
	}

	// Find the file in the flat list and set cursor
	fullPath := filepath.Join(m.rootPath, relPath)
	flat := m.flatEntries()
	for i, e := range flat {
		if e.path == fullPath {
			m.cursor = i
			break
		}
	}

	return m
}

func expandPath(entries []entry, path string) []entry {
	for i, e := range entries {
		if e.path == path && e.isDir && !e.expanded {
			entries[i].expanded = true
			entries[i].children = loadDirectory(path, e.depth+1)
			return entries
		}
		if e.expanded && len(e.children) > 0 {
			entries[i].children = expandPath(e.children, path)
		}
	}
	return entries
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(0, 1)

	header := headerStyle.Render("contexTUI") +
		lipgloss.NewStyle().Faint(true).Render(" " + m.rootPath)

	// Tree pane - use dynamic width from splitRatio
	leftWidth := m.leftPaneWidth()
	rightWidth := m.rightPaneWidth()
	paneHeight := m.height - 4 // header(1) + footer(1) + borders(2)

	treeStyle := lipgloss.NewStyle().
		Width(leftWidth).
		Height(paneHeight).
		Padding(0, 1)

	if m.activePane == treePane {
		treeStyle = treeStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	} else {
		treeStyle = treeStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
	}

	tree := treeStyle.Render(m.tree.View())

	// Preview pane - use dynamic width
	previewStyle := lipgloss.NewStyle().
		Width(rightWidth).
		Height(paneHeight).
		Padding(0, 1)

	if m.activePane == previewPane {
		previewStyle = previewStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	} else {
		previewStyle = previewStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
	}

	preview := previewStyle.Render(m.preview.View())

	// Footer - minimal, single line
	footerStyle := lipgloss.NewStyle().Faint(true)
	footer := footerStyle.Render(" [tab] switch  [j/k] nav  [←/→] resize  [c] copy  [/] search  [g] groups  [q] quit")

	// Compose layout
	body := lipgloss.JoinHorizontal(lipgloss.Top, tree, preview)
	mainView := header + "\n" + body + "\n" + footer

	// Overlay search if active
	if m.searching {
		return m.renderSearchOverlay(mainView)
	}

	// Overlay groups if active
	if m.showingGroups {
		return m.renderGroupsOverlay(mainView)
	}

	return mainView
}

func (m model) renderSearchOverlay(background string) string {
	// Build search box content
	var content strings.Builder
	content.WriteString(m.searchInput.View())
	content.WriteString("\n\n")

	if len(m.searchResults) == 0 && m.searchInput.Value() != "" {
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("No matches"))
	} else {
		for i, result := range m.searchResults {
			line := result.displayName
			if i == m.searchCursor {
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("205")).
					Foreground(lipgloss.Color("0")).
					Render(line)
			} else {
				line = lipgloss.NewStyle().Faint(true).Render(line)
			}
			content.WriteString(line + "\n")
		}
	}

	// Style the search box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(50)

	searchBox := boxStyle.Render(content.String())

	// Center the search box
	centeredBox := lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		searchBox,
	)

	return centeredBox
}

func (m model) renderTree() string {
	var b strings.Builder
	flat := m.flatEntries()

	badgeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Faint(true)

	for i, e := range flat {
		indent := strings.Repeat("  ", e.depth)

		icon := "  "
		if e.isDir {
			if e.expanded {
				icon = "v "
			} else {
				icon = "> "
			}
		}

		line := indent + icon + e.name

		// Add group badges for files
		if !e.isDir {
			relPath, _ := filepath.Rel(m.rootPath, e.path)
			if groups, ok := m.fileToGroups[relPath]; ok {
				for _, g := range groups {
					line += " " + badgeStyle.Render("["+g+"]")
				}
			}
		}

		if i == m.cursor {
			style := lipgloss.NewStyle().
				Background(lipgloss.Color("205")).
				Foreground(lipgloss.Color("0"))
			line = style.Render(line)
		} else if e.isDir {
			style := lipgloss.NewStyle().Bold(true)
			line = style.Render(line)
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

func copyToClipboard(path string) {
	// Use @filepath format for Claude Code context
	formatted := "@" + path

	// Use pbcopy on macOS
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(formatted)
	cmd.Run()
}

func copyGroupToClipboard(rootPath string, group ContextGroup) {
	var refs []string
	for _, relPath := range group.Files {
		fullPath := filepath.Join(rootPath, relPath)
		refs = append(refs, "@"+fullPath)
	}

	// Use pbcopy on macOS
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(strings.Join(refs, " "))
	cmd.Run()
}

func (m model) renderGroupsOverlay(background string) string {
	var content strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	content.WriteString(titleStyle.Render("Context Groups"))
	content.WriteString("\n")

	if len(m.contextGroups) == 0 {
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("No groups defined"))
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("Add groups in .context-groups.md"))
	} else if len(m.layers) == 0 {
		// Fallback to simple list if no layers defined
		content.WriteString("\n")
		for i, group := range m.contextGroups {
			line := fmt.Sprintf("%s (%d files)", group.Name, len(group.Files))
			if m.layerCursor == 0 && i == m.groupCursor {
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("205")).
					Foreground(lipgloss.Color("0")).
					Render(line)
			} else {
				line = lipgloss.NewStyle().Faint(true).Render(line)
			}
			content.WriteString(line + "\n")
		}
	} else {
		// Swimlane view
		layerNameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("105"))
		separatorStyle := lipgloss.NewStyle().Faint(true)
		selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("205")).Foreground(lipgloss.Color("0"))
		normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		childStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

		// Build parent->children map for indentation
		childrenOf := make(map[string][]string)
		for _, g := range m.contextGroups {
			if g.Parent != "" {
				childrenOf[g.Parent] = append(childrenOf[g.Parent], g.Name)
			}
		}

		for layerIdx, layer := range m.layers {
			groups := m.layerGroups[layer.ID]
			if len(groups) == 0 {
				continue
			}

			content.WriteString(separatorStyle.Render("─────────────────────────────────────────────────"))
			content.WriteString("\n")
			content.WriteString(layerNameStyle.Render(layer.Name))
			content.WriteString("\n")

			// Render groups, with children indented under parents
			rendered := make(map[string]bool)
			groupIdx := 0
			for _, group := range groups {
				if rendered[group.Name] {
					continue
				}
				// Skip if this is a child (will be rendered under parent)
				if group.Parent != "" {
					continue
				}

				// Render parent group
				isSelected := layerIdx == m.layerCursor && groupIdx == m.groupCursor
				line := fmt.Sprintf("  [%s]", group.Name)
				if isSelected {
					content.WriteString(selectedStyle.Render(line))
				} else {
					content.WriteString(normalStyle.Render(line))
				}
				rendered[group.Name] = true
				groupIdx++

				// Render children indented
				for _, childName := range childrenOf[group.Name] {
					// Find child group
					for _, g := range groups {
						if g.Name == childName {
							content.WriteString("\n")
							isChildSelected := layerIdx == m.layerCursor && groupIdx == m.groupCursor
							childLine := fmt.Sprintf("    ↳ [%s]", g.Name)
							if isChildSelected {
								content.WriteString(selectedStyle.Render(childLine))
							} else {
								content.WriteString(childStyle.Render(childLine))
							}
							rendered[childName] = true
							groupIdx++
							break
						}
					}
				}
				content.WriteString("\n")
			}

			// Render any remaining groups (children without parents in this layer)
			for _, group := range groups {
				if rendered[group.Name] {
					continue
				}
				isSelected := layerIdx == m.layerCursor && groupIdx == m.groupCursor
				line := fmt.Sprintf("  [%s]", group.Name)
				if isSelected {
					content.WriteString(selectedStyle.Render(line))
				} else {
					content.WriteString(normalStyle.Render(line))
				}
				content.WriteString("\n")
				rendered[group.Name] = true
				groupIdx++
			}
		}

		// Show selected group details
		selectedGroup := m.getSelectedGroup()
		if selectedGroup != nil {
			content.WriteString(separatorStyle.Render("─────────────────────────────────────────────────"))
			content.WriteString("\n")
			detailStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
			content.WriteString(detailStyle.Render(fmt.Sprintf("%d files", len(selectedGroup.Files))))
			if len(selectedGroup.Tags) > 0 {
				content.WriteString(detailStyle.Render("  tags: " + strings.Join(selectedGroup.Tags, ", ")))
			}
			content.WriteString("\n")
			if selectedGroup.Description != "" {
				descStyle := lipgloss.NewStyle().Faint(true).Width(45)
				content.WriteString(descStyle.Render(selectedGroup.Description))
			}
		}
	}

	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Faint(true).Render("[h/l] switch layer  [j/k] switch group  [enter/c] copy  [esc] close"))

	// Style the box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(55)

	groupsBox := boxStyle.Render(content.String())

	// Center the box
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		groupsBox,
	)
}

// getSelectedGroup returns the currently selected group in swimlane view
func (m model) getSelectedGroup() *ContextGroup {
	if len(m.layers) == 0 {
		if m.groupCursor < len(m.contextGroups) {
			return &m.contextGroups[m.groupCursor]
		}
		return nil
	}

	if m.layerCursor >= len(m.layers) {
		return nil
	}

	layer := m.layers[m.layerCursor]

	// Build ordered list matching render order (parents first, then children)
	orderedGroups := m.getOrderedGroupsForLayer(layer.ID)
	if m.groupCursor < len(orderedGroups) {
		return &orderedGroups[m.groupCursor]
	}
	return nil
}

// getOrderedGroupsForLayer returns groups in render order (parents first, children indented)
func (m model) getOrderedGroupsForLayer(layerID string) []ContextGroup {
	groups := m.layerGroups[layerID]
	if len(groups) == 0 {
		return nil
	}

	// Build parent->children map
	childrenOf := make(map[string][]string)
	for _, g := range m.contextGroups {
		if g.Parent != "" {
			childrenOf[g.Parent] = append(childrenOf[g.Parent], g.Name)
		}
	}

	var ordered []ContextGroup
	rendered := make(map[string]bool)

	// First pass: parents and their children
	for _, group := range groups {
		if rendered[group.Name] || group.Parent != "" {
			continue
		}
		ordered = append(ordered, group)
		rendered[group.Name] = true

		// Add children
		for _, childName := range childrenOf[group.Name] {
			for _, g := range groups {
				if g.Name == childName {
					ordered = append(ordered, g)
					rendered[childName] = true
					break
				}
			}
		}
	}

	// Second pass: orphan children
	for _, group := range groups {
		if !rendered[group.Name] {
			ordered = append(ordered, group)
		}
	}

	return ordered
}
