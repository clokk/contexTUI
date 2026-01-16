package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/clipboard"
	"github.com/connorleisz/contexTUI/internal/config"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/sahilm/fuzzy"
)

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle filesystem events first (before mode checks) so context docs auto-reload
	// FsEventMsg just schedules a debounced reload (100ms delay)
	if _, ok := msg.(FsEventMsg); ok {
		return m, tea.Batch(
			ScheduleFsReload(100*time.Millisecond),
			m.waitForFsEvent(),
		)
	}

	// DebouncedFsEventMsg triggers the actual async reload
	if _, ok := msg.(DebouncedFsEventMsg); ok {
		m.loadingMessage = "Refreshing..."
		m.pendingLoads = 3 // directory, allFiles, registry
		cmds := []tea.Cmd{
			m.loadDirectoryAsync(),
			m.loadAllFilesAsync(),
			m.loadRegistryAsync(),
			SpinnerTick(),
		}
		if m.isGitRepo {
			m.pendingLoads = 4 // +git status
			cmds = append(cmds, m.loadGitStatusAsync())
		}
		return m, tea.Batch(cmds...)
	}

	// Handle async directory load completion
	if msg, ok := msg.(DirectoryLoadedMsg); ok {
		m.entries = msg.Entries
		if m.ready {
			m.tree.SetContent(m.RenderTree())
		}
		m.checkLoadingComplete()
		return m, nil
	}

	// Handle async all files load completion
	if msg, ok := msg.(AllFilesLoadedMsg); ok {
		m.allFiles = msg.Files
		m.checkLoadingComplete()
		return m, nil
	}

	// Handle async registry load completion
	if msg, ok := msg.(RegistryLoadedMsg); ok {
		m.docRegistry = msg.Registry
		m.checkLoadingComplete()
		return m, nil
	}

	// Handle async git status load completion
	if msg, ok := msg.(GitStatusLoadedMsg); ok {
		m.gitStatus = msg.Status
		m.gitChanges = msg.Changes
		m.gitDirStatus = msg.DirStatus
		m.gitBranch = msg.Branch
		m.gitAhead = msg.Ahead
		m.gitBehind = msg.Behind
		m.gitHasUpstream = msg.HasUpstream
		if m.ready {
			m.tree.SetContent(m.RenderTree())
		}
		m.checkLoadingComplete()
		return m, nil
	}

	// Handle spinner animation tick
	if _, ok := msg.(SpinnerTickMsg); ok {
		if m.loadingMessage != "" {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(SpinnerChars)
			return m, SpinnerTick()
		}
		return m, nil
	}

	// Handle git fetch completion
	if fetchMsg, ok := msg.(GitFetchDoneMsg); ok {
		m.gitFetching = false
		if fetchMsg.Err == nil && m.isGitRepo {
			// Refresh git status asynchronously after fetch
			m.loadingMessage = "Updating git status..."
			m.pendingLoads = 1
			return m, tea.Batch(m.loadGitStatusAsync(), SpinnerTick())
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

	// Handle file operation completion
	if msg, ok := msg.(FileOpCompleteMsg); ok {
		m.fileOpMode = FileOpNone
		m.fileOpInput.Blur()
		m.fileOpError = ""
		m.fileOpConfirm = false
		m.fileOpScrollOffset = 0

		if msg.Success {
			opNames := map[FileOpMode]string{
				FileOpCreateFile:   "Created",
				FileOpCreateFolder: "Created folder",
				FileOpRename:       "Renamed to",
				FileOpDelete:       "Deleted",
			}
			if msg.NewPath != "" {
				m.statusMessage = opNames[msg.Op] + " " + filepath.Base(msg.NewPath)
			} else {
				m.statusMessage = opNames[msg.Op] + " " + filepath.Base(m.fileOpTargetPath)
			}
		} else {
			m.statusMessage = "Error: " + msg.Error.Error()
		}
		m.statusMessageTime = time.Now()
		return m, ClearStatusAfter(5 * time.Second)
	}

	// Handle help toggle (works from any mode)
	if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "?" {
		m.showingHelp = !m.showingHelp
		if !m.showingHelp {
			m.helpScrollOffset = 0
		}
		return m, nil
	}

	// Handle help overlay - close on q/esc, scroll with j/k
	if m.showingHelp {
		// Calculate max scroll for clamping
		helpContentLines := 21 // Number of content lines in help
		maxContentHeight := m.height - 6 - 4
		if maxContentHeight < 5 {
			maxContentHeight = 5
		}
		maxScroll := helpContentLines - maxContentHeight
		if maxScroll < 0 {
			maxScroll = 0
		}

		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "q", "esc":
				m.showingHelp = false
				m.helpScrollOffset = 0
			case "j", "down":
				m.helpScrollOffset++
				if m.helpScrollOffset > maxScroll {
					m.helpScrollOffset = maxScroll
				}
			case "k", "up":
				if m.helpScrollOffset > 0 {
					m.helpScrollOffset--
				}
			}
		}
		// Mouse wheel handling
		if mouseMsg, ok := msg.(tea.MouseMsg); ok {
			if mouseMsg.Button == tea.MouseButtonWheelUp {
				m.helpScrollOffset -= 3
				if m.helpScrollOffset < 0 {
					m.helpScrollOffset = 0
				}
			} else if mouseMsg.Button == tea.MouseButtonWheelDown {
				m.helpScrollOffset += 3
				if m.helpScrollOffset > maxScroll {
					m.helpScrollOffset = maxScroll
				}
			}
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

	// Handle file operation mode
	if m.fileOpMode != FileOpNone {
		return m.updateFileOp(msg)
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

		case "n":
			// Create new file
			if m.activePane == TreePane {
				m.fileOpMode = FileOpCreateFile
				m.fileOpInput.SetValue("")
				m.fileOpInput.Placeholder = "filename"
				m.fileOpInput.Focus()
				m.fileOpTargetPath = m.getTargetDirectory()
				m.fileOpError = ""
				m.fileOpConfirm = false
				m.fileOpScrollOffset = 0
				return m, textinput.Blink
			}

		case "N":
			// Create new folder
			if m.activePane == TreePane {
				m.fileOpMode = FileOpCreateFolder
				m.fileOpInput.SetValue("")
				m.fileOpInput.Placeholder = "folder name"
				m.fileOpInput.Focus()
				m.fileOpTargetPath = m.getTargetDirectory()
				m.fileOpError = ""
				m.fileOpConfirm = false
				m.fileOpScrollOffset = 0
				return m, textinput.Blink
			}

		case "r":
			// Rename file or folder
			if m.activePane == TreePane {
				flat := m.FlatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					m.fileOpMode = FileOpRename
					m.fileOpInput.SetValue(e.Name)
					m.fileOpInput.Placeholder = "new name"
					m.fileOpInput.Focus()
					// Select all text for easy replacement
					m.fileOpInput.CursorEnd()
					m.fileOpTargetPath = e.Path
					m.fileOpError = ""
					m.fileOpConfirm = false
					m.fileOpScrollOffset = 0
					return m, textinput.Blink
				}
			}

		case "d", "x":
			// Delete file or folder
			if m.activePane == TreePane {
				flat := m.FlatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					m.fileOpMode = FileOpDelete
					m.fileOpTargetPath = e.Path
					m.fileOpError = ""
					m.fileOpConfirm = false
					m.fileOpScrollOffset = 0
					return m, nil
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

		case ".":
			// Toggle dotfile visibility
			m.showDotfiles = !m.showDotfiles
			// Save to config
			config.Save(m.rootPath, config.Config{
				SplitRatio:   m.splitRatio,
				ShowDotfiles: m.showDotfiles,
			})
			// Trigger async reload
			m.loadingMessage = "Refreshing..."
			m.pendingLoads = 2
			cmds := []tea.Cmd{
				m.loadDirectoryAsync(),
				m.loadAllFilesAsync(),
				SpinnerTick(),
			}
			// Show status message
			if m.showDotfiles {
				m.statusMessage = "Showing dotfiles"
			} else {
				m.statusMessage = "Hiding dotfiles"
			}
			m.statusMessageTime = time.Now()
			cmds = append(cmds, ClearStatusAfter(3*time.Second))
			return m, tea.Batch(cmds...)

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
			m.searchScrollOffset = 0
			m.lastSearchQuery = ""
			return m, nil

		case "enter":
			// Select the current result
			if len(m.searchResults) > 0 && m.searchCursor < len(m.searchResults) {
				result := m.searchResults[m.searchCursor]
				m.searching = false
				m.searchInput.Blur()
				m.searchScrollOffset = 0
				m.lastSearchQuery = ""
				// Navigate to the file
				m = m.NavigateToFile(result.Path)
				var cmd tea.Cmd
				m, cmd = m.UpdatePreview()
				return m, cmd
			}
			m.searching = false
			m.searchScrollOffset = 0
			m.lastSearchQuery = ""
			return m, nil

		case "up", "ctrl+p":
			if m.searchCursor > 0 {
				m.searchCursor--
				m.ensureSearchCursorVisible()
			}
			return m, nil

		case "down", "ctrl+n":
			if m.searchCursor < len(m.searchResults)-1 {
				m.searchCursor++
				m.ensureSearchCursorVisible()
			}
			return m, nil
		}

	case tea.MouseMsg:
		// Mouse wheel scrolling
		maxVisible := m.getSearchMaxVisibleResults()
		maxScroll := len(m.searchResults) - maxVisible
		if maxScroll < 0 {
			maxScroll = 0
		}

		if msg.Button == tea.MouseButtonWheelUp {
			m.searchScrollOffset -= 3
			if m.searchScrollOffset < 0 {
				m.searchScrollOffset = 0
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.searchScrollOffset += 3
			if m.searchScrollOffset > maxScroll {
				m.searchScrollOffset = maxScroll
			}
			return m, nil
		}
	}

	// Update the text input
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	cmds = append(cmds, cmd)

	// Update search results only when query changes
	query := m.searchInput.Value()
	if query != m.lastSearchQuery {
		m.lastSearchQuery = query
		if query == "" {
			m.searchResults = nil
			m.searchScrollOffset = 0
		} else {
			matches := fuzzy.Find(query, m.allFiles)
			m.searchResults = make([]SearchResult, 0, len(matches))
			for _, match := range matches {
				m.searchResults = append(m.searchResults, SearchResult{
					Path:        m.allFiles[match.Index],
					DisplayName: m.allFiles[match.Index],
				})
			}
			// Reset cursor and scroll when query changes
			m.searchCursor = 0
			m.searchScrollOffset = 0
		}
	}

	return m, tea.Batch(cmds...)
}

// getSearchMaxVisibleResults calculates max visible results based on viewport
func (m Model) getSearchMaxVisibleResults() int {
	fixedHeight := m.height - 6
	if fixedHeight < 10 {
		fixedHeight = 10
	}
	if fixedHeight > 25 {
		fixedHeight = 25
	}
	maxVisible := fixedHeight - 7
	if maxVisible < 3 {
		maxVisible = 3
	}
	return maxVisible
}

// ensureSearchCursorVisible adjusts scroll offset to keep cursor visible
func (m *Model) ensureSearchCursorVisible() {
	maxVisible := m.getSearchMaxVisibleResults()

	if m.searchCursor < m.searchScrollOffset {
		m.searchScrollOffset = m.searchCursor
	} else if m.searchCursor >= m.searchScrollOffset+maxVisible {
		m.searchScrollOffset = m.searchCursor - maxVisible + 1
	}
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
