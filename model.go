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
	"github.com/muesli/reflow/truncate"
	"github.com/sahilm/fuzzy"
)

type pane int

const (
	treePane pane = iota
	previewPane
)

// ContextGroup represents a named group of files
type ContextGroup struct {
	Name        string
	Description string
	Files       []string // Relative paths
}

type model struct {
	rootPath       string
	entries        []entry
	cursor         int
	activePane     pane
	preview        viewport.Model
	previewContent string
	previewPath    string
	loading        bool
	width          int
	height         int
	ready          bool
	lastClickTime  time.Time
	lastClickIndex int

	// Fuzzy finder
	searching     bool
	searchInput   textinput.Model
	searchResults []searchResult
	searchCursor  int
	allFiles      []string // Flat list of all file paths for searching

	// Context groups
	contextGroups []ContextGroup
	fileToGroups  map[string][]string // Maps file path to group names
	showingGroups bool
	groupCursor   int
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
	groups, fileToGroups := loadContextGroups(absPath)

	return model{
		rootPath:      absPath,
		entries:       entries,
		cursor:        0,
		activePane:    treePane,
		searchInput:   ti,
		allFiles:      allFiles,
		contextGroups: groups,
		fileToGroups:  fileToGroups,
	}
}

// loadContextGroups parses .context-groups.md and returns groups and file mapping
func loadContextGroups(rootPath string) ([]ContextGroup, map[string][]string) {
	groups := []ContextGroup{}
	fileToGroups := make(map[string][]string)

	groupsFile := filepath.Join(rootPath, ".context-groups.md")
	content, err := os.ReadFile(groupsFile)
	if err != nil {
		return groups, fileToGroups
	}

	lines := strings.Split(string(content), "\n")
	var currentGroup *ContextGroup
	var descLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

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

		// Description text (non-empty, non-heading, non-file lines)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "---") {
			descLines = append(descLines, trimmed)
		}
	}

	// Save last group
	if currentGroup != nil {
		currentGroup.Description = strings.TrimSpace(strings.Join(descLines, " "))
		groups = append(groups, *currentGroup)
	}

	return groups, fileToGroups
}

// Recursively collect all file paths
func collectAllFiles(root string) []string {
	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden files/dirs and common ignores
		name := info.Name()
		if strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
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
		// Skip hidden files and common ignores
		if strings.HasPrefix(f.Name(), ".") {
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
	return nil
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
	case fileLoadedMsg:
		// Only update if this is still the file we're waiting for
		if msg.path == m.previewPath {
			m.loading = false
			m.preview.SetContent(msg.content)
			m.preview.GotoTop()
		}
		return m, nil

	case tea.MouseMsg:
		// Auto-switch pane based on mouse position
		if msg.X < m.width/2 {
			m.activePane = treePane
		} else {
			m.activePane = previewPane
		}

		if msg.Button == tea.MouseButtonWheelUp {
			if m.activePane == treePane {
				if m.cursor > 0 {
					m.cursor--
					var cmd tea.Cmd
					m, cmd = m.updatePreview()
					cmds = append(cmds, cmd)
				}
			} else {
				m.preview.LineUp(3)
			}
		} else if msg.Button == tea.MouseButtonWheelDown {
			if m.activePane == treePane {
				if m.cursor < len(m.flatEntries())-1 {
					m.cursor++
					var cmd tea.Cmd
					m, cmd = m.updatePreview()
					cmds = append(cmds, cmd)
				}
			} else {
				m.preview.LineDown(3)
			}
		} else if msg.Button == tea.MouseButtonLeft && m.activePane == treePane {
			// Click in tree pane - calculate which entry was clicked
			// Account for header (1 line) + border (1 line)
			headerOffset := 2
			clickedIndex := msg.Y - headerOffset

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
					}
					m.lastClickTime = time.Time{} // Reset to prevent triple-click
				} else {
					// Single click: move cursor and update preview
					m.cursor = clickedIndex
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
				if m.cursor < len(m.flatEntries())-1 {
					m.cursor++
					var cmd tea.Cmd
					m, cmd = m.updatePreview()
					cmds = append(cmds, cmd)
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
					var cmd tea.Cmd
					m, cmd = m.updatePreview()
					cmds = append(cmds, cmd)
				}
			} else {
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				cmds = append(cmds, cmd)
			}

		case "enter", "l", "right":
			if m.activePane == treePane {
				flat := m.flatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.isDir {
						m = m.toggleExpand(e.path)
					}
				}
			}

		case "h", "left":
			if m.activePane == treePane {
				flat := m.flatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.isDir {
						m = m.collapse(e.path)
					}
				}
			}

		case "c":
			// Copy current file to clipboard
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

		// Match pane calculations from View()
		paneWidth := (m.width / 2) - 4
		paneHeight := m.height - 4 // Same as paneHeight in View()
		// Viewport width should account for padding (1 on each side) and border (1 on each side)
		viewportWidth := paneWidth - 2 // subtract padding
		viewportHeight := paneHeight

		if !m.ready {
			m.preview = viewport.New(viewportWidth, viewportHeight)
			m.preview.SetContent("Select a file to preview")
			m.ready = true
		} else {
			m.preview.Width = viewportWidth
			m.preview.Height = viewportHeight
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) flatEntries() []entry {
	return flattenEntries(m.entries)
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
			return truncateLines(code, maxWidth)
		}
	}

	var buf bytes.Buffer

	// Use filename to detect language, "terminal256" formatter for terminal colors
	err := quick.Highlight(&buf, code, filename, "terminal256", "monokai")
	if err != nil {
		// Fall back to plain text if highlighting fails
		return truncateLines(code, maxWidth)
	}

	// Truncate highlighted output too
	return truncateLines(buf.String(), maxWidth)
}

