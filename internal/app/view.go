package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/connorleisz/contexTUI/internal/ui/styles"
)

// View implements tea.Model
func (m Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header
	headerStyle := styles.Header.Copy().Padding(0, 1)

	header := headerStyle.Render("contexTUI") +
		styles.Faint.Render(" " + m.rootPath)

	// Add loading spinner to header if loading
	if m.loadingMessage != "" {
		spinner := string(SpinnerChars[m.spinnerFrame])
		loadingIndicator := styles.StatusWarning.Render(spinner + " " + m.loadingMessage)
		// Calculate padding to right-align the loading indicator
		headerLen := lipgloss.Width(header)
		loadingLen := lipgloss.Width(loadingIndicator)
		padding := m.width - headerLen - loadingLen - 2
		if padding > 0 {
			header = header + strings.Repeat(" ", padding) + loadingIndicator
		} else {
			header = header + " " + loadingIndicator
		}
	}

	paneHeight := m.height - 4 // header(1) + footer(1) + borders(2)
	footerStyle := styles.Faint

	var body, footer string

	// In copy mode, show only the preview pane at full width with selection highlighting
	if m.selectMode {
		fullWidth := m.width - 4 // borders
		previewStyle := styles.ActiveBorder().
			Width(fullWidth).
			Height(paneHeight).
			Padding(0, 1)

		// Render preview with selection highlighting
		body = previewStyle.Render(m.renderPreviewWithSelection(fullWidth-2, paneHeight))

		selectStyle := styles.Header
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
		gitStyle := styles.StatusSuccess
		footer = m.renderBranchStatus() + gitStyle.Render("GIT") + footerStyle.Render("  / search  f fetch  esc close  ? help")
	} else {
		// Normal mode - show both panes
		leftWidth := m.LeftPaneWidth()
		rightWidth := m.RightPaneWidth()

		var treeStyle lipgloss.Style
		if m.activePane == TreePane {
			treeStyle = styles.ActiveBorder()
		} else {
			treeStyle = styles.InactiveBorder()
		}
		treeStyle = treeStyle.
			Width(leftWidth).
			Height(paneHeight).
			Padding(0, 1)

		tree := treeStyle.Render(m.tree.View())

		var previewStyle lipgloss.Style
		if m.activePane == PreviewPane {
			previewStyle = styles.ActiveBorder()
		} else {
			previewStyle = styles.InactiveBorder()
		}
		previewStyle = previewStyle.
			Width(rightWidth).
			Height(paneHeight).
			Padding(0, 1)

		preview := previewStyle.Render(m.preview.View())

		body = lipgloss.JoinHorizontal(lipgloss.Top, tree, preview)
		footer = m.renderBranchStatus() + footerStyle.Render("/ search  g docs  v select  s git  q quit  ? help")
	}

	// Prepend status message to footer if present and recent
	if m.statusMessage != "" && time.Since(m.statusMessageTime) < 5*time.Second {
		footer = styles.StatusSuccess.Render(m.statusMessage) + "  " + footer
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

	// Overlay docs if active
	if m.showingDocs {
		return m.renderDocsOverlay(mainView)
	}

	// Overlay file operation if active
	if m.fileOpMode != FileOpNone {
		return m.renderFileOpOverlay(mainView)
	}

	return mainView
}

// renderPreviewWithSelection renders the preview content with selection highlighting
func (m Model) renderPreviewWithSelection(width, height int) string {
	if len(m.previewLines) == 0 {
		return "Select a file to preview"
	}

	var b strings.Builder

	// Highlight style - strip existing colors and apply solid background
	highlightStyle := styles.Highlight

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

func (m Model) renderSearchOverlay(background string) string {
	metaStyle := styles.Faint

	// Calculate box dimensions based on viewport
	boxWidth := m.width * 60 / 100
	if boxWidth > 70 {
		boxWidth = 70
	}
	if boxWidth < 40 {
		boxWidth = 40
	}

	fixedHeight := m.height - 6
	if fixedHeight < 10 {
		fixedHeight = 10
	}
	if fixedHeight > 25 {
		fixedHeight = 25
	}

	// Calculate max visible results (account for input, padding, indicators, footer)
	maxVisibleResults := fixedHeight - 7
	if maxVisibleResults < 3 {
		maxVisibleResults = 3
	}

	// Build search box content
	var content strings.Builder
	content.WriteString(m.searchInput.View())
	content.WriteString("\n\n")

	if len(m.searchResults) == 0 && m.searchInput.Value() != "" {
		content.WriteString(styles.Faint.Render("No matches"))
	} else if len(m.searchResults) > 0 {
		totalResults := len(m.searchResults)

		// Clamp scroll offset
		maxScroll := totalResults - maxVisibleResults
		if maxScroll < 0 {
			maxScroll = 0
		}
		scrollOffset := m.searchScrollOffset
		if scrollOffset > maxScroll {
			scrollOffset = maxScroll
		}
		if scrollOffset < 0 {
			scrollOffset = 0
		}

		// Scroll indicator (above)
		if scrollOffset > 0 {
			content.WriteString(metaStyle.Render("  ▲ more above"))
			content.WriteString("\n")
		}

		// Render visible results
		endIdx := scrollOffset + maxVisibleResults
		if endIdx > totalResults {
			endIdx = totalResults
		}

		for i := scrollOffset; i < endIdx; i++ {
			result := m.searchResults[i]
			line := result.DisplayName
			if i == m.searchCursor {
				line = styles.Selected.Render(line)
			} else {
				line = styles.Faint.Render(line)
			}
			content.WriteString(line + "\n")
		}

		// Scroll indicator (below)
		if endIdx < totalResults {
			content.WriteString(metaStyle.Render("  ▼ more below"))
			content.WriteString("\n")
		}

		// Result count
		content.WriteString("\n")
		content.WriteString(metaStyle.Render(fmt.Sprintf("%d results", totalResults)))
	}

	// Style the search box
	boxStyle := styles.ActiveBorder().
		Padding(1, 2).
		Width(boxWidth).
		Height(fixedHeight)

	searchBox := boxStyle.Render(content.String())

	// Center the search box
	centeredBox := lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		searchBox,
	)

	return centeredBox
}

// RenderTree renders the tree pane content
func (m Model) RenderTree() string {
	var b strings.Builder
	flat := m.FlatEntries()

	// Git status styles
	gitStyles := styles.GitStatusStyles()
	dirIndicatorStyle := lipgloss.NewStyle().Foreground(styles.TextFaint)

	for i, e := range flat {
		indent := strings.Repeat("  ", e.Depth)

		icon := "  "
		if e.IsDir {
			if e.Expanded {
				icon = "v "
			} else {
				icon = "> "
			}
		}

		line := indent + icon + e.Name
		relPath, _ := filepath.Rel(m.rootPath, e.Path)

		// Add git status badge
		if m.isGitRepo {
			if e.IsDir {
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
			line = styles.Selected.Render(line)
		} else if e.IsDir {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

func (m Model) renderDocsOverlay(background string) string {
	// Use add doc picker if in that mode
	if m.addingDoc {
		return m.renderAddDocOverlay(background)
	}

	// Use doc-based rendering
	return m.renderContextDocsOverlay(background)
}

// renderAddDocOverlay renders the add doc file picker
func (m Model) renderAddDocOverlay(background string) string {
	titleStyle := styles.Title
	selectedStyle := styles.Selected
	normalStyle := styles.Normal
	metaStyle := styles.Faint
	separatorStyle := styles.Faint

	var lines []string
	lines = append(lines, titleStyle.Render("Add Context Doc"))
	lines = append(lines, "")
	lines = append(lines, metaStyle.Render("Select a markdown file to add as a context doc:"))
	lines = append(lines, "")

	for i, file := range m.availableMdFiles {
		isCursor := i == m.addDocCursor

		// Selection indicator (checkmark for selected files)
		selectionPrefix := "  "
		if m.selectedAddFiles[file] {
			selectionPrefix = lipgloss.NewStyle().Foreground(styles.SuccessBold).Render("✓ ")
		}

		line := selectionPrefix + file
		if isCursor {
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
	scrollOffset := m.addDocScroll

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
	// Show selection count if any files selected
	if len(m.selectedAddFiles) > 0 {
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
		content.WriteString(statusStyle.Render(fmt.Sprintf("%d selected  ", len(m.selectedAddFiles))))
	}
	content.WriteString(metaStyle.Render("[j/k] navigate  [space] select  [enter] add  [esc] cancel"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(70).
		MaxHeight(m.height - 4)

	docsBox := boxStyle.Render(content.String())

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		docsBox,
	)
}

// getDocsColumnCount returns how many columns to use based on terminal aspect ratio
func (m Model) getDocsColumnCount() int {
	// Card width (68) + padding (8) + gap between columns (2)
	colWidth := 78

	// Use aspect ratio to determine columns - wider terminals get more columns
	// Also ensure we have enough absolute width for the columns
	if m.width >= m.height*3 && m.width >= colWidth*3 {
		return 3
	}
	if m.width >= m.height*2 && m.width >= colWidth*2 {
		return 2
	}
	return 1
}

// renderContextDocsOverlay renders the v2 documentation-first context docs as cards
// Supports multi-column layout for wide terminals
func (m Model) renderContextDocsOverlay(background string) string {
	// Card width for description wrapping
	cardWidth := 68

	// Calculate column count based on terminal aspect ratio
	numCols := m.getDocsColumnCount()

	// Calculate total content width: cards + padding + gaps between columns
	// Each card has width cardWidth, plus 4 for padding (2 each side from card style)
	// Gap between columns is 2 spaces
	contentWidth := (cardWidth+4)*numCols + (numCols-1)*2
	boxWidth := contentWidth + 8 // Add box padding (4) and border (4)

	titleStyle := styles.Title
	separatorStyle := styles.Faint
	warningStyle := styles.StatusWarning
	errorStyle := styles.StatusError
	staleStyle := lipgloss.NewStyle().Foreground(styles.TextFaint)
	descStyle := styles.Muted
	metaStyle := styles.Faint
	copiedStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.SuccessBold)

	// Card styles
	selectedCardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.BorderActive).
		Padding(0, 1).
		Width(cardWidth)

	normalCardStyle := lipgloss.NewStyle().
		Border(lipgloss.HiddenBorder()).
		Padding(0, 1).
		Width(cardWidth)

	// === STICKY HEADER (not scrolled) ===
	var headerLines []string

	// Title with copy feedback - centered across full width
	titleLine := titleStyle.Render("Context Docs")
	if m.statusMessage != "" && strings.HasPrefix(m.statusMessage, "Copied:") {
		titleLine += "  " + copiedStyle.Render(m.statusMessage)
	}
	headerLines = append(headerLines, titleLine)
	headerLines = append(headerLines, "")

	// Category gallery navigation - show prev | current | next
	if m.docRegistry != nil && len(m.docRegistry.Categories) > 0 {
		numCategories := len(m.docRegistry.Categories)
		catIdx := m.selectedCategory
		if catIdx < 0 {
			catIdx = 0
		}
		if catIdx >= numCategories {
			catIdx = numCategories - 1
		}

		// Get prev, current, next categories (with wraparound)
		prevIdx := (catIdx - 1 + numCategories) % numCategories
		nextIdx := (catIdx + 1) % numCategories

		prevCat := m.docRegistry.Categories[prevIdx]
		currCat := m.docRegistry.Categories[catIdx]
		nextCat := m.docRegistry.Categories[nextIdx]

		currCount := len(m.docRegistry.ByCategory[currCat.ID])

		// Styles
		fadedStyle := lipgloss.NewStyle().Foreground(styles.BorderInactive)
		activeStyle := styles.Header
		arrowStyle := lipgloss.NewStyle().Foreground(styles.TextFaint)

		// Build gallery: ◀ PrevName  |  CurrentName (count)  |  NextName ▶
		prevText := fadedStyle.Render(fmt.Sprintf("◀ %s", prevCat.Name))
		currText := activeStyle.Render(fmt.Sprintf("%s (%d)", currCat.Name, currCount))
		nextText := fadedStyle.Render(fmt.Sprintf("%s ▶", nextCat.Name))
		divider := arrowStyle.Render("  │  ")

		navLine := prevText + divider + currText + divider + nextText

		// Center the navigation bar across full content width
		centeredNav := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(navLine)
		headerLines = append(headerLines, centeredNav)
		headerLines = append(headerLines, "")
		// Separator spans full content width
		headerLines = append(headerLines, separatorStyle.Render(strings.Repeat("─", contentWidth)))
		headerLines = append(headerLines, "")
	}

	// === SCROLLABLE CONTENT (cards in columns) ===
	var cardLines []string

	// Get docs for selected category
	docs := m.getDocsForSelectedCategory()

	if m.docRegistry == nil || len(m.docRegistry.Docs) == 0 {
		cardLines = append(cardLines, metaStyle.Render("No context docs defined yet."))
		cardLines = append(cardLines, "")
		cardLines = append(cardLines, metaStyle.Render("Press 'a' to add a markdown file as a context doc."))
	} else if len(docs) == 0 {
		cardLines = append(cardLines, metaStyle.Render("No docs in this category."))
		cardLines = append(cardLines, "")
		cardLines = append(cardLines, metaStyle.Render("Use h/l to switch categories, or 'a' to add a doc."))
	} else {
		// Calculate docs per column (column-first ordering)
		docsPerCol := (len(docs) + numCols - 1) / numCols // Round up

		// Render cards into separate column buffers
		columns := make([][]string, numCols)
		for i := range columns {
			columns[i] = []string{}
		}

		for docIdx, doc := range docs {
			// Column-first ordering: doc 0,1,2 in col 0, doc 3,4,5 in col 1, etc.
			colIdx := docIdx / docsPerCol
			if colIdx >= numCols {
				colIdx = numCols - 1
			}

			isSelected := docIdx == m.docCursor

			// Build card content
			var cardContent []string

			// Selection indicator
			selectionPrefix := "  "
			if m.selectedDocs[doc.FilePath] {
				selectionPrefix = lipgloss.NewStyle().Foreground(styles.SuccessBold).Render("✓ ")
			}

			// Title line with status indicators
			cardTitleLine := selectionPrefix + lipgloss.NewStyle().Bold(true).Render(doc.Name)

			// Status badge
			statusBadge := ""
			if doc.Status != "" {
				statusColor := "244" // gray default
				switch doc.Status {
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
					Render(" [" + doc.Status + "]")
			}

			// Issue indicators
			var indicators []string
			if len(doc.MissingFields) > 0 {
				indicators = append(indicators, warningStyle.Render(" ⚠ incomplete"))
			}
			if len(doc.BrokenKeyFiles) > 0 {
				indicators = append(indicators, errorStyle.Render(" ✗ broken refs"))
			}
			if doc.IsStale {
				indicators = append(indicators, staleStyle.Render(" ○ stale"))
			}

			cardContent = append(cardContent, cardTitleLine+statusBadge+strings.Join(indicators, ""))

			// Filepath - show below title for clarity
			cardContent = append(cardContent, metaStyle.Render(doc.FilePath))

			// Description - word wrap to card width
			if doc.Description != "" {
				desc := doc.Description
				wrapped := wrapText(desc, cardWidth-4)
				for _, line := range wrapped {
					cardContent = append(cardContent, descStyle.Render(line))
				}
			}

			// Key files count and token estimate
			var metaParts []string
			if len(doc.KeyFiles) > 0 {
				metaParts = append(metaParts, fmt.Sprintf("%d key files", len(doc.KeyFiles)))
			}
			if doc.TokenEstimate > 0 {
				metaParts = append(metaParts, fmt.Sprintf("~%d tokens", doc.TokenEstimate))
			}
			if len(metaParts) > 0 {
				cardContent = append(cardContent, metaStyle.Render(strings.Join(metaParts, " · ")))
			}

			// Render the card
			cardContentStr := strings.Join(cardContent, "\n")
			var renderedCard string
			if isSelected {
				renderedCard = selectedCardStyle.Render(cardContentStr)
			} else {
				renderedCard = normalCardStyle.Render(cardContentStr)
			}

			// Add card lines to the appropriate column
			for _, line := range strings.Split(renderedCard, "\n") {
				columns[colIdx] = append(columns[colIdx], line)
			}
		}

		// Join columns horizontally line by line
		// First, find the max height across all columns
		maxColHeight := 0
		for _, col := range columns {
			if len(col) > maxColHeight {
				maxColHeight = len(col)
			}
		}

		// Build combined lines by joining corresponding lines from each column
		emptyCard := strings.Repeat(" ", cardWidth+4) // Width of a card including padding
		colGap := "  "                                // Gap between columns

		for lineIdx := 0; lineIdx < maxColHeight; lineIdx++ {
			var rowParts []string
			for _, col := range columns {
				if lineIdx < len(col) {
					rowParts = append(rowParts, col[lineIdx])
				} else {
					rowParts = append(rowParts, emptyCard)
				}
			}
			cardLines = append(cardLines, strings.Join(rowParts, colGap))
		}
	}

	// === CALCULATE SCROLLING (cards only) ===
	headerHeight := len(headerLines)
	footerHeight := 3 // scroll indicator + footer line + padding

	maxContentHeight := m.height - 8 - headerHeight - footerHeight
	if maxContentHeight < 5 {
		maxContentHeight = 5
	}

	totalCardLines := len(cardLines)
	scrollOffset := m.docsScrollOffset

	maxScroll := totalCardLines - maxContentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// === BUILD FINAL CONTENT ===
	var content strings.Builder

	// 1. Sticky header (always visible)
	for _, line := range headerLines {
		content.WriteString(line)
		content.WriteString("\n")
	}

	// 2. Scroll indicator (above)
	if scrollOffset > 0 {
		content.WriteString(separatorStyle.Render("  ▲ more above"))
		content.WriteString("\n")
	}

	// 3. Visible cards (scrolled portion)
	endIdx := scrollOffset + maxContentHeight
	if endIdx > totalCardLines {
		endIdx = totalCardLines
	}

	for i := scrollOffset; i < endIdx; i++ {
		content.WriteString(cardLines[i])
		content.WriteString("\n")
	}

	// 4. Scroll indicator (below)
	if endIdx < totalCardLines {
		content.WriteString(separatorStyle.Render("  ▼ more below"))
	}

	content.WriteString("\n")

	// 5. Footer with status message or selection count
	footerText := "[h/l] cat  [j/k] nav  [J/K] reorder  [space] select  [c] copy  [a] add  [d] rm  [esc] close"
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	if m.statusMessage != "" && time.Since(m.statusMessageTime) < 5*time.Second {
		// Show status message (copy feedback, etc.)
		content.WriteString(statusStyle.Render(m.statusMessage))
		content.WriteString("  ")
	} else if len(m.selectedDocs) > 0 {
		// Show selection count when no status message
		content.WriteString(statusStyle.Render(fmt.Sprintf("%d selected  ", len(m.selectedDocs))))
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
		Width(boxWidth).
		Height(fixedHeight)

	docsBox := boxStyle.Render(content.String())

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		docsBox,
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
func (m Model) renderBranchStatus() string {
	if !m.isGitRepo || m.gitBranch == "" {
		return ""
	}

	branchStyle := styles.Branch
	indicatorStyle := lipgloss.NewStyle().Foreground(styles.TextFaint)

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
func (m Model) renderHelpOverlay(background string) string {
	titleStyle := styles.Title
	sectionStyle := styles.SectionHeader
	keyStyle := styles.Key
	descStyle := styles.Faint
	metaStyle := styles.Faint

	// Calculate box dimensions based on viewport
	boxWidth := m.width * 70 / 100
	if boxWidth > 80 {
		boxWidth = 80
	}
	if boxWidth < 50 {
		boxWidth = 50
	}

	fixedHeight := m.height - 6
	if fixedHeight < 15 {
		fixedHeight = 15
	}
	if fixedHeight > 30 {
		fixedHeight = 30
	}

	// Build content as lines array
	var contentLines []string

	contentLines = append(contentLines, titleStyle.Render("Keyboard Shortcuts"))
	contentLines = append(contentLines, "")

	// Navigation
	contentLines = append(contentLines, sectionStyle.Render("Navigation"))
	contentLines = append(contentLines, fmt.Sprintf("  %s  %s", keyStyle.Render("j/k ↑/↓"), descStyle.Render("Move cursor")))
	contentLines = append(contentLines, fmt.Sprintf("  %s      %s", keyStyle.Render("tab"), descStyle.Render("Switch panes")))
	contentLines = append(contentLines, fmt.Sprintf("  %s  %s", keyStyle.Render("enter/l"), descStyle.Render("Open/expand")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("h"), descStyle.Render("Collapse")))
	contentLines = append(contentLines, "")

	// Views
	contentLines = append(contentLines, sectionStyle.Render("Views"))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("s"), descStyle.Render("Git status")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("g"), descStyle.Render("Context docs")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("/"), descStyle.Render("Search files")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("v"), descStyle.Render("Copy mode")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("."), descStyle.Render("Toggle dotfiles")))
	contentLines = append(contentLines, "")

	// Actions
	contentLines = append(contentLines, sectionStyle.Render("Actions"))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("n"), descStyle.Render("Create file")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("N"), descStyle.Render("Create folder")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("r"), descStyle.Render("Rename")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("d"), descStyle.Render("Delete")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("c"), descStyle.Render("Copy file path")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("f"), descStyle.Render("Git fetch")))
	contentLines = append(contentLines, fmt.Sprintf("  %s      %s", keyStyle.Render("←/→"), descStyle.Render("Resize panes")))
	contentLines = append(contentLines, "")

	// General
	contentLines = append(contentLines, sectionStyle.Render("General"))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("?"), descStyle.Render("Toggle help")))
	contentLines = append(contentLines, fmt.Sprintf("  %s        %s", keyStyle.Render("q"), descStyle.Render("Quit")))

	// Calculate scrolling
	maxContentHeight := fixedHeight - 4 // Account for box padding/borders
	totalLines := len(contentLines)

	// Clamp scroll offset
	maxScroll := totalLines - maxContentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	scrollOffset := m.helpScrollOffset
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Build final content with scroll indicators
	var content strings.Builder

	// Scroll indicator (above)
	if scrollOffset > 0 {
		content.WriteString(metaStyle.Render("  ▲ more above"))
		content.WriteString("\n")
	}

	// Visible content
	endIdx := scrollOffset + maxContentHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}

	for i := scrollOffset; i < endIdx; i++ {
		content.WriteString(contentLines[i])
		content.WriteString("\n")
	}

	// Scroll indicator (below)
	if endIdx < totalLines {
		content.WriteString(metaStyle.Render("  ▼ more below"))
		content.WriteString("\n")
	}

	// Footer
	content.WriteString("\n")
	content.WriteString(descStyle.Render("q/esc close · j/k scroll"))

	// Style the help box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 3).
		Width(boxWidth).
		Height(fixedHeight)

	helpBox := boxStyle.Render(content.String())

	// Center the box
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		helpBox,
	)
}

// renderGitFileList renders just the categorized file list for the git viewport
func (m Model) renderGitFileList() string {
	leftWidth := m.LeftPaneWidth()

	if len(m.gitChanges) == 0 {
		return styles.Faint.Render("Working tree clean")
	}

	var b strings.Builder

	// Group by status
	staged, unstaged, untracked := m.CategorizeGitChanges()

	// Style definitions
	stagedStyle := lipgloss.NewStyle().Foreground(styles.GitAdded).Bold(true)
	unstagedStyle := lipgloss.NewStyle().Foreground(styles.GitModified).Bold(true)
	untrackedStyle := lipgloss.NewStyle().Foreground(styles.GitUntracked)
	selectedStyle := styles.Selected

	statusStyles := styles.GitStatusStyles()

	idx := 0

	// Render staged changes
	if len(staged) > 0 {
		b.WriteString(stagedStyle.Render("Staged Changes"))
		b.WriteString("\n")
		for _, c := range staged {
			var line string
			if idx == m.gitStatusCursor {
				line = fmt.Sprintf("  %s %s", c.Status, c.Path)
				if len(line) < leftWidth-4 {
					line = line + strings.Repeat(" ", leftWidth-4-len(line))
				}
				line = selectedStyle.Render(line)
			} else {
				statusStyle := statusStyles[c.Status]
				line = fmt.Sprintf("  %s %s", statusStyle.Render(c.Status), c.Path)
			}
			b.WriteString(line + "\n")
			idx++
		}
		b.WriteString("\n")
	}

	// Render unstaged changes
	if len(unstaged) > 0 {
		b.WriteString(unstagedStyle.Render("Changes not staged"))
		b.WriteString("\n")
		for _, c := range unstaged {
			var line string
			if idx == m.gitStatusCursor {
				line = fmt.Sprintf("  %s %s", c.Status, c.Path)
				if len(line) < leftWidth-4 {
					line = line + strings.Repeat(" ", leftWidth-4-len(line))
				}
				line = selectedStyle.Render(line)
			} else {
				statusStyle := statusStyles[c.Status]
				line = fmt.Sprintf("  %s %s", statusStyle.Render(c.Status), c.Path)
			}
			b.WriteString(line + "\n")
			idx++
		}
		b.WriteString("\n")
	}

	// Render untracked files
	if len(untracked) > 0 {
		b.WriteString(untrackedStyle.Render("Untracked files"))
		b.WriteString("\n")
		for _, c := range untracked {
			var line string
			if idx == m.gitStatusCursor {
				line = fmt.Sprintf("  %s %s", c.Status, c.Path)
				if len(line) < leftWidth-4 {
					line = line + strings.Repeat(" ", leftWidth-4-len(line))
				}
				line = selectedStyle.Render(line)
			} else {
				line = fmt.Sprintf("  %s %s", untrackedStyle.Render(c.Status), c.Path)
			}
			b.WriteString(line + "\n")
			idx++
		}
	}

	return strings.TrimSuffix(b.String(), "\n")
}

// renderGitStatusView renders the git status view with file list and preview
func (m Model) renderGitStatusView(paneHeight int) string {
	leftWidth := m.LeftPaneWidth()
	rightWidth := m.RightPaneWidth()

	// Left pane: Header + scrollable file list
	header := styles.Header.Render("Git Status") + "\n\n"
	leftContent := header + m.gitList.View()

	// Style the left pane
	var leftPaneStyle lipgloss.Style
	if m.activePane == TreePane {
		leftPaneStyle = styles.ActiveBorder()
	} else {
		leftPaneStyle = styles.InactiveBorder()
	}
	leftPaneStyle = leftPaneStyle.
		Width(leftWidth).
		Height(paneHeight).
		Padding(0, 1)

	// Right pane: File preview
	var previewStyle lipgloss.Style
	if m.activePane == PreviewPane {
		previewStyle = styles.ActiveBorder()
	} else {
		previewStyle = styles.InactiveBorder()
	}
	previewStyle = previewStyle.
		Width(rightWidth).
		Height(paneHeight).
		Padding(0, 1)

	leftPane := leftPaneStyle.Render(leftContent)
	rightPane := previewStyle.Render(m.preview.View())

	return lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
}

// renderFileOpOverlay renders the file operation overlay (create/rename/delete)
func (m Model) renderFileOpOverlay(background string) string {
	// Calculate box dimensions based on viewport
	boxWidth := m.width * 80 / 100
	if boxWidth > 100 {
		boxWidth = 100
	}
	if boxWidth < 50 {
		boxWidth = 50
	}

	// Fixed height based on viewport
	fixedHeight := m.height - 6
	if fixedHeight < 15 {
		fixedHeight = 15
	}
	if fixedHeight > 20 {
		fixedHeight = 20 // Cap height for file operations (they don't need much)
	}

	// Update text input width to match box
	inputWidth := boxWidth - 8 // Account for padding and borders
	if inputWidth > 60 {
		inputWidth = 60
	}
	m.fileOpInput.Width = inputWidth

	// Styles
	titleStyle := styles.Header
	metaStyle := styles.Faint
	errorStyle := styles.StatusError
	warningStyle := styles.StatusWarning

	var contentLines []string

	switch m.fileOpMode {
	case FileOpCreateFile:
		contentLines = append(contentLines, titleStyle.Render("Create File"))
		contentLines = append(contentLines, "")
		// Wrap target path if long
		targetLabel := "in: " + m.fileOpTargetPath
		wrapped := wrapText(targetLabel, boxWidth-8)
		for _, line := range wrapped {
			contentLines = append(contentLines, metaStyle.Render(line))
		}
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, m.fileOpInput.View())

	case FileOpCreateFolder:
		contentLines = append(contentLines, titleStyle.Render("Create Folder"))
		contentLines = append(contentLines, "")
		targetLabel := "in: " + m.fileOpTargetPath
		wrapped := wrapText(targetLabel, boxWidth-8)
		for _, line := range wrapped {
			contentLines = append(contentLines, metaStyle.Render(line))
		}
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, m.fileOpInput.View())

	case FileOpRename:
		contentLines = append(contentLines, titleStyle.Render("Rename"))
		contentLines = append(contentLines, "")
		wrapped := wrapText(m.fileOpTargetPath, boxWidth-8)
		for _, line := range wrapped {
			contentLines = append(contentLines, metaStyle.Render(line))
		}
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, m.fileOpInput.View())

	case FileOpDelete:
		contentLines = append(contentLines, warningStyle.Render("Delete"))
		contentLines = append(contentLines, "")
		wrapped := wrapText(m.fileOpTargetPath, boxWidth-8)
		for _, line := range wrapped {
			contentLines = append(contentLines, line)
		}
		contentLines = append(contentLines, "")
		if m.fileOpConfirm {
			contentLines = append(contentLines, errorStyle.Render("Press Enter or 'y' to confirm deletion"))
		} else {
			contentLines = append(contentLines, metaStyle.Render("Press Enter to confirm"))
		}
	}

	// Add error message if present
	if m.fileOpError != "" {
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, errorStyle.Render(m.fileOpError))
	}

	// Add footer hint
	contentLines = append(contentLines, "")
	contentLines = append(contentLines, metaStyle.Render("[enter] confirm  [esc] cancel"))

	// Calculate scrolling
	maxContentHeight := fixedHeight - 4 // Account for box padding/borders
	totalLines := len(contentLines)

	// Clamp scroll offset
	maxScroll := totalLines - maxContentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	scrollOffset := m.fileOpScrollOffset
	if scrollOffset > maxScroll {
		scrollOffset = maxScroll
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}

	// Build final content with scroll indicators
	var content strings.Builder

	// Scroll indicator (above)
	if scrollOffset > 0 {
		content.WriteString(metaStyle.Render("  ▲ more above"))
		content.WriteString("\n")
	}

	// Visible content
	endIdx := scrollOffset + maxContentHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}

	for i := scrollOffset; i < endIdx; i++ {
		content.WriteString(contentLines[i])
		content.WriteString("\n")
	}

	// Scroll indicator (below)
	if endIdx < totalLines {
		content.WriteString(metaStyle.Render("  ▼ more below"))
	}

	// Create the box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(1, 2).
		Width(boxWidth).
		Height(fixedHeight)

	opBox := boxStyle.Render(content.String())

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		opBox,
	)
}
