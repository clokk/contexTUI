package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

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
		footer = m.renderBranchStatus() + footerStyle.Render("/ search  g groups  v select  s git  q quit  ? help")
	}

	// Prepend status message to footer if present and recent
	if m.statusMessage != "" && time.Since(m.statusMessageTime) < 5*time.Second {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("118")).Bold(true)
		footer = statusStyle.Render(m.statusMessage) + "  " + footer
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
	// Use add group picker if in that mode
	if m.addingGroup {
		return m.renderAddGroupOverlay(background)
	}

	// Use doc-based groups rendering
	return m.renderDocGroupsOverlay(background)
}

// renderAddGroupOverlay renders the add group file picker
func (m model) renderAddGroupOverlay(background string) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	selectedStyle := lipgloss.NewStyle().Background(lipgloss.Color("205")).Foreground(lipgloss.Color("0"))
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	metaStyle := lipgloss.NewStyle().Faint(true)
	separatorStyle := lipgloss.NewStyle().Faint(true)

	var lines []string
	lines = append(lines, titleStyle.Render("Add Context Group"))
	lines = append(lines, "")
	lines = append(lines, metaStyle.Render("Select a markdown file to add as a context group:"))
	lines = append(lines, "")

	for i, file := range m.availableMdFiles {
		isSelected := i == m.addGroupCursor
		line := "  " + file
		if isSelected {
			lines = append(lines, selectedStyle.Render(line))
		} else {
			lines = append(lines, normalStyle.Render(line))
		}
	}

	// Scrolling
	maxHeight := m.height - 12
	if maxHeight < 5 {
		maxHeight = 5
	}

	totalLines := len(lines)
	scrollOffset := m.addGroupScroll

	maxScroll := totalLines - maxHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	var content strings.Builder
	endIdx := scrollOffset + maxHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}

	if scrollOffset > 0 {
		content.WriteString(separatorStyle.Render("  ▲ more above"))
		content.WriteString("\n")
	}

	for i := scrollOffset; i < endIdx; i++ {
		content.WriteString(lines[i])
		content.WriteString("\n")
	}

	if endIdx < totalLines {
		content.WriteString(separatorStyle.Render("  ▼ more below"))
	}

	content.WriteString("\n")
	content.WriteString(metaStyle.Render("[j/k] navigate  [enter] add  [esc] cancel"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(70).
		MaxHeight(m.height - 4)

	groupsBox := boxStyle.Render(content.String())

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		groupsBox,
	)
}

