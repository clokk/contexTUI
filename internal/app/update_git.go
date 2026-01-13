package app

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/clipboard"
	"github.com/connorleisz/contexTUI/internal/config"
	"github.com/connorleisz/contexTUI/internal/git"
)

// updateGitStatus handles input in git status view mode
func (m Model) updateGitStatus(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		// Exit git status
		case "esc", "s":
			m.gitStatusMode = false
			return m, nil

		// Quit
		case "q", "ctrl+c":
			return m, tea.Quit

		// Navigation - behavior depends on active pane
		case "j", "down":
			if m.activePane == TreePane {
				if m.gitStatusCursor < len(m.gitChanges)-1 {
					m.gitStatusCursor++
					return m.UpdateGitStatusPreview()
				}
			} else {
				m.HandlePreviewScroll("down")
			}
			return m, nil

		case "k", "up":
			if m.activePane == TreePane {
				if m.gitStatusCursor > 0 {
					m.gitStatusCursor--
					return m.UpdateGitStatusPreview()
				}
			} else {
				m.HandlePreviewScroll("up")
			}
			return m, nil

		case "enter", "l":
			// Navigate to file in tree view
			if m.gitStatusCursor < len(m.gitChanges) {
				change := m.gitChanges[m.gitStatusCursor]
				m.gitStatusMode = false
				m = m.NavigateToFile(change.Path)
				m.tree.SetContent(m.RenderTree())
				var cmd tea.Cmd
				m, cmd = m.UpdatePreview()
				return m, cmd
			}

		case "tab":
			if m.activePane == TreePane {
				m.activePane = PreviewPane
			} else {
				m.activePane = TreePane
			}
			return m, nil

		// Pane resize - SHARED
		case "left":
			m.HandlePaneResize("left")
			return m, nil
		case "right":
			m.HandlePaneResize("right")
			return m, nil

		// Copy file path - SHARED
		case "c":
			if m.gitStatusCursor < len(m.gitChanges) {
				change := m.gitChanges[m.gitStatusCursor]
				fullPath := filepath.Join(m.gitRepoRoot, change.Path)
				if err := clipboard.CopyFilePath(fullPath); err != nil {
					m.statusMessage = "Clipboard unavailable"
				} else {
					m.statusMessage = "Copied!"
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(3 * time.Second)
			}
			return m, nil

		// Enter search mode - SHARED
		case "/":
			m.searching = true
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			m.searchResults = nil
			m.searchCursor = 0
			return m, textinput.Blink

		// Show groups overlay - SHARED
		case "g":
			m.showingGroups = true
			m.docGroupCursor = 0
			m.groupsScrollOffset = 0
			return m, nil

		// Enter copy mode - SHARED
		case "v":
			m.selectMode = true
			m.selectStart = -1
			m.selectEnd = -1
			m.isSelecting = false
			return m, nil

		// Git fetch - SHARED
		case "f":
			if m.isGitRepo && !m.gitFetching {
				m.gitFetching = true
				repoRoot := m.gitRepoRoot
				return m, func() tea.Msg {
					err := git.Fetch(repoRoot)
					return GitFetchDoneMsg{Err: err}
				}
			}
			return m, nil

		// Preview scrolling
		case "ctrl+d":
			m.HandlePreviewScroll("half-down")
			return m, nil
		case "ctrl+u":
			m.HandlePreviewScroll("half-up")
			return m, nil
		case "G":
			m.HandlePreviewScroll("bottom")
			return m, nil
		}

	case tea.MouseMsg:
		divX := m.DividerX()

		// Handle divider dragging
		if m.draggingSplit {
			if msg.Action == tea.MouseActionRelease {
				m.draggingSplit = false
				config.Save(m.rootPath, config.Config{SplitRatio: m.splitRatio})
			} else if msg.Action == tea.MouseActionMotion {
				newRatio := float64(msg.X) / float64(m.width)
				if newRatio < 0.2 {
					newRatio = 0.2
				} else if newRatio > 0.8 {
					newRatio = 0.8
				}
				m.splitRatio = newRatio
				m.tree.Width = m.LeftPaneWidth() - 2
				m.preview.Width = m.RightPaneWidth() - 2
			}
			return m, nil
		}

		// Check for divider click
		nearDivider := msg.X >= divX-2 && msg.X <= divX+2
		if msg.Button == tea.MouseButtonLeft && nearDivider {
			m.draggingSplit = true
			return m, nil
		}

		// Auto-switch pane based on mouse position
		if msg.X < divX {
			m.activePane = TreePane
		} else {
			m.activePane = PreviewPane
		}

		// Mouse wheel scrolling
		if msg.Button == tea.MouseButtonWheelUp {
			if m.activePane == PreviewPane {
				m.preview.LineUp(3)
			}
		} else if msg.Button == tea.MouseButtonWheelDown {
			if m.activePane == PreviewPane {
				m.preview.LineDown(3)
			}
		}

		// Mouse click on file list
		if msg.Button == tea.MouseButtonLeft && m.activePane == TreePane {
			headerOffset := 2
			clickedLine := msg.Y - headerOffset
			clickedIndex := m.gitLineToIndex(clickedLine)
			if clickedIndex >= 0 && clickedIndex < len(m.gitChanges) {
				m.gitStatusCursor = clickedIndex
				return m.UpdateGitStatusPreview()
			}
		}

		return m, nil

	case FileLoadedMsg:
		if msg.Path != "" {
			m.preview.SetContent(msg.Content)
			m.previewPath = msg.Path
			m.previewLines = strings.Split(msg.Content, "\n")
			m.loading = false
			m.preview.GotoTop()

			if m.previewCache == nil {
				m.previewCache = make(map[string]CachedPreview)
			}
			m.previewCache[msg.Path] = CachedPreview{
				Content: msg.Content,
				ModTime: msg.ModTime,
			}
		}
		return m, nil
	}
	return m, nil
}

// gitLineToIndex converts a clicked line number to an index in gitChanges
// This accounts for category headers in the rendered output
func (m Model) gitLineToIndex(clickedLine int) int {
	staged, unstaged, untracked := m.CategorizeGitChanges()

	currentLine := 2 // After "Git Status\n\n"
	idx := 0

	if len(staged) > 0 {
		currentLine++ // "Staged Changes" header
		for range staged {
			if clickedLine == currentLine {
				return idx
			}
			currentLine++
			idx++
		}
		currentLine++ // Blank line after section
	}

	if len(unstaged) > 0 {
		currentLine++ // "Changes not staged" header
		for range unstaged {
			if clickedLine == currentLine {
				return idx
			}
			currentLine++
			idx++
		}
		currentLine++ // Blank line
	}

	if len(untracked) > 0 {
		currentLine++ // "Untracked files" header
		for range untracked {
			if clickedLine == currentLine {
				return idx
			}
			currentLine++
			idx++
		}
	}

	return -1 // Not on a file line
}

// CategorizeGitChanges returns staged, unstaged, and untracked files separately
func (m Model) CategorizeGitChanges() (staged, unstaged, untracked []git.FileStatus) {
	for _, c := range m.gitChanges {
		if c.Status == "?" {
			untracked = append(untracked, c)
		} else if c.Staged {
			staged = append(staged, c)
		} else {
			unstaged = append(unstaged, c)
		}
	}
	return
}
