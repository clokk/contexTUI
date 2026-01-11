package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

func initialModel(rootPath string) model {
	absPath, _ := filepath.Abs(rootPath)

	// Load user config
	cfg := loadConfig(absPath)

	// Determine split ratio (config or default)
	splitRatio := 0.5
	if cfg.SplitRatio >= 0.2 && cfg.SplitRatio <= 0.8 {
		splitRatio = cfg.SplitRatio
	}

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
		// Explicitly watch .context-groups.md for auto-reload
		contextGroupsPath := filepath.Join(absPath, ".context-groups.md")
		watcher.Add(contextGroupsPath)
	}

	return model{
		rootPath:      absPath,
		entries:       entries,
		cursor:        0,
		activePane:    treePane,
		splitRatio:    splitRatio,
		previewCache:  make(map[string]cachedPreview),
		searchInput:   ti,
		allFiles:      allFiles,
		layers:        layers,
		layerGroups:   layerGroups,
		contextGroups: groups,
		fileToGroups:  fileToGroups,
		watcher:       watcher,
	}
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

	// Handle filesystem events first (before mode checks) so context groups auto-reload
	if _, ok := msg.(fsEventMsg); ok {
		m.entries = loadDirectory(m.rootPath, 0)
		m.allFiles = collectAllFiles(m.rootPath)
		m.layers, m.layerGroups, m.contextGroups, m.fileToGroups = loadContextGroups(m.rootPath)
		if m.ready {
			m.tree.SetContent(m.renderTree())
		}
		return m, m.waitForFsEvent()
	}

	// Handle search mode separately
	if m.searching {
		return m.updateSearch(msg)
	}

	// Handle groups panel mode
	if m.showingGroups {
		return m.updateGroups(msg)
	}

	// Handle visual selection mode
	if m.selectMode {
		return m.updateSelect(msg)
	}

	switch msg := msg.(type) {
	case fileLoadedMsg:
		// Only update if this is still the file we're waiting for
		if msg.path == m.previewPath {
			m.loading = false
			m.preview.SetContent(msg.content)
			m.preview.GotoTop()
			// Store lines for copy mode selection
			m.previewLines = strings.Split(msg.content, "\n")
			// Cache the rendered content
			if !msg.modTime.IsZero() {
				m.previewCache[msg.path] = cachedPreview{
					content: msg.content,
					modTime: msg.modTime,
				}
			}
		}
		return m, nil

	case tea.MouseMsg:
		divX := m.dividerX()

		// Handle divider dragging
		if m.draggingSplit {
			if msg.Action == tea.MouseActionRelease {
				m.draggingSplit = false
				// Save config when drag ends
				saveConfig(m.rootPath, Config{SplitRatio: m.splitRatio})
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
					// Double-click: toggle directory or refresh file preview
					e := flat[clickedIndex]
					if e.isDir {
						m.cursor = clickedIndex
						m = m.toggleExpand(e.path)
						m.tree.SetContent(m.renderTree())
					} else {
						// For files, ensure preview is triggered
						m.cursor = clickedIndex
						var cmd tea.Cmd
						m, cmd = m.updatePreview()
						cmds = append(cmds, cmd)
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
					} else {
						// Trigger preview for files
						var cmd tea.Cmd
						m, cmd = m.updatePreview()
						cmds = append(cmds, cmd)
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
				saveConfig(m.rootPath, Config{SplitRatio: m.splitRatio})
			}

		case "left":
			// Resize: left arrow decreases tree pane (increases preview)
			if m.splitRatio > 0.2 {
				m.splitRatio -= 0.05
				m.tree.Width = m.leftPaneWidth() - 2
				m.preview.Width = m.rightPaneWidth() - 2
				m.tree.SetContent(m.renderTree())
				saveConfig(m.rootPath, Config{SplitRatio: m.splitRatio})
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
				m.layerCursor = 0
				m.groupsScrollOffset = 0
			}
			return m, nil

		case "v":
			// Toggle copy mode
			if !m.selectMode {
				m.selectMode = true
				m.selectStart = -1
				m.selectEnd = -1
				m.isSelecting = false
			} else {
				m.selectMode = false
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
