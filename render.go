package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

	paneHeight := m.height - 4 // header(1) + footer(1) + borders(2)
	footerStyle := lipgloss.NewStyle().Faint(true)

	var body, footer string

	// In copy mode, show only the preview pane at full width with selection highlighting
	if m.selectMode {
		fullWidth := m.width - 4 // borders
		previewStyle := lipgloss.NewStyle().
			Width(fullWidth).
			Height(paneHeight).
			Padding(0, 1).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))

		// Render preview with selection highlighting
		body = previewStyle.Render(m.renderPreviewWithSelection(fullWidth-2, paneHeight))

		selectStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
		if m.selectStart >= 0 && m.selectEnd >= 0 {
			start, end := m.selectStart, m.selectEnd
			if start > end {
				start, end = end, start
			}
			footer = selectStyle.Render(fmt.Sprintf(" COPY MODE [%d-%d] ", start+1, end+1)) +
				footerStyle.Render("drag to select  [c/ctrl+c] copy  [j/k] scroll  [v] copy+exit  [esc] cancel")
		} else {
			footer = selectStyle.Render(" COPY MODE ") +
				footerStyle.Render("drag to select  [c/ctrl+c] copy  [j/k] scroll  [v/esc] exit")
		}
	} else if m.gitStatusMode {
		// Git status view - show changed files list and preview
		body = m.renderGitStatusView(paneHeight)
		gitStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("118")).Bold(true)
		footer = m.renderBranchStatus() + gitStyle.Render("GIT") + footerStyle.Render("  / search  f fetch  esc close  ? help")
	} else {
		// Normal mode - show both panes
		leftWidth := m.leftPaneWidth()
		rightWidth := m.rightPaneWidth()

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

		body = lipgloss.JoinHorizontal(lipgloss.Top, tree, preview)
		footer = m.renderBranchStatus() + footerStyle.Render("/ search  g groups  s git  q quit  ? help")
	}

	mainView := header + "\n" + body + "\n" + footer

	// Overlay help if active
	if m.showingHelp {
		return m.renderHelpOverlay(mainView)
	}

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

