package app

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/clipboard"
	"github.com/connorleisz/contexTUI/internal/config"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/connorleisz/contexTUI/internal/groups"
	"github.com/sahilm/fuzzy"
)

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle filesystem events first (before mode checks) so context docs auto-reload
	if _, ok := msg.(FsEventMsg); ok {
		m.entries = LoadDirectory(m.rootPath, 0)
		m.allFiles = CollectAllFiles(m.rootPath)
		// Reload doc-based context docs
		m.docRegistry, _ = groups.LoadContextDocRegistry(m.rootPath)
		// Refresh git status
		if m.isGitRepo {
			m.gitStatus, m.gitChanges = git.LoadStatus(m.gitRepoRoot)
			m.gitDirStatus = git.ComputeDirStatus(m.gitStatus)
		}
		if m.ready {
			m.tree.SetContent(m.RenderTree())
		}
		return m, m.waitForFsEvent()
	}

	// Handle git fetch completion
	if fetchMsg, ok := msg.(GitFetchDoneMsg); ok {
		m.gitFetching = false
		if fetchMsg.Err == nil && m.isGitRepo {
			// Refresh branch info after fetch
			m.gitAhead, m.gitBehind, m.gitHasUpstream = git.GetAheadBehind(m.gitRepoRoot)
			// Also refresh git status
			m.gitStatus, m.gitChanges = git.LoadStatus(m.gitRepoRoot)
			m.gitDirStatus = git.ComputeDirStatus(m.gitStatus)
			if m.ready {
				m.tree.SetContent(m.RenderTree())
			}
		}
		return m, nil
	}

	// Handle status message clear
	if _, ok := msg.(ClearStatusMsg); ok {
		m.statusMessage = ""
		return m, nil
	}

	// Handle debounced registry save timer
	if _, ok := msg.(SaveRegistryMsg); ok {
		// Only save if still dirty and not already saving
		if m.registryDirty && !m.registrySaving {
			m.registryDirty = false // Clear before save so we detect new changes during save
			m.registrySaving = true
			return m, m.saveRegistryAsync()
		}
		return m, nil
	}

	// Handle registry save completion
	if saveMsg, ok := msg.(RegistrySavedMsg); ok {
		m.registrySaving = false
		if saveMsg.Err != nil {
			m.statusMessage = "Failed to save registry"
			m.statusMessageTime = time.Now()
		}
		// If dirty again (user moved more docs while saving), schedule another save
		if m.registryDirty {
			return m, ScheduleRegistrySave(150 * time.Millisecond)
		}
		return m, nil
	}

	// Handle help toggle (works from any mode)
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "?" {
		m.showingHelp = !m.showingHelp
		return m, nil
	}

	// Handle help overlay - just close on any key
	if m.showingHelp {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.showingHelp = false
		}
		return m, nil
	}

	// Handle search mode separately
	if m.searching {
		return m.updateSearch(msg)
	}

	// Handle docs panel mode
	if m.showingDocs {
		return m.updateDocs(msg)
	}

	// Handle visual selection mode
	if m.selectMode {
		return m.updateSelect(msg)
	}

	// Handle git status view mode
	if m.gitStatusMode {
		return m.updateGitStatus(msg)
	}

	switch msg := msg.(type) {
	case FileLoadedMsg:
		// Only update if this is still the file we're waiting for
		if msg.Path == m.previewPath {
			m.loading = false
			m.preview.SetContent(msg.Content)
			m.preview.GotoTop()
			// Store lines for copy mode selection
			m.previewLines = strings.Split(msg.Content, "\n")
			// Cache the rendered content
			if !msg.ModTime.IsZero() {
				m.previewCache[msg.Path] = CachedPreview{
					Content: msg.Content,
					ModTime: msg.ModTime,
				}
			}
		}
		return m, nil

	case tea.MouseMsg:
		divX := m.DividerX()

		// Handle divider dragging
		if m.draggingSplit {
			if msg.Action == tea.MouseActionRelease {
				m.draggingSplit = false
				// Save config when drag ends
				config.Save(m.rootPath, config.Config{SplitRatio: m.splitRatio})
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
				m.tree.Width = m.LeftPaneWidth() - 2
				m.preview.Width = m.RightPaneWidth() - 2
				m.tree.SetContent(m.RenderTree())
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
			m.activePane = TreePane
		} else {
			m.activePane = PreviewPane
		}

		if msg.Button == tea.MouseButtonWheelUp {
			if m.activePane == TreePane {
				m.tree.LineUp(3)
			} else {
				m.preview.LineUp(3)
			}
		} else if msg.Button == tea.MouseButtonWheelDown {
			if m.activePane == TreePane {
				m.tree.LineDown(3)
			} else {
				m.preview.LineDown(3)
			}
		} else if msg.Button == tea.MouseButtonLeft && m.activePane == TreePane {
			// Click in tree pane - calculate which entry was clicked
			// Account for header (1 line) + border (1 line) + viewport scroll
			headerOffset := 2
			clickedLine := msg.Y - headerOffset
			clickedIndex := clickedLine + m.tree.YOffset

			flat := m.FlatEntries()
			if clickedIndex >= 0 && clickedIndex < len(flat) {
				now := time.Now()
				isDoubleClick := clickedIndex == m.lastClickIndex &&
					now.Sub(m.lastClickTime) < 400*time.Millisecond

				if isDoubleClick {
					// Double-click: toggle directory or refresh file preview
					e := flat[clickedIndex]
					if e.IsDir {
						m.cursor = clickedIndex
						m = m.ToggleExpand(e.Path)
						m.tree.SetContent(m.RenderTree())
					} else {
						// For files, ensure preview is triggered
						m.cursor = clickedIndex
						var cmd tea.Cmd
						m, cmd = m.UpdatePreview()
						cmds = append(cmds, cmd)
					}
					m.lastClickTime = time.Time{} // Reset to prevent triple-click
				} else {
					// Single click: move cursor and update preview
					m.cursor = clickedIndex
					m.tree.SetContent(m.RenderTree())
					var cmd tea.Cmd
					m, cmd = m.UpdatePreview()
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
			if m.activePane == TreePane {
				m.activePane = PreviewPane
			} else {
				m.activePane = TreePane
			}

		case "j", "down":
			if m.activePane == TreePane {
				flat := m.FlatEntries()
				if m.cursor < len(flat)-1 {
					m.cursor++
					m.tree.SetContent(m.RenderTree())
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
			if m.activePane == TreePane {
				if m.cursor > 0 {
					m.cursor--
					m.tree.SetContent(m.RenderTree())
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
			if m.activePane == TreePane {
				flat := m.FlatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.IsDir {
						m = m.ToggleExpand(e.Path)
						m.tree.SetContent(m.RenderTree())
					} else {
						// Trigger preview for files
						var cmd tea.Cmd
						m, cmd = m.UpdatePreview()
						cmds = append(cmds, cmd)
					}
				}
			}

		case "h":
			if m.activePane == TreePane {
				flat := m.FlatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.IsDir {
						m = m.Collapse(e.Path)
						m.tree.SetContent(m.RenderTree())
					}
				}
			}

		case "right":
			// Resize: right arrow increases tree pane
			m.HandlePaneResize("right")

		case "left":
			// Resize: left arrow decreases tree pane (increases preview)
			m.HandlePaneResize("left")

		case "c":
			// Copy selected file to clipboard
			flat := m.FlatEntries()
			if m.cursor < len(flat) {
				e := flat[m.cursor]
				if !e.IsDir {
					if err := clipboard.CopyFilePath(e.Path); err != nil {
						m.statusMessage = "Clipboard unavailable"
					} else {
						m.statusMessage = "Copied!"
					}
					m.statusMessageTime = time.Now()
					return m, ClearStatusAfter(3 * time.Second)
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
			// Show docs panel
			m.showingDocs = true
			m.docCursor = 0
			m.docsScrollOffset = 0
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

		case "s":
			// Toggle git status view
			if m.isGitRepo {
				if !m.gitStatusMode {
					m.gitStatusMode = true
					m.gitStatusCursor = 0
					// Refresh git status when entering
					m.gitStatus, m.gitChanges = git.LoadStatus(m.gitRepoRoot)
					m.gitDirStatus = git.ComputeDirStatus(m.gitStatus)
					// Initialize viewport content and reset scroll
					m.gitList.SetContent(m.renderGitFileList())
					m.gitList.GotoTop()
					// Load preview for first item if there are changes
					if len(m.gitChanges) > 0 {
						var cmd tea.Cmd
						m, cmd = m.UpdateGitStatusPreview()
						return m, cmd
					}
				} else {
					m.gitStatusMode = false
				}
			}
			return m, nil

		case "f":
			// Git fetch
			if m.isGitRepo && !m.gitFetching {
				m.gitFetching = true
				repoRoot := m.gitRepoRoot
				return m, func() tea.Msg {
					err := git.Fetch(repoRoot)
					return GitFetchDoneMsg{Err: err}
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Use dynamic pane widths based on splitRatio
		paneHeight := m.height - 4
		treeWidth := m.LeftPaneWidth() - 2 // subtract padding
		previewWidth := m.RightPaneWidth() - 2

		if !m.ready {
			m.tree = viewport.New(treeWidth, paneHeight)
			m.tree.SetContent(m.RenderTree())
			m.preview = viewport.New(previewWidth, paneHeight)
			m.preview.SetContent("Select a file to preview")
			// gitList is 2 lines shorter to account for "Git Status\n\n" header
			m.gitList = viewport.New(treeWidth, paneHeight-2)
			m.ready = true
		} else {
			m.tree.Width = treeWidth
			m.tree.Height = paneHeight
			m.tree.SetContent(m.RenderTree())
			m.preview.Width = previewWidth
			m.preview.Height = paneHeight
			m.gitList.Width = treeWidth
			m.gitList.Height = paneHeight - 2
		}
	}

	return m, tea.Batch(cmds...)
}

// updateSearch handles events in search mode
func (m Model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
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
				m = m.NavigateToFile(result.Path)
				var cmd tea.Cmd
				m, cmd = m.UpdatePreview()
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
		m.searchResults = make([]SearchResult, 0, len(matches))
		for _, match := range matches {
			if len(m.searchResults) >= 10 { // Limit results
				break
			}
			m.searchResults = append(m.searchResults, SearchResult{
				Path:        m.allFiles[match.Index],
				DisplayName: m.allFiles[match.Index],
			})
		}
		// Reset cursor if it's out of bounds
		if m.searchCursor >= len(m.searchResults) {
			m.searchCursor = 0
		}
	}

	return m, tea.Batch(cmds...)
}

// updateSelect handles events in copy mode (custom selection with mouse)
func (m Model) updateSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ScrollTickMsg:
		// Continuous scroll while dragging near edge
		if m.isSelecting && m.scrollDir != 0 {
			if m.scrollDir < 0 {
				m.preview.LineUp(1)
				// Update selection to follow scroll
				m.selectEnd = m.preview.YOffset
			} else {
				m.preview.LineDown(1)
				// Update selection to follow scroll
				m.selectEnd = m.preview.YOffset + m.preview.Height - 1
			}
			// Continue ticking while still scrolling
			return m, ScrollTick()
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			// Exit copy mode
			m.selectMode = false
			m.selectStart = -1
			m.selectEnd = -1
			m.scrollDir = 0
			return m, nil

		case "v":
			// If we have a selection, copy it first then exit
			if m.selectStart >= 0 && m.selectEnd >= 0 {
				m.copySelection()
			}
			m.selectMode = false
			m.selectStart = -1
			m.selectEnd = -1
			m.scrollDir = 0
			return m, nil

		case "y", "c", "ctrl+c":
			// Copy selection (ctrl+c works in copy mode instead of quit)
			if m.selectStart >= 0 && m.selectEnd >= 0 {
				if err := m.copySelection(); err != nil {
					m.statusMessage = "Clipboard unavailable"
				} else {
					m.statusMessage = "Copied selection!"
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(3 * time.Second)
			}
			return m, nil

		// Scrolling
		case "j", "down":
			m.preview.LineDown(1)
			return m, nil
		case "k", "up":
			m.preview.LineUp(1)
			return m, nil
		case "d", "ctrl+d":
			m.preview.HalfViewDown()
			return m, nil
		case "u", "ctrl+u":
			m.preview.HalfViewUp()
			return m, nil
		case "g":
			m.preview.GotoTop()
			return m, nil
		case "G":
			m.preview.GotoBottom()
			return m, nil
		}

	case tea.MouseMsg:
		// Calculate which line was clicked (accounting for header/border)
		headerOffset := 2 // header + border
		clickedLine := msg.Y - headerOffset + m.preview.YOffset

		// Handle mouse actions
		switch msg.Action {
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft {
				// Start selection
				m.isSelecting = true
				m.selectStart = clickedLine
				m.selectEnd = clickedLine
			}

		case tea.MouseActionRelease:
			if msg.Button == tea.MouseButtonLeft {
				m.isSelecting = false
				m.scrollDir = 0 // Stop continuous scroll
			}

		case tea.MouseActionMotion:
			// Update selection while dragging
			if m.isSelecting {
				m.selectEnd = clickedLine

				// Check if near edges for continuous scroll
				visibleTop := m.preview.YOffset
				visibleBottom := m.preview.YOffset + m.preview.Height - 1
				edgeZone := 3 // Lines from edge to trigger scroll

				oldScrollDir := m.scrollDir

				if clickedLine <= visibleTop+edgeZone && visibleTop > 0 {
					m.scrollDir = -1 // Scroll up
				} else if clickedLine >= visibleBottom-edgeZone {
					m.scrollDir = 1 // Scroll down
				} else {
					m.scrollDir = 0 // Stop scrolling
				}

				// Start tick if we just entered an edge zone
				if m.scrollDir != 0 && oldScrollDir == 0 {
					return m, ScrollTick()
				}
			}
		}

		// Handle scroll wheel (works independently of selection)
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.preview.LineUp(3)
		case tea.MouseButtonWheelDown:
			m.preview.LineDown(3)
		}

		return m, nil
	}
	return m, nil
}

// copySelection copies the selected lines from preview to clipboard
func (m Model) copySelection() error {
	return clipboard.CopyLines(m.previewLines, m.selectStart, m.selectEnd, StripLineNumbers)
}
