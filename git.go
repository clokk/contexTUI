package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// isGitRepo checks if the path is inside a git repository
func isGitRepo(path string) (bool, string) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	return true, strings.TrimSpace(string(output))
}

// loadGitStatus runs git status and returns file statuses
func loadGitStatus(repoRoot string) (map[string]GitFileStatus, []GitFileStatus) {
	statusMap := make(map[string]GitFileStatus)
	var changes []GitFileStatus

	// Run git status --porcelain=v1 for machine-readable output
	cmd := exec.Command("git", "-C", repoRoot, "status", "--porcelain=v1")
	output, err := cmd.Output()
	if err != nil {
		return statusMap, changes
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		// Format: XY PATH or XY OLDPATH -> NEWPATH
		// X = index status, Y = working tree status
		indexStatus := line[0]
		workStatus := line[1]
		path := line[3:] // Skip "XY "

		// Handle renames: "R  oldpath -> newpath"
		oldPath := ""
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			oldPath = parts[0]
			path = parts[1]
		}

		status := GitFileStatus{
			Path:    path,
			OldPath: oldPath,
		}

		// Determine display status and staged flag
		// Priority: show staged status if staged, otherwise working tree status
		if indexStatus != ' ' && indexStatus != '?' {
			status.Staged = true
			status.Status = string(indexStatus)
		} else if workStatus != ' ' {
			status.Staged = false
			status.Status = string(workStatus)
		} else if indexStatus == '?' {
			status.Staged = false
			status.Status = "?"
		}

		if status.Status != "" {
			statusMap[path] = status
			changes = append(changes, status)
		}
	}

	return statusMap, changes
}

// computeDirStatus aggregates file statuses to parent directories
func computeDirStatus(statusMap map[string]GitFileStatus) map[string]string {
	dirStatus := make(map[string]string)

	// Priority: ! > M > A > D > R > ?
	priority := map[string]int{"!": 6, "U": 5, "M": 4, "A": 3, "D": 2, "R": 1, "?": 0}

	for path, status := range statusMap {
		dir := filepath.Dir(path)
		for dir != "." && dir != "" && dir != "/" {
			current := dirStatus[dir]
			if priority[status.Status] > priority[current] {
				dirStatus[dir] = status.Status
			}
			dir = filepath.Dir(dir)
		}
	}

	return dirStatus
}

// getGitBranch returns the current branch name
func getGitBranch(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// getAheadBehind returns commits ahead and behind upstream
// Returns (ahead, behind, hasUpstream)
func getAheadBehind(repoRoot string) (int, int, bool) {
	// Check if upstream exists
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "@{upstream}")
	if _, err := cmd.Output(); err != nil {
		return 0, 0, false // No upstream configured
	}

	// Get ahead/behind counts
	cmd = exec.Command("git", "-C", repoRoot, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, false
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return 0, 0, false
	}

	behind, _ := strconv.Atoi(parts[0])
	ahead, _ := strconv.Atoi(parts[1])
	return ahead, behind, true
}

// gitFetch runs git fetch for the current branch's upstream
func gitFetch(repoRoot string) error {
	cmd := exec.Command("git", "-C", repoRoot, "fetch")
	return cmd.Run()
}