// renderPreviewWithSelection renders the preview content with selection highlighting
func (m model) renderPreviewWithSelection(width, height int) string {
	if len(m.previewLines) == 0 {
		return "Select a file to preview"
	}

	var b strings.Builder

	// Highlight style - strip existing colors and apply solid background
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("205")).
		Foreground(lipgloss.Color("0"))

	// Determine selection range
	selStart, selEnd := -1, -1
	if m.selectStart >= 0 && m.selectEnd >= 0 {
		selStart, selEnd = m.selectStart, m.selectEnd
		if selStart > selEnd {
			selStart, selEnd = selEnd, selStart
		}
	}

	// Render visible lines with selection highlighting
	startLine := m.preview.YOffset
	endLine := startLine + height
	if endLine > len(m.previewLines) {
		endLine = len(m.previewLines)
	}

	for i := startLine; i < endLine; i++ {
		line := m.previewLines[i]

		// Check if this line is in the selection
		if selStart >= 0 && i >= selStart && i <= selEnd {
			// Strip ANSI codes and apply highlight (selection overrides syntax colors)
			cleanLine := stripAnsi(line)
			// Pad line to full width for solid highlight block
			if len(cleanLine) < width {
				cleanLine = cleanLine + strings.Repeat(" ", width-len(cleanLine))
			}
			line = highlightStyle.Render(cleanLine)
		}

		b.WriteString(line)
		if i < endLine-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// stripAnsi removes ANSI escape codes from a string
func stripAnsi(s string) string {
	var result strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}
	return result.String()
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

	// Context group badge style
	badgeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Faint(true)

	// Git status styles
	gitStyles := map[string]lipgloss.Style{
		"M": lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true), // Yellow - modified
		"A": lipgloss.NewStyle().Foreground(lipgloss.Color("118")).Bold(true), // Green - added
		"D": lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true), // Red - deleted
		"R": lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true),  // Blue - renamed
		"?": lipgloss.NewStyle().Foreground(lipgloss.Color("244")),            // Gray - untracked
		"U": lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true), // Red - conflict
		"!": lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true), // Red - conflict
	}
	dirIndicatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

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
		relPath, _ := filepath.Rel(m.rootPath, e.path)

		// Add git status badge
		if m.isGitRepo {
			if e.isDir {
				// Directory indicator - show dot if contains changes
				if _, ok := m.gitDirStatus[relPath]; ok {
					line += " " + dirIndicatorStyle.Render("●")
				}
			} else {
				// File status badge
				if status, ok := m.gitStatus[relPath]; ok {
					if style, ok := gitStyles[status.Status]; ok {
						line += " " + style.Render(status.Status)
					}
				}
			}
		}

		// Add context group badges for files
		if !e.isDir {
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

func (m model) renderGroupsOverlay(background string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	separatorStyle := lipgloss.NewStyle().Faint(true)

	// Build content as lines for scrolling support
	var lines []string
	var selectedLineIdx int // Track which line has the selected item

	lines = append(lines, titleStyle.Render("Context Groups"))

	if len(m.contextGroups) == 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render("No groups defined"))
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render("Add groups in .context-groups.md"))
	} else if len(m.layers) == 0 {
		// Fallback to simple list if no layers defined
		lines = append(lines, "")
		for i, group := range m.contextGroups {
			line := fmt.Sprintf("%s (%d files)", group.Name, len(group.Files))
			if m.layerCursor == 0 && i == m.groupCursor {
				selectedLineIdx = len(lines)
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("205")).
					Foreground(lipgloss.Color("0")).
					Render(line)
			} else {
				line = lipgloss.NewStyle().Faint(true).Render(line)
			}
			lines = append(lines, line)
		}
	} else {
		// Swimlane view
		layerNameStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("105"))
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

			lines = append(lines, separatorStyle.Render("─────────────────────────────────────────────────"))
			lines = append(lines, layerNameStyle.Render(layer.Name))

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
					selectedLineIdx = len(lines)
					lines = append(lines, selectedStyle.Render(line))
				} else {
					lines = append(lines, normalStyle.Render(line))
				}
				rendered[group.Name] = true
				groupIdx++

				// Render children indented
				for _, childName := range childrenOf[group.Name] {
					// Find child group
					for _, g := range groups {
						if g.Name == childName {
							isChildSelected := layerIdx == m.layerCursor && groupIdx == m.groupCursor
							childLine := fmt.Sprintf("    ↳ [%s]", g.Name)
							if isChildSelected {
								selectedLineIdx = len(lines)
								lines = append(lines, selectedStyle.Render(childLine))
							} else {
								lines = append(lines, childStyle.Render(childLine))
							}
							rendered[childName] = true
							groupIdx++
							break
						}
					}
				}
			}

			// Render any remaining groups (children without parents in this layer)
			for _, group := range groups {
				if rendered[group.Name] {
					continue
				}
				isSelected := layerIdx == m.layerCursor && groupIdx == m.groupCursor
				line := fmt.Sprintf("  [%s]", group.Name)
				if isSelected {
					selectedLineIdx = len(lines)
					lines = append(lines, selectedStyle.Render(line))
				} else {
					lines = append(lines, normalStyle.Render(line))
				}
				rendered[group.Name] = true
				groupIdx++
			}
		}
	}

	// Calculate max height for scrollable content
	// Account for: border(2) + padding(2) + scroll indicators(2) + details(3) + footer(1) + buffer(2)
	maxContentHeight := m.height - 16
	if maxContentHeight < 8 {
		maxContentHeight = 8
	}

	// Use stored scroll offset (keyboard navigation maintains visibility via ensureGroupSelectionVisible)
	totalLines := len(lines)
	scrollOffset := m.groupsScrollOffset

	// Clamp scroll offset to valid range
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

	// Silence unused variable warning (selectedLineIdx used for highlighting)
	_ = selectedLineIdx

	// Build visible content
	var content strings.Builder
	endIdx := scrollOffset + maxContentHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}

	// Add scroll indicator at top if scrolled
	if scrollOffset > 0 {
		content.WriteString(separatorStyle.Render("  ▲ more above"))
		content.WriteString("\n")
	}

	for i := scrollOffset; i < endIdx; i++ {
		content.WriteString(lines[i])
		content.WriteString("\n")
	}

	// Add scroll indicator at bottom if more content
	if endIdx < totalLines {
		content.WriteString(separatorStyle.Render("  ▼ more below"))
		content.WriteString("\n")
	}

	// Show selected group details (compact, single line each)
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
			// Truncate description to fit on one line
			desc := selectedGroup.Description
			if len(desc) > 48 {
				desc = desc[:45] + "..."
			}
			descStyle := lipgloss.NewStyle().Faint(true)
			content.WriteString(descStyle.Render(desc))
			content.WriteString("\n")
		}
	}

	content.WriteString("\n")
	content.WriteString(lipgloss.NewStyle().Faint(true).Render("[j/k] navigate  [enter/c] copy  [click] copy  [esc] close"))

	// Style the box with max height
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(55).
		MaxHeight(m.height - 4)

	groupsBox := boxStyle.Render(content.String())

	// Center the box
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		groupsBox,
	)
}