// truncateLines ensures no line exceeds maxWidth visually to prevent wrapping
// Uses ANSI-aware truncation to handle syntax highlighting escape codes
func truncateLines(content string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	// Leave buffer for padding and border
	maxWidth = maxWidth - 4

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// Use ANSI-aware truncation from muesli/reflow
		lines[i] = truncate.StringWithTail(line, uint(maxWidth), "…")
	}
	return strings.Join(lines, "\n")
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
			if m.groupCursor > 0 {
				m.groupCursor--
			}
			return m, nil

		case "down", "j":
			if m.groupCursor < len(m.contextGroups)-1 {
				m.groupCursor++
			}
			return m, nil

		case "enter", "c":
			// Copy the selected group to clipboard
			if m.groupCursor < len(m.contextGroups) {
				group := m.contextGroups[m.groupCursor]
				copyGroupToClipboard(m.rootPath, group)
				m.showingGroups = false
			}
			return m, nil
		}
	}
	return m, nil
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

	// Tree pane
	paneWidth := (m.width / 2) - 4
	paneHeight := m.height - 4 // header(1) + footer(1) + borders(2)

	treeStyle := lipgloss.NewStyle().
		Width(paneWidth).
		Height(paneHeight).
		Padding(0, 1)

	if m.activePane == treePane {
		treeStyle = treeStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	} else {
		treeStyle = treeStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
	}

	treeContent := m.renderTree()
	tree := treeStyle.Render(treeContent)

	// Preview pane - same height, no MaxHeight
	previewStyle := lipgloss.NewStyle().
		Width(paneWidth).
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
	footer := footerStyle.Render(" [tab] switch  [j/k] nav  [c] copy  [/] search  [g] groups  [q] quit")

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
	content.WriteString("\n\n")

	if len(m.contextGroups) == 0 {
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("No groups defined"))
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("Add groups in .context-groups.md"))
	} else {
		for i, group := range m.contextGroups {
			line := fmt.Sprintf("%s (%d files)", group.Name, len(group.Files))

			if i == m.groupCursor {
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("205")).
					Foreground(lipgloss.Color("0")).
					Render(line)
			} else {
				line = lipgloss.NewStyle().Faint(true).Render(line)
			}
			content.WriteString(line + "\n")
		}

		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Faint(true).Render("[enter/c] copy group  [esc] close"))
	}

	// Style the box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(50)

	groupsBox := boxStyle.Render(content.String())

	// Center the box
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		groupsBox,
	)
}