// updateGitStatus handles input in git status view mode
// This includes all shared handlers from normal mode
func (m model) updateGitStatus(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if m.activePane == treePane {
				if m.gitStatusCursor < len(m.gitChanges)-1 {
					m.gitStatusCursor++
					return m.updateGitStatusPreview()
				}
			} else {
				m.handlePreviewScroll("down")
			}
			return m, nil

		case "k", "up":
			if m.activePane == treePane {
				if m.gitStatusCursor > 0 {
					m.gitStatusCursor--
					return m.updateGitStatusPreview()
				}
			} else {
				m.handlePreviewScroll("up")
			}
			return m, nil

		case "enter", "l":
			// Navigate to file in tree view
			if m.gitStatusCursor < len(m.gitChanges) {
				change := m.gitChanges[m.gitStatusCursor]
				m.gitStatusMode = false
				m = m.navigateToFile(change.Path)
				m.tree.SetContent(m.renderTree())
				var cmd tea.Cmd
				m, cmd = m.updatePreview()
				return m, cmd
			}

		case "tab":
			if m.activePane == treePane {
				m.activePane = previewPane
			} else {
				m.activePane = treePane
			}
			return m, nil

		// Pane resize - SHARED
		case "left":
			m.handlePaneResize("left")
			return m, nil
		case "right":
			m.handlePaneResize("right")
			return m, nil

		// Copy file path - SHARED
		case "c":
			if m.gitStatusCursor < len(m.gitChanges) {
				change := m.gitChanges[m.gitStatusCursor]
				fullPath := filepath.Join(m.gitRepoRoot, change.Path)
				copyToClipboard(fullPath)
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
			if len(m.contextGroups) > 0 {
				m.showingGroups = true
				m.groupCursor = 0
				m.layerCursor = 0
				m.groupsScrollOffset = 0
			}
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
					err := gitFetch(repoRoot)
					return gitFetchDoneMsg{err: err}
				}
			}
			return m, nil

		// Preview scrolling
		case "ctrl+d":
			m.handlePreviewScroll("half-down")
			return m, nil
		case "ctrl+u":
			m.handlePreviewScroll("half-up")
			return m, nil
		case "G":
			m.handlePreviewScroll("bottom")
			return m, nil
		}

	case tea.MouseMsg:
		divX := m.dividerX()

		// Handle divider dragging
		if m.draggingSplit {
			if msg.Action == tea.MouseActionRelease {
				m.draggingSplit = false
				saveConfig(m.rootPath, Config{SplitRatio: m.splitRatio})
			} else if msg.Action == tea.MouseActionMotion {
				newRatio := float64(msg.X) / float64(m.width)
				if newRatio < 0.2 {
					newRatio = 0.2
				} else if newRatio > 0.8 {
					newRatio = 0.8
				}
				m.splitRatio = newRatio
				m.tree.Width = m.leftPaneWidth() - 2
				m.preview.Width = m.rightPaneWidth() - 2
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
			m.activePane = treePane
		} else {
			m.activePane = previewPane
		}

		// Mouse wheel scrolling
		if msg.Button == tea.MouseButtonWheelUp {
			if m.activePane == previewPane {
				m.preview.LineUp(3)
			}
		} else if msg.Button == tea.MouseButtonWheelDown {
			if m.activePane == previewPane {
				m.preview.LineDown(3)
			}
		}

		// Mouse click on file list
		if msg.Button == tea.MouseButtonLeft && m.activePane == treePane {
			headerOffset := 2
			clickedLine := msg.Y - headerOffset
			clickedIndex := m.gitLineToIndex(clickedLine)
			if clickedIndex >= 0 && clickedIndex < len(m.gitChanges) {
				m.gitStatusCursor = clickedIndex
				return m.updateGitStatusPreview()
			}
		}

		return m, nil

	case fileLoadedMsg:
		if msg.path != "" {
			m.preview.SetContent(msg.content)
			m.previewPath = msg.path
			m.previewLines = strings.Split(msg.content, "\n")
			m.loading = false
			m.preview.GotoTop()

			if m.previewCache == nil {
				m.previewCache = make(map[string]cachedPreview)
			}
			m.previewCache[msg.path] = cachedPreview{
				content: msg.content,
				modTime: msg.modTime,
			}
		}
		return m, nil
	}
	return m, nil
}