// renderBranchStatus returns the git branch name with ahead/behind indicators
func (m model) renderBranchStatus() string {
	if !m.isGitRepo || m.gitBranch == "" {
		return ""
	}

	branchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Bold(true)
	indicatorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	var status string
	if m.gitFetching {
		status = branchStyle.Render(m.gitBranch) + indicatorStyle.Render(" ⟳")
	} else if !m.gitHasUpstream {
		status = branchStyle.Render(m.gitBranch)
	} else if m.gitAhead == 0 && m.gitBehind == 0 {
		status = branchStyle.Render(m.gitBranch) + indicatorStyle.Render(" ✓")
	} else {
		status = branchStyle.Render(m.gitBranch)
		if m.gitAhead > 0 {
			status += indicatorStyle.Render(fmt.Sprintf(" ↑%d", m.gitAhead))
		}
		if m.gitBehind > 0 {
			status += indicatorStyle.Render(fmt.Sprintf(" ↓%d", m.gitBehind))
		}
	}

	return status + "  "
}

// renderHelpOverlay renders the help overlay with all keybindings
func (m model) renderHelpOverlay(background string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	descStyle := lipgloss.NewStyle().Faint(true)

	var content strings.Builder

	content.WriteString(titleStyle.Render("Keyboard Shortcuts"))
	content.WriteString("\n\n")

	// Navigation
	content.WriteString(sectionStyle.Render("Navigation"))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("j/k ↑/↓"), descStyle.Render("Move cursor")))
	content.WriteString(fmt.Sprintf("  %s      %s\n", keyStyle.Render("tab"), descStyle.Render("Switch panes")))
	content.WriteString(fmt.Sprintf("  %s  %s\n", keyStyle.Render("enter/l"), descStyle.Render("Open/expand")))
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("h"), descStyle.Render("Collapse")))
	content.WriteString("\n")

	// Views
	content.WriteString(sectionStyle.Render("Views"))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("s"), descStyle.Render("Git status")))
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("g"), descStyle.Render("Context groups")))
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("/"), descStyle.Render("Search files")))
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("v"), descStyle.Render("Copy mode")))
	content.WriteString("\n")

	// Actions
	content.WriteString(sectionStyle.Render("Actions"))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("c"), descStyle.Render("Copy file path")))
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("f"), descStyle.Render("Git fetch")))
	content.WriteString(fmt.Sprintf("  %s      %s\n", keyStyle.Render("←/→"), descStyle.Render("Resize panes")))
	content.WriteString("\n")

	// General
	content.WriteString(sectionStyle.Render("General"))
	content.WriteString("\n")
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("?"), descStyle.Render("Toggle help")))
	content.WriteString(fmt.Sprintf("  %s        %s\n", keyStyle.Render("q"), descStyle.Render("Quit")))
	content.WriteString("\n")

	content.WriteString(descStyle.Render("Press any key to close"))

	// Style the help box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 3)

	helpBox := boxStyle.Render(content.String())

	// Center the box
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		helpBox,
	)
}
