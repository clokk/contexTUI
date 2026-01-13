package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/clipboard"
	"github.com/connorleisz/contexTUI/internal/groups"
)

// StructureNeededTag is inserted into files that need context group structuring
const StructureNeededTag = "<!-- contexTUI: structure-needed -->\n"

// StructuringPrompt is copied when user presses 'p' in groups overlay
const StructuringPrompt = `Find all markdown files in this project containing the comment:
<!-- contexTUI: structure-needed -->

For each file, read .context-groups.md to understand the required structure,
then update the file to include:
- **Supergroup:** (Meta, Feature, or custom category)
- **Status:** Active
- ## Description section
- ## Key Files section

Remove the <!-- contexTUI: structure-needed --> tag after structuring.`

// updateGroups handles the context groups overlay
func (m Model) updateGroups(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle add group mode separately
	if m.addingGroup {
		return m.updateAddGroup(msg)
	}

	// Get groups for current supergroup
	currentGroups := m.getGroupsForSelectedSupergroup()
	totalGroups := len(currentGroups)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.showingGroups = false
			return m, nil

		case "left", "h":
			// Previous supergroup
			if m.docRegistry != nil && len(m.docRegistry.Supergroups) > 0 {
				m.selectedSupergroup--
				if m.selectedSupergroup < 0 {
					m.selectedSupergroup = len(m.docRegistry.Supergroups) - 1
				}
				m.docGroupCursor = 0
				m.groupsScrollOffset = 0
			}
			return m, nil

		case "right", "l":
			// Next supergroup
			if m.docRegistry != nil && len(m.docRegistry.Supergroups) > 0 {
				m.selectedSupergroup++
				if m.selectedSupergroup >= len(m.docRegistry.Supergroups) {
					m.selectedSupergroup = 0
				}
				m.docGroupCursor = 0
				m.groupsScrollOffset = 0
			}
			return m, nil

		case "up", "k":
			if m.docGroupCursor > 0 {
				m.docGroupCursor--
				m.ensureDocGroupVisible()
			}
			return m, nil

		case "down", "j":
			if m.docGroupCursor < totalGroups-1 {
				m.docGroupCursor++
				m.ensureDocGroupVisible()
			}
			return m, nil

		case "enter", "c":
			// Copy selected groups (or current if none selected) as @filepath references
			if len(m.selectedGroups) > 0 {
				// Copy all selected groups - iterate directly over selectedGroups map
				var refs []string
				for path := range m.selectedGroups {
					refs = append(refs, "@"+path)
				}
				combined := strings.Join(refs, "\n")
				if err := clipboard.CopyRaw(combined); err != nil {
					m.statusMessage = "Clipboard unavailable"
				} else {
					m.statusMessage = fmt.Sprintf("Copied %d references", len(refs))
				}
				// Clear selections after copy
				m.selectedGroups = make(map[string]bool)
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			} else if m.docGroupCursor < totalGroups {
				// Copy single current group as @filepath reference
				group := currentGroups[m.docGroupCursor]
				if err := clipboard.CopyFilePath(group.FilePath); err != nil {
					m.statusMessage = "Clipboard unavailable"
				} else {
					m.statusMessage = fmt.Sprintf("Copied: @%s", group.FilePath)
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			}
			return m, nil

		case "a":
			// Find available .md files to add
			mdFiles, _ := groups.FindMarkdownFiles(m.rootPath)
			// Filter out already-added files
			var available []string
			existingPaths := make(map[string]bool)
			if m.docRegistry != nil {
				for _, g := range m.docRegistry.Groups {
					existingPaths[g.FilePath] = true
				}
			}
			for _, f := range mdFiles {
				if !existingPaths[f] {
					available = append(available, f)
				}
			}
			if len(available) == 0 {
				m.statusMessage = "No markdown files available to add"
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			}
			m.availableMdFiles = available
			m.addGroupCursor = 0
			m.addGroupScroll = 0
			m.addingGroup = true
			return m, nil

		case "p":
			// Copy the structuring prompt to clipboard
			if err := clipboard.CopyFilePath(StructuringPrompt); err != nil {
				m.statusMessage = "Clipboard unavailable"
			} else {
				m.statusMessage = "Copied structuring prompt!"
			}
			m.statusMessageTime = time.Now()
			return m, ClearStatusAfter(5 * time.Second)

		case "d", "x":
			// Remove the selected group from registry
			if m.docGroupCursor < totalGroups && m.docRegistry != nil {
				group := currentGroups[m.docGroupCursor]

				// Remove from Groups slice
				for i, g := range m.docRegistry.Groups {
					if g.FilePath == group.FilePath {
						m.docRegistry.Groups = append(m.docRegistry.Groups[:i], m.docRegistry.Groups[i+1:]...)
						break
					}
				}

				// Remove from BySuper map
				sgID := strings.ToLower(strings.ReplaceAll(group.Supergroup, " ", "-"))
				if sgID == "" {
					sgID = "miscellaneous"
				}
				docGroups := m.docRegistry.BySuper[sgID]
				for i, g := range docGroups {
					if g.FilePath == group.FilePath {
						m.docRegistry.BySuper[sgID] = append(docGroups[:i], docGroups[i+1:]...)
						break
					}
				}

				// Adjust cursor if needed
				if m.docGroupCursor >= len(m.docRegistry.BySuper[sgID]) {
					m.docGroupCursor = len(m.docRegistry.BySuper[sgID]) - 1
				}
				if m.docGroupCursor < 0 {
					m.docGroupCursor = 0
				}

				// Strip contexTUI metadata from the markdown file
				stripContextGroupMetadata(m.rootPath, group.FilePath)

				// Save registry
				if err := groups.SaveDocGroupRegistry(m.rootPath, m.docRegistry); err != nil {
					m.statusMessage = fmt.Sprintf("Error: %v", err)
				} else {
					m.statusMessage = fmt.Sprintf("Removed %s", group.Name)
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			}
			return m, nil

		case " ":
			// Toggle selection of current group for multi-copy
			if m.docGroupCursor < totalGroups {
				group := currentGroups[m.docGroupCursor]
				if m.selectedGroups[group.FilePath] {
					delete(m.selectedGroups, group.FilePath)
					m.statusMessage = fmt.Sprintf("Deselected (%d total)", len(m.selectedGroups))
				} else {
					m.selectedGroups[group.FilePath] = true
					m.statusMessage = fmt.Sprintf("Selected (%d total)", len(m.selectedGroups))
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(2 * time.Second)
			}
			return m, nil

		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// First check if clicking on navigation bar (prev/next arrows)
			navClick := m.findClickedNav(msg.X, msg.Y)
			if navClick == navClickPrev && m.docRegistry != nil {
				// Go to previous supergroup
				m.selectedSupergroup--
				if m.selectedSupergroup < 0 {
					m.selectedSupergroup = len(m.docRegistry.Supergroups) - 1
				}
				m.docGroupCursor = 0
				m.groupsScrollOffset = 0
				return m, nil
			} else if navClick == navClickNext && m.docRegistry != nil {
				// Go to next supergroup
				m.selectedSupergroup++
				if m.selectedSupergroup >= len(m.docRegistry.Supergroups) {
					m.selectedSupergroup = 0
				}
				m.docGroupCursor = 0
				m.groupsScrollOffset = 0
				return m, nil
			}

			// Try to find which card was clicked
			clickedIdx := m.findClickedGroup(msg.X, msg.Y)
			if clickedIdx >= 0 && clickedIdx < totalGroups {
				// Move cursor to clicked item
				m.docGroupCursor = clickedIdx
				m.ensureDocGroupVisible()

				// If multi-select is active, copy all selected
				if len(m.selectedGroups) > 0 {
					var refs []string
					for path := range m.selectedGroups {
						refs = append(refs, "@"+path)
					}
					combined := strings.Join(refs, "\n")
					if err := clipboard.CopyRaw(combined); err != nil {
						m.statusMessage = "Clipboard unavailable"
					} else {
						m.statusMessage = fmt.Sprintf("Copied %d references", len(refs))
					}
					m.selectedGroups = make(map[string]bool)
				} else {
					// Copy the clicked group as @filepath reference
					group := currentGroups[clickedIdx]
					if err := clipboard.CopyFilePath(group.FilePath); err != nil {
						m.statusMessage = "Clipboard unavailable"
					} else {
						m.statusMessage = fmt.Sprintf("Copied: @%s", group.FilePath)
					}
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			}
		} else if msg.Button == tea.MouseButtonWheelUp {
			m.groupsScrollOffset--
			if m.groupsScrollOffset < 0 {
				m.groupsScrollOffset = 0
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.groupsScrollOffset++
			// Estimate max scroll based on card layout (~7 lines per card + headers)
			maxContentHeight := m.height - 8
			if maxContentHeight < 10 {
				maxContentHeight = 10
			}
			estimatedLines := m.estimateGroupsLineCount()
			maxScroll := estimatedLines - maxContentHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.groupsScrollOffset > maxScroll {
				m.groupsScrollOffset = maxScroll
			}
			return m, nil
		}
	}
	return m, nil
}

// ensureDocGroupVisible ensures the selected doc group is visible
func (m *Model) ensureDocGroupVisible() {
	if m.docRegistry == nil {
		return
	}

	maxContentHeight := m.height - 16
	if maxContentHeight < 8 {
		maxContentHeight = 8
	}

	lineIdx := m.getDocGroupLineIndex(m.docGroupCursor)
	totalLines := m.getDocGroupTotalLines()

	maxScroll := totalLines - maxContentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.groupsScrollOffset > maxScroll {
		m.groupsScrollOffset = maxScroll
	}
	if m.groupsScrollOffset < 0 {
		m.groupsScrollOffset = 0
	}

	if lineIdx < m.groupsScrollOffset {
		m.groupsScrollOffset = lineIdx
	} else if lineIdx >= m.groupsScrollOffset+maxContentHeight {
		m.groupsScrollOffset = lineIdx - maxContentHeight + 1
	}
}

// getDocGroupLineIndex returns the line index for a given group index
func (m Model) getDocGroupLineIndex(groupIdx int) int {
	if m.docRegistry == nil {
		return 0
	}

	lineIdx := 1 // Title line

	currentGroupIdx := 0
	for _, sg := range m.docRegistry.Supergroups {
		sgGroups := m.docRegistry.BySuper[sg.ID]
		if len(sgGroups) == 0 {
			continue
		}

		lineIdx += 2 // separator + supergroup name

		for range sgGroups {
			if currentGroupIdx == groupIdx {
				return lineIdx
			}
			lineIdx++
			currentGroupIdx++
		}
	}

	return lineIdx
}

// getDocGroupTotalLines returns total lines in the doc groups overlay
func (m Model) getDocGroupTotalLines() int {
	return m.estimateGroupsLineCount()
}

// estimateGroupsLineCount estimates actual rendered line count for card layout
func (m Model) estimateGroupsLineCount() int {
	docGroups := m.getGroupsForSelectedSupergroup()

	if len(docGroups) == 0 {
		return 10 // Title + tabs + empty message
	}

	lineCount := 6 // Title + blank + tabs + separator + blank

	// Each card: ~8 lines (border top/bottom + title + filepath + 3 desc + key files)
	for _, group := range docGroups {
		cardLines := 5 // borders (2) + title (1) + filepath (1) + key files (1)
		if group.Description != "" {
			// Estimate wrapped description lines (max 3)
			descLen := len(group.Description)
			descLines := (descLen / 60) + 1
			if descLines > 3 {
				descLines = 3
			}
			cardLines += descLines
		}
		lineCount += cardLines
	}

	return lineCount
}

// Navigation click constants
const (
	navClickNone = -1
	navClickPrev = -2
	navClickNext = -3
)

// findClickedNav detects clicks on the gallery navigation bar
// Returns: navClickPrev (-2) for left third, navClickNext (-3) for right third, navClickNone (-1) otherwise
func (m Model) findClickedNav(clickX, clickY int) int {
	if m.docRegistry == nil || len(m.docRegistry.Supergroups) == 0 {
		return navClickNone
	}

	// Box dimensions (must match render.go)
	cardWidth := 68
	boxWidth := cardWidth + 8
	boxLeft := (m.width - boxWidth) / 2

	// Fixed height calculation (must match render.go)
	fixedHeight := m.height - 6
	if fixedHeight < 15 {
		fixedHeight = 15
	}
	boxTop := (m.height - fixedHeight) / 2

	// Check X is within box
	if clickX < boxLeft || clickX > boxLeft+boxWidth {
		return navClickNone
	}

	// Navigation bar is on line: boxTop + border(1) + padding(1) + title(1) + blank(1) = boxTop + 4
	navLineY := boxTop + 4

	// Check if click Y is on the navigation line (with generous tolerance for the gallery row)
	if clickY < navLineY-1 || clickY > navLineY+2 {
		return navClickNone
	}

	// Split box into thirds: left = prev, middle = current (no action), right = next
	thirdWidth := boxWidth / 3
	leftThirdEnd := boxLeft + thirdWidth
	rightThirdStart := boxLeft + 2*thirdWidth

	if clickX < leftThirdEnd {
		return navClickPrev
	} else if clickX >= rightThirdStart {
		return navClickNext
	}

	// Click in middle third (current) - no navigation
	return navClickNone
}

// findClickedGroup returns the index of the group at the click position, or -1
func (m Model) findClickedGroup(clickX, clickY int) int {
	docGroups := m.getGroupsForSelectedSupergroup()
	if len(docGroups) == 0 {
		return -1
	}

	// Box dimensions (must match render.go)
	cardWidth := 68
	boxWidth := cardWidth + 8

	// Calculate box position (centered)
	boxLeft := (m.width - boxWidth) / 2
	boxRight := boxLeft + boxWidth

	// Check X bounds
	if clickX < boxLeft || clickX > boxRight {
		return -1
	}

	// Calculate content layout
	maxContentHeight := m.height - 8
	if maxContentHeight < 10 {
		maxContentHeight = 10
	}

	// Build the same lines array structure as render does
	// Header: title(1) + blank(1) + nav bar(1) + blank(1) + separator(1) + blank(1) = 6 lines
	headerLines := 6

	// Calculate card line ranges (start line index for each card in the lines array)
	cardStarts := make([]int, len(docGroups))
	currentLine := headerLines
	for i, group := range docGroups {
		cardStarts[i] = currentLine
		// Card height: border(2) + title(1) + filepath(1) + description(0-3) + keyfiles(1)
		cardHeight := 5 // border top/bottom + title + filepath + key files
		if group.Description != "" {
			descLines := (len(group.Description) / 60) + 1
			if descLines > 3 {
				descLines = 3
			}
			cardHeight += descLines
		}
		currentLine += cardHeight
	}
	totalLines := currentLine

	// Calculate scroll bounds
	scrollOffset := m.groupsScrollOffset
	maxScroll := totalLines - maxContentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Fixed height calculation (must match render.go)
	fixedHeight := m.height - 6
	if fixedHeight < 15 {
		fixedHeight = 15
	}
	boxTop := (m.height - fixedHeight) / 2

	// Calculate which line in the lines array was clicked
	// From click Y: subtract boxTop, border(1), padding(1)
	contentY := clickY - boxTop - 2 // 2 = border + padding

	// Account for "more above" indicator
	if scrollOffset > 0 {
		contentY-- // first visible line is "more above"
	}

	if contentY < 0 {
		return -1
	}

	// The clicked line in the lines array
	clickedLineIdx := scrollOffset + contentY

	// Find which card contains this line
	for i := len(docGroups) - 1; i >= 0; i-- {
		if clickedLineIdx >= cardStarts[i] {
			return i
		}
	}

	return -1
}

// updateAddGroup handles the add group picker
func (m Model) updateAddGroup(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalFiles := len(m.availableMdFiles)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.addingGroup = false
			return m, nil

		case "up", "k":
			if m.addGroupCursor > 0 {
				m.addGroupCursor--
				m.ensureAddGroupVisible()
			}
			return m, nil

		case "down", "j":
			if m.addGroupCursor < totalFiles-1 {
				m.addGroupCursor++
				m.ensureAddGroupVisible()
			}
			return m, nil

		case "enter":
			// Add the selected file as a context group
			if m.addGroupCursor < totalFiles {
				selectedPath := m.availableMdFiles[m.addGroupCursor]

				// Parse the doc
				group, err := groups.ParseDocContextGroup(m.rootPath, selectedPath)
				if err != nil {
					m.statusMessage = fmt.Sprintf("Error: %v", err)
					m.statusMessageTime = time.Now()
					m.addingGroup = false
					return m, ClearStatusAfter(5 * time.Second)
				}

				// Validate and check staleness
				group.ValidateKeyFiles(m.rootPath)
				group.CheckStaleness(m.rootPath)

				// If file is missing required structure, insert tag
				if len(group.MissingFields) > 0 {
					insertStructureTag(m.rootPath, selectedPath)
				}

				// Initialize registry if needed
				if m.docRegistry == nil {
					m.docRegistry = &groups.DocGroupRegistry{
						Supergroups: groups.DefaultSupergroups(),
						Groups:      []groups.DocContextGroup{},
						BySuper:     make(map[string][]groups.DocContextGroup),
					}
				}

				// Add to registry
				m.docRegistry.Groups = append(m.docRegistry.Groups, *group)

				// Update BySuper map
				sgID := strings.ToLower(strings.ReplaceAll(group.Supergroup, " ", "-"))
				if sgID == "" {
					sgID = "miscellaneous"
				}
				m.docRegistry.BySuper[sgID] = append(m.docRegistry.BySuper[sgID], *group)

				// Save registry
				if err := groups.SaveDocGroupRegistry(m.rootPath, m.docRegistry); err != nil {
					m.statusMessage = fmt.Sprintf("Error saving: %v", err)
				} else if len(group.MissingFields) > 0 {
					m.statusMessage = "Added (incomplete)! Press 'p' for structuring prompt"
				} else {
					m.statusMessage = fmt.Sprintf("Added %s!", group.Name)
				}
				m.statusMessageTime = time.Now()
				m.addingGroup = false
				return m, ClearStatusAfter(5 * time.Second)
			}
			return m, nil
		}

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp {
			m.addGroupScroll--
			if m.addGroupScroll < 0 {
				m.addGroupScroll = 0
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.addGroupScroll++
			maxScroll := totalFiles - (m.height - 12)
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.addGroupScroll > maxScroll {
				m.addGroupScroll = maxScroll
			}
			return m, nil
		}
	}
	return m, nil
}

// ensureAddGroupVisible keeps the cursor visible in add group picker
func (m *Model) ensureAddGroupVisible() {
	maxHeight := m.height - 12
	if maxHeight < 5 {
		maxHeight = 5
	}

	if m.addGroupCursor < m.addGroupScroll {
		m.addGroupScroll = m.addGroupCursor
	} else if m.addGroupCursor >= m.addGroupScroll+maxHeight {
		m.addGroupScroll = m.addGroupCursor - maxHeight + 1
	}
}

// getGroupsForSelectedSupergroup returns groups for the currently selected supergroup
func (m Model) getGroupsForSelectedSupergroup() []groups.DocContextGroup {
	if m.docRegistry == nil || len(m.docRegistry.Supergroups) == 0 {
		return nil
	}

	// Clamp selected supergroup
	sgIdx := m.selectedSupergroup
	if sgIdx < 0 {
		sgIdx = 0
	}
	if sgIdx >= len(m.docRegistry.Supergroups) {
		sgIdx = len(m.docRegistry.Supergroups) - 1
	}

	sg := m.docRegistry.Supergroups[sgIdx]
	return m.docRegistry.BySuper[sg.ID]
}

// getSelectedSupergroupName returns the name of the currently selected supergroup
func (m Model) getSelectedSupergroupName() string {
	if m.docRegistry == nil || len(m.docRegistry.Supergroups) == 0 {
		return ""
	}

	sgIdx := m.selectedSupergroup
	if sgIdx < 0 {
		sgIdx = 0
	}
	if sgIdx >= len(m.docRegistry.Supergroups) {
		sgIdx = len(m.docRegistry.Supergroups) - 1
	}

	return m.docRegistry.Supergroups[sgIdx].Name
}

// insertStructureTag adds the structure-needed tag to a file if not already present
func insertStructureTag(rootPath, filePath string) error {
	fullPath := filepath.Join(rootPath, filePath)

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}

	// Check if tag already exists
	if strings.Contains(string(content), "<!-- contexTUI: structure-needed -->") {
		return nil
	}

	// Prepend tag to file
	newContent := StructureNeededTag + string(content)
	return os.WriteFile(fullPath, []byte(newContent), 0644)
}

// stripContextGroupMetadata removes contexTUI-specific metadata from a markdown file
func stripContextGroupMetadata(rootPath, filePath string) error {
	fullPath := filepath.Join(rootPath, filePath)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip contexTUI metadata lines
		if strings.HasPrefix(trimmed, "**Supergroup:**") ||
			strings.HasPrefix(trimmed, "**Status:**") ||
			strings.HasPrefix(trimmed, "**Related:**") ||
			strings.Contains(trimmed, "<!-- contexTUI: structure-needed -->") {
			continue
		}
		newLines = append(newLines, line)
	}

	return os.WriteFile(fullPath, []byte(strings.Join(newLines, "\n")), 0644)
}