// gitLineToIndex converts a clicked line number to an index in gitChanges
// This accounts for category headers in the rendered output
func (m model) gitLineToIndex(clickedLine int) int {
	staged, unstaged, untracked := m.categorizeGitChanges()

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

// categorizeGitChanges returns staged, unstaged, and untracked files separately
func (m model) categorizeGitChanges() (staged, unstaged, untracked []GitFileStatus) {
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

// updateGitStatusPreview loads the diff preview for the currently selected git change
func (m model) updateGitStatusPreview() (model, tea.Cmd) {
	if m.gitStatusCursor >= len(m.gitChanges) {
		return m, nil
	}

	change := m.gitChanges[m.gitStatusCursor]
	fullPath := filepath.Join(m.gitRepoRoot, change.Path)

	// Set loading state
	m.loading = true
	m.previewPath = fullPath
	m.preview.SetContent("Loading...")

	// Capture values for async command
	previewWidth := m.preview.Width
	repoRoot := m.gitRepoRoot
	staged := change.Staged
	status := change.Status
	relPath := change.Path
	fileName := filepath.Base(change.Path)

	// Return async command
	return m, func() tea.Msg {
		// Untracked files - show file content (no diff exists)
		if status == "?" {
			return loadFileContent(fullPath, fileName, previewWidth)
		}
		// All other changes - show diff
		return loadGitDiff(repoRoot, relPath, staged, previewWidth)
	}
}

// loadGitDiff runs git diff and returns the diff output for a file
func loadGitDiff(repoRoot, filePath string, staged bool, previewWidth int) fileLoadedMsg {
	var args []string
	if staged {
		args = []string{"-C", repoRoot, "diff", "--cached", "--", filePath}
	} else {
		args = []string{"-C", repoRoot, "diff", "--", filePath}
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return fileLoadedMsg{
			path:    filepath.Join(repoRoot, filePath),
			content: "No diff available",
		}
	}

	diffText := string(output)

	// Apply diff syntax highlighting
	highlighted := highlightDiff(diffText, previewWidth)

	return fileLoadedMsg{
		path:    filepath.Join(repoRoot, filePath),
		content: highlighted,
		modTime: time.Now(),
	}
}

// loadFilePreview returns a command that loads file content asynchronously
func loadFilePreview(e entry, previewWidth int) tea.Cmd {
	return func() tea.Msg {
		return loadFileContent(e.path, e.name, previewWidth)
	}
}

// renderGitStatusView renders the git status view with file list and preview
func (m model) renderGitStatusView(paneHeight int) string {
	leftWidth := m.leftPaneWidth()
	rightWidth := m.rightPaneWidth()

	// Left pane: Changed files list
	var left strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	left.WriteString(headerStyle.Render("Git Status"))
	left.WriteString("\n\n")

	if len(m.gitChanges) == 0 {
		left.WriteString(lipgloss.NewStyle().Faint(true).Render("Working tree clean"))
	} else {
		// Group by status
		var staged, unstaged, untracked []GitFileStatus

		for _, c := range m.gitChanges {
			if c.Status == "?" {
				untracked = append(untracked, c)
			} else if c.Staged {
				staged = append(staged, c)
			} else {
				unstaged = append(unstaged, c)
			}
		}

		// Style definitions
		stagedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("118")).Bold(true)
		unstagedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)
		untrackedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
		selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("205")).Foreground(lipgloss.Color("0"))

		statusStyles := map[string]lipgloss.Style{
			"M": lipgloss.NewStyle().Foreground(lipgloss.Color("226")),
			"A": lipgloss.NewStyle().Foreground(lipgloss.Color("118")),
			"D": lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
			"R": lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
			"?": lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
			"U": lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		}

		idx := 0

		// Render staged changes
		if len(staged) > 0 {
			left.WriteString(stagedStyle.Render("Staged Changes"))
			left.WriteString("\n")
			for _, c := range staged {
				var line string
				if idx == m.gitStatusCursor {
					// For selected line, don't apply status color - use uniform highlight
					line = fmt.Sprintf("  %s %s", c.Status, c.Path)
					// Pad line for full highlight
					if len(line) < leftWidth-4 {
						line = line + strings.Repeat(" ", leftWidth-4-len(line))
					}
					line = selectedStyle.Render(line)
				} else {
					statusStyle := statusStyles[c.Status]
					line = fmt.Sprintf("  %s %s", statusStyle.Render(c.Status), c.Path)
				}
				left.WriteString(line + "\n")
				idx++
			}
			left.WriteString("\n")
		}

		// Render unstaged changes
		if len(unstaged) > 0 {
			left.WriteString(unstagedStyle.Render("Changes not staged"))
			left.WriteString("\n")
			for _, c := range unstaged {
				var line string
				if idx == m.gitStatusCursor {
					// For selected line, don't apply status color - use uniform highlight
					line = fmt.Sprintf("  %s %s", c.Status, c.Path)
					if len(line) < leftWidth-4 {
						line = line + strings.Repeat(" ", leftWidth-4-len(line))
					}
					line = selectedStyle.Render(line)
				} else {
					statusStyle := statusStyles[c.Status]
					line = fmt.Sprintf("  %s %s", statusStyle.Render(c.Status), c.Path)
				}
				left.WriteString(line + "\n")
				idx++
			}
			left.WriteString("\n")
		}

		// Render untracked files
		if len(untracked) > 0 {
			left.WriteString(untrackedStyle.Render("Untracked files"))
			left.WriteString("\n")
			for _, c := range untracked {
				var line string
				if idx == m.gitStatusCursor {
					// For selected line, don't apply status color - use uniform highlight
					line = fmt.Sprintf("  %s %s", c.Status, c.Path)
					if len(line) < leftWidth-4 {
						line = line + strings.Repeat(" ", leftWidth-4-len(line))
					}
					line = selectedStyle.Render(line)
				} else {
					line = fmt.Sprintf("  %s %s", untrackedStyle.Render(c.Status), c.Path)
				}
				left.WriteString(line + "\n")
				idx++
			}
		}
	}

	// Style the left pane
	leftPaneStyle := lipgloss.NewStyle().
		Width(leftWidth).
		Height(paneHeight).
		Padding(0, 1)

	if m.activePane == treePane {
		leftPaneStyle = leftPaneStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	} else {
		leftPaneStyle = leftPaneStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
	}

	// Right pane: File preview
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

	leftPane := leftPaneStyle.Render(left.String())
	rightPane := previewStyle.Render(m.preview.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}