// renderDocGroupsOverlay renders the v2 documentation-first context groups as cards
func (m model) renderDocGroupsOverlay(background string) string {
	// Card width for description wrapping
	cardWidth := 68

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	separatorStyle := lipgloss.NewStyle().Faint(true)
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	staleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	metaStyle := lipgloss.NewStyle().Faint(true)
	copiedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))

	// Card styles
	selectedCardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(0, 1).
		Width(cardWidth)

	normalCardStyle := lipgloss.NewStyle().
		Border(lipgloss.HiddenBorder()).
		Padding(0, 1).
		Width(cardWidth)

	var lines []string

	// Title with copy feedback
	titleLine := titleStyle.Render("Context Groups")
	if m.statusMessage != "" && strings.HasPrefix(m.statusMessage, "Copied:") {
		titleLine += "  " + copiedStyle.Render(m.statusMessage)
	}
	lines = append(lines, titleLine)
	lines = append(lines, "")

	// Supergroup gallery navigation - show prev | current | next
	if m.docRegistry != nil && len(m.docRegistry.Supergroups) > 0 {
		numSupergroups := len(m.docRegistry.Supergroups)
		sgIdx := m.selectedSupergroup
		if sgIdx < 0 {
			sgIdx = 0
		}
		if sgIdx >= numSupergroups {
			sgIdx = numSupergroups - 1
		}

		// Get prev, current, next supergroups (with wraparound)
		prevIdx := (sgIdx - 1 + numSupergroups) % numSupergroups
		nextIdx := (sgIdx + 1) % numSupergroups

		prevSg := m.docRegistry.Supergroups[prevIdx]
		currSg := m.docRegistry.Supergroups[sgIdx]
		nextSg := m.docRegistry.Supergroups[nextIdx]

		currCount := len(m.docRegistry.BySuper[currSg.ID])

		// Styles
		fadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		activeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
		arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

		// Build gallery: ◀ PrevName  |  CurrentName (count)  |  NextName ▶
		prevText := fadedStyle.Render(fmt.Sprintf("◀ %s", prevSg.Name))
		currText := activeStyle.Render(fmt.Sprintf("%s (%d)", currSg.Name, currCount))
		nextText := fadedStyle.Render(fmt.Sprintf("%s ▶", nextSg.Name))
		divider := arrowStyle.Render("  │  ")

		navLine := prevText + divider + currText + divider + nextText

		// Center the navigation bar
		centeredNav := lipgloss.NewStyle().Width(cardWidth).Align(lipgloss.Center).Render(navLine)
		lines = append(lines, centeredNav)
		lines = append(lines, "")
		lines = append(lines, separatorStyle.Render("────────────────────────────────────────────────────────────────"))
		lines = append(lines, "")
	}

	// Get groups for selected supergroup
	groups := m.getGroupsForSelectedSupergroup()

	if m.docRegistry == nil || len(m.docRegistry.Groups) == 0 {
		lines = append(lines, metaStyle.Render("No context groups defined yet."))
		lines = append(lines, "")
		lines = append(lines, metaStyle.Render("Press 'a' to add a markdown file as a context group."))
	} else if len(groups) == 0 {
		lines = append(lines, metaStyle.Render("No groups in this category."))
		lines = append(lines, "")
		lines = append(lines, metaStyle.Render("Use h/l to switch categories, or 'a' to add a group."))
	} else {
		// Render each group as a card
		for groupIdx, group := range groups {
			isSelected := groupIdx == m.docGroupCursor

			// Build card content
			var cardLines []string

			// Selection indicator
			selectionPrefix := "  "
			if m.selectedGroups[group.FilePath] {
				selectionPrefix = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("✓ ")
			}

			// Title line with status indicators
			titleLine := selectionPrefix + lipgloss.NewStyle().Bold(true).Render(group.Name)

			// Status badge
			statusBadge := ""
			if group.Status != "" {
				statusColor := "244" // gray default
				switch group.Status {
				case "Active":
					statusColor = "82" // green
				case "Deprecated":
					statusColor = "196" // red
				case "Experimental":
					statusColor = "214" // orange
				case "Planned":
					statusColor = "105" // purple
				}
				statusBadge = lipgloss.NewStyle().
					Foreground(lipgloss.Color(statusColor)).
					Render(" [" + group.Status + "]")
			}

			// Issue indicators
			var indicators []string
			if len(group.MissingFields) > 0 {
				indicators = append(indicators, warningStyle.Render(" ⚠ incomplete"))
			}
			if len(group.BrokenKeyFiles) > 0 {
				indicators = append(indicators, errorStyle.Render(" ✗ broken refs"))
			}
			if group.IsStale {
				indicators = append(indicators, staleStyle.Render(" ○ stale"))
			}

			cardLines = append(cardLines, titleLine+statusBadge+strings.Join(indicators, ""))

			// Filepath - show below title for clarity
			cardLines = append(cardLines, metaStyle.Render(group.FilePath))

			// Description - word wrap to card width
			if group.Description != "" {
				desc := group.Description
				wrapped := wrapText(desc, cardWidth-4)
				for _, line := range wrapped {
					cardLines = append(cardLines, descStyle.Render(line))
				}
			}

			// Key files count and token estimate
			var metaParts []string
			if len(group.KeyFiles) > 0 {
				metaParts = append(metaParts, fmt.Sprintf("%d key files", len(group.KeyFiles)))
			}
			if group.TokenEstimate > 0 {
				metaParts = append(metaParts, fmt.Sprintf("~%d tokens", group.TokenEstimate))
			}
			if len(metaParts) > 0 {
				cardLines = append(cardLines, metaStyle.Render(strings.Join(metaParts, " · ")))
			}

			// Render the card
			cardContent := strings.Join(cardLines, "\n")
			var renderedCard string
			if isSelected {
				renderedCard = selectedCardStyle.Render(cardContent)
			} else {
				renderedCard = normalCardStyle.Render(cardContent)
			}

			// Add card lines
			for _, line := range strings.Split(renderedCard, "\n") {
				lines = append(lines, line)
			}
		}
	}

	// Calculate scrolling
	maxContentHeight := m.height - 8
	if maxContentHeight < 10 {
		maxContentHeight = 10
	}

	totalLines := len(lines)
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

	// Build visible content
	var content strings.Builder
	endIdx := scrollOffset + maxContentHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}

	if scrollOffset > 0 {
		content.WriteString(separatorStyle.Render("  ▲ more above"))
		content.WriteString("\n")
	}

	for i := scrollOffset; i < endIdx; i++ {
		content.WriteString(lines[i])
		content.WriteString("\n")
	}

	if endIdx < totalLines {
		content.WriteString(separatorStyle.Render("  ▼ more below"))
	}

	content.WriteString("\n")
	// Footer with status message or selection count
	footerText := "[h/l] category  [j/k] nav  [space] select  [c] copy  [a] add  [d] remove  [p] prompt  [esc] close"
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	if m.statusMessage != "" && time.Since(m.statusMessageTime) < 5*time.Second {
		// Show status message (copy feedback, etc.)
		content.WriteString(statusStyle.Render(m.statusMessage))
		content.WriteString("  ")
	} else if len(m.selectedGroups) > 0 {
		// Show selection count when no status message
		content.WriteString(statusStyle.Render(fmt.Sprintf("%d selected  ", len(m.selectedGroups))))
	}
	content.WriteString(metaStyle.Render(footerText))

	// Use fixed height for consistent overlay position
	fixedHeight := m.height - 6
	if fixedHeight < 15 {
		fixedHeight = 15
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(cardWidth + 8).
		Height(fixedHeight)

	groupsBox := boxStyle.Render(content.String())

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		groupsBox,
	)
}

// wrapText wraps text to the specified width
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	var lines []string
	words := strings.Fields(text)
	if len(words) == 0 {
		return lines
	}

	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= width {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)

	// Limit to 3 lines max
	if len(lines) > 3 {
		lines = lines[:3]
		lines[2] = lines[2][:min(len(lines[2]), width-3)] + "..."
	}

	return lines
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
