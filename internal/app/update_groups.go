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

// StructureNeededTag is inserted into files that need context doc structuring
const StructureNeededTag = "<!-- contexTUI: structure-needed -->\n"

// StructuringPrompt is copied when user presses 'p' in docs overlay
const StructuringPrompt = `Find all markdown files in this project containing the comment:
<!-- contexTUI: structure-needed -->

For each file, read .context-docs.md to understand the required structure,
then update the file to include:
- **Category:** Use your judgment based on the project. Defaults are "Meta" (project-level/architecture docs) and "Feature" (feature-specific docs). If a different category better fits the content (e.g., "API", "Infrastructure", "Testing"), suggest it, explain your reasoning, and ask the user to confirm before applying.
- **Status:** Active
- ## Description section
- ## Key Files section (IMPORTANT: must use list format, not tables)

Key Files format (required):
` + "```" + `
## Key Files

- path/to/file.ts - Description of this entry point
- another/file.go - Another key file
` + "```" + `

Each entry must start with "- " followed by the file path. Description after " - " is optional.
Tables are NOT supported for Key Files.

Remove the <!-- contexTUI: structure-needed --> tag after structuring.`

// updateDocs handles the context docs overlay
func (m Model) updateDocs(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle add doc mode separately
	if m.addingDoc {
		return m.updateAddDoc(msg)
	}

	// Get docs for current category
	currentDocs := m.getDocsForSelectedCategory()
	totalDocs := len(currentDocs)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Save immediately if dirty before closing
			if m.registryDirty && !m.registrySaving {
				groups.SaveContextDocRegistry(m.rootPath, m.docRegistry)
				m.registryDirty = false
			}
			m.showingDocs = false
			return m, nil

		case "left", "h":
			// Previous category
			if m.docRegistry != nil && len(m.docRegistry.Categories) > 0 {
				m.selectedCategory--
				if m.selectedCategory < 0 {
					m.selectedCategory = len(m.docRegistry.Categories) - 1
				}
				m.docCursor = 0
				m.docsScrollOffset = 0
			}
			return m, nil

		case "right", "l":
			// Next category
			if m.docRegistry != nil && len(m.docRegistry.Categories) > 0 {
				m.selectedCategory++
				if m.selectedCategory >= len(m.docRegistry.Categories) {
					m.selectedCategory = 0
				}
				m.docCursor = 0
				m.docsScrollOffset = 0
			}
			return m, nil

		case "up", "k":
			if m.docCursor > 0 {
				m.docCursor--
				m.ensureDocVisible()
			}
			return m, nil

		case "down", "j":
			if m.docCursor < totalDocs-1 {
				m.docCursor++
				m.ensureDocVisible()
			}
			return m, nil

		case "K", "shift+up":
			// Move doc up in category
			if m.docCursor > 0 {
				m.moveDocInCategory(m.docCursor, m.docCursor-1)
				m.docCursor--
				m.ensureDocVisible()
				m.registryDirty = true
				m.statusMessage = "Moved doc up"
				m.statusMessageTime = time.Now()
				// Schedule debounced save (150ms delay)
				return m, ScheduleRegistrySave(150 * time.Millisecond)
			}
			return m, nil

		case "J", "shift+down":
			// Move doc down in category
			if m.docCursor < totalDocs-1 {
				m.moveDocInCategory(m.docCursor, m.docCursor+1)
				m.docCursor++
				m.ensureDocVisible()
				m.registryDirty = true
				m.statusMessage = "Moved doc down"
				m.statusMessageTime = time.Now()
				// Schedule debounced save (150ms delay)
				return m, ScheduleRegistrySave(150 * time.Millisecond)
			}
			return m, nil

		case "enter", "c":
			// Copy selected docs (or current if none selected) as @filepath references
			if len(m.selectedDocs) > 0 {
				// Copy all selected docs - iterate directly over selectedDocs map
				var refs []string
				for path := range m.selectedDocs {
					refs = append(refs, "@"+path)
				}
				combined := strings.Join(refs, "\n")
				if err := clipboard.CopyRaw(combined); err != nil {
					m.statusMessage = "Clipboard unavailable"
				} else {
					m.statusMessage = fmt.Sprintf("Copied %d references", len(refs))
				}
				// Clear selections after copy
				m.selectedDocs = make(map[string]bool)
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			} else if m.docCursor < totalDocs {
				// Copy single current doc as @filepath reference
				doc := currentDocs[m.docCursor]
				if err := clipboard.CopyFilePath(doc.FilePath); err != nil {
					m.statusMessage = "Clipboard unavailable"
				} else {
					m.statusMessage = fmt.Sprintf("Copied: @%s", doc.FilePath)
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
				for _, d := range m.docRegistry.Docs {
					existingPaths[d.FilePath] = true
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
			m.addDocCursor = 0
			m.addDocScroll = 0
			m.addingDoc = true
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
			// Remove the selected doc from registry
			if m.docCursor < totalDocs && m.docRegistry != nil {
				doc := currentDocs[m.docCursor]

				// Remove from Docs slice
				for i, d := range m.docRegistry.Docs {
					if d.FilePath == doc.FilePath {
						m.docRegistry.Docs = append(m.docRegistry.Docs[:i], m.docRegistry.Docs[i+1:]...)
						break
					}
				}

				// Remove from ByCategory map
				catID := strings.ToLower(strings.ReplaceAll(doc.Category, " ", "-"))
				if catID == "" {
					catID = "uncategorized"
				}
				catDocs := m.docRegistry.ByCategory[catID]
				for i, d := range catDocs {
					if d.FilePath == doc.FilePath {
						m.docRegistry.ByCategory[catID] = append(catDocs[:i], catDocs[i+1:]...)
						break
					}
				}

				// If uncategorized is now empty, remove it from Categories list
				if catID == "uncategorized" && len(m.docRegistry.ByCategory["uncategorized"]) == 0 {
					for i, cat := range m.docRegistry.Categories {
						if cat.ID == "uncategorized" {
							m.docRegistry.Categories = append(m.docRegistry.Categories[:i], m.docRegistry.Categories[i+1:]...)
							// Adjust selected category if it was pointing to the removed one
							if m.selectedCategory >= len(m.docRegistry.Categories) {
								m.selectedCategory = len(m.docRegistry.Categories) - 1
							}
							if m.selectedCategory < 0 {
								m.selectedCategory = 0
							}
							break
						}
					}
				}

				// Adjust cursor if needed
				if m.docCursor >= len(m.docRegistry.ByCategory[catID]) {
					m.docCursor = len(m.docRegistry.ByCategory[catID]) - 1
				}
				if m.docCursor < 0 {
					m.docCursor = 0
				}

				// Strip contexTUI metadata from the markdown file
				stripContextDocMetadata(m.rootPath, doc.FilePath)

				// Save registry
				if err := groups.SaveContextDocRegistry(m.rootPath, m.docRegistry); err != nil {
					m.statusMessage = fmt.Sprintf("Error: %v", err)
				} else {
					m.statusMessage = fmt.Sprintf("Removed %s", doc.Name)
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			}
			return m, nil

		case " ":
			// Toggle selection of current doc for multi-copy
			if m.docCursor < totalDocs {
				doc := currentDocs[m.docCursor]
				if m.selectedDocs[doc.FilePath] {
					delete(m.selectedDocs, doc.FilePath)
					m.statusMessage = fmt.Sprintf("Deselected (%d total)", len(m.selectedDocs))
				} else {
					m.selectedDocs[doc.FilePath] = true
					m.statusMessage = fmt.Sprintf("Selected (%d total)", len(m.selectedDocs))
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
				// Go to previous category
				m.selectedCategory--
				if m.selectedCategory < 0 {
					m.selectedCategory = len(m.docRegistry.Categories) - 1
				}
				m.docCursor = 0
				m.docsScrollOffset = 0
				return m, nil
			} else if navClick == navClickNext && m.docRegistry != nil {
				// Go to next category
				m.selectedCategory++
				if m.selectedCategory >= len(m.docRegistry.Categories) {
					m.selectedCategory = 0
				}
				m.docCursor = 0
				m.docsScrollOffset = 0
				return m, nil
			}

			// Try to find which card was clicked
			clickedIdx := m.findClickedDoc(msg.X, msg.Y)
			if clickedIdx >= 0 && clickedIdx < totalDocs {
				// Move cursor to clicked item
				m.docCursor = clickedIdx
				m.ensureDocVisible()

				// If multi-select is active, copy all selected
				if len(m.selectedDocs) > 0 {
					var refs []string
					for path := range m.selectedDocs {
						refs = append(refs, "@"+path)
					}
					combined := strings.Join(refs, "\n")
					if err := clipboard.CopyRaw(combined); err != nil {
						m.statusMessage = "Clipboard unavailable"
					} else {
						m.statusMessage = fmt.Sprintf("Copied %d references", len(refs))
					}
					m.selectedDocs = make(map[string]bool)
				} else {
					// Copy the clicked doc as @filepath reference
					doc := currentDocs[clickedIdx]
					if err := clipboard.CopyFilePath(doc.FilePath); err != nil {
						m.statusMessage = "Clipboard unavailable"
					} else {
						m.statusMessage = fmt.Sprintf("Copied: @%s", doc.FilePath)
					}
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(5 * time.Second)
			}
		} else if msg.Button == tea.MouseButtonWheelUp {
			m.docsScrollOffset -= 3 // Scroll 3 lines at a time for smoother scrolling
			if m.docsScrollOffset < 0 {
				m.docsScrollOffset = 0
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.docsScrollOffset += 3 // Scroll 3 lines at a time for smoother scrolling
			// Calculate max scroll based on card layout (consistent with view.go)
			maxContentHeight := m.height - 19 // Same as ensureDocVisible
			if maxContentHeight < 5 {
				maxContentHeight = 5
			}
			totalLines := m.estimateDocsLineCount()
			maxScroll := totalLines - maxContentHeight
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.docsScrollOffset > maxScroll {
				m.docsScrollOffset = maxScroll
			}
			return m, nil
		}
	}
	return m, nil
}

// ensureDocVisible ensures the selected doc is visible
func (m *Model) ensureDocVisible() {
	if m.docRegistry == nil {
		return
	}

	docs := m.getDocsForSelectedCategory()
	if len(docs) == 0 || m.docCursor < 0 || m.docCursor >= len(docs) {
		return
	}

	// Calculate visible content height (must match view.go calculation)
	// Box overhead: 8, header: ~6, footer: ~3, scroll indicators: ~2
	maxContentHeight := m.height - 19
	if maxContentHeight < 5 {
		maxContentHeight = 5
	}

	// Get the line range for the selected card
	lineIdx := m.getDocLineIndex(m.docCursor)
	cardHeight := m.getCardHeight(docs[m.docCursor])
	cardEndLine := lineIdx + cardHeight

	totalLines := m.getDocTotalLines()

	// Clamp scroll offset to valid range
	maxScroll := totalLines - maxContentHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.docsScrollOffset > maxScroll {
		m.docsScrollOffset = maxScroll
	}
	if m.docsScrollOffset < 0 {
		m.docsScrollOffset = 0
	}

	// Ensure the entire card is visible
	if lineIdx < m.docsScrollOffset {
		// Card starts above viewport - scroll up to show it
		m.docsScrollOffset = lineIdx
	} else if cardEndLine > m.docsScrollOffset+maxContentHeight {
		// Card ends below viewport - scroll down to show it
		m.docsScrollOffset = cardEndLine - maxContentHeight
		if m.docsScrollOffset < 0 {
			m.docsScrollOffset = 0
		}
	}
}

// getDocLineIndex returns the line index for a given doc index within the current category
// In multi-column mode, this returns the row position within the doc's column
func (m Model) getDocLineIndex(docIdx int) int {
	docs := m.getDocsForSelectedCategory()
	if len(docs) == 0 || docIdx < 0 || docIdx >= len(docs) {
		return 0
	}

	numCols := m.getDocsColumnCount()
	if numCols == 1 {
		// Single column: simple cumulative height
		lineIdx := 0
		for i := 0; i < docIdx; i++ {
			lineIdx += m.getCardHeight(docs[i])
		}
		return lineIdx
	}

	// Multi-column: calculate position within column
	docsPerCol := (len(docs) + numCols - 1) / numCols
	colIdx := docIdx / docsPerCol
	posInCol := docIdx % docsPerCol

	// Calculate line index within this column
	lineIdx := 0
	startDocIdx := colIdx * docsPerCol
	for i := startDocIdx; i < startDocIdx+posInCol && i < len(docs); i++ {
		lineIdx += m.getCardHeight(docs[i])
	}

	return lineIdx
}

// getCardHeight returns the rendered height of a doc card in lines
func (m Model) getCardHeight(doc groups.ContextDoc) int {
	// Card structure: border top (1) + title (1) + filepath (1) + desc (1-3) + meta (0-1) + border bottom (1)
	cardLines := 4 // borders (2) + title (1) + filepath (1)

	if doc.Description != "" {
		// Estimate wrapped description lines (max 3)
		descLen := len(doc.Description)
		descLines := (descLen / 60) + 1
		if descLines > 3 {
			descLines = 3
		}
		cardLines += descLines
	}

	// Meta line (key files + token estimate)
	if len(doc.KeyFiles) > 0 || doc.TokenEstimate > 0 {
		cardLines++
	}

	return cardLines
}

// getDocTotalLines returns total lines in the docs overlay
func (m Model) getDocTotalLines() int {
	return m.estimateDocsLineCount()
}

// estimateDocsLineCount estimates total scrollable content height (cards only, header is sticky)
// In multi-column mode, returns the height of the tallest column
func (m Model) estimateDocsLineCount() int {
	docs := m.getDocsForSelectedCategory()

	if len(docs) == 0 {
		return 3 // Empty message takes a few lines
	}

	numCols := m.getDocsColumnCount()
	if numCols == 1 {
		// Single column: sum all card heights
		lineCount := 0
		for _, doc := range docs {
			lineCount += m.getCardHeight(doc)
		}
		return lineCount
	}

	// Multi-column: calculate height of each column, return the max
	docsPerCol := (len(docs) + numCols - 1) / numCols
	maxColHeight := 0

	for colIdx := 0; colIdx < numCols; colIdx++ {
		colHeight := 0
		startIdx := colIdx * docsPerCol
		endIdx := startIdx + docsPerCol
		if endIdx > len(docs) {
			endIdx = len(docs)
		}
		for i := startIdx; i < endIdx; i++ {
			colHeight += m.getCardHeight(docs[i])
		}
		if colHeight > maxColHeight {
			maxColHeight = colHeight
		}
	}

	return maxColHeight
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
	if m.docRegistry == nil || len(m.docRegistry.Categories) == 0 {
		return navClickNone
	}

	// Box dimensions (must match view.go - now uses dynamic width based on columns)
	cardWidth := 68
	numCols := m.getDocsColumnCount()
	contentWidth := (cardWidth+4)*numCols + (numCols-1)*2
	boxWidth := contentWidth + 8
	boxLeft := (m.width - boxWidth) / 2

	// Fixed height calculation (must match view.go)
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

// findClickedDoc returns the index of the doc at the click position, or -1
// Supports multi-column layout
func (m Model) findClickedDoc(clickX, clickY int) int {
	docs := m.getDocsForSelectedCategory()
	if len(docs) == 0 {
		return -1
	}

	// Box dimensions (must match view.go - now uses dynamic width based on columns)
	cardWidth := 68
	numCols := m.getDocsColumnCount()
	contentWidth := (cardWidth+4)*numCols + (numCols-1)*2
	boxWidth := contentWidth + 8

	// Calculate box position (centered)
	boxLeft := (m.width - boxWidth) / 2
	boxRight := boxLeft + boxWidth

	// Check X bounds
	if clickX < boxLeft || clickX > boxRight {
		return -1
	}

	// Fixed height calculation (must match view.go)
	fixedHeight := m.height - 6
	if fixedHeight < 15 {
		fixedHeight = 15
	}
	boxTop := (m.height - fixedHeight) / 2

	// Header: title(1) + blank(1) + nav bar(1) + blank(1) + separator(1) + blank(1) = 6 lines
	headerLines := 6

	// Calculate content Y position (within scrollable area)
	// From click Y: subtract boxTop, border(1), padding(1)
	contentY := clickY - boxTop - 2 // 2 = border + padding

	// Account for header
	contentY -= headerLines

	// Account for "more above" indicator
	scrollOffset := m.docsScrollOffset
	if scrollOffset > 0 {
		contentY-- // first visible line is "more above"
	}

	if contentY < 0 {
		return -1
	}

	// The clicked line in the content area (accounting for scroll)
	clickedLineIdx := scrollOffset + contentY

	if numCols == 1 {
		// Single column: simple line-to-doc mapping
		currentLine := 0
		for i, doc := range docs {
			cardHeight := m.getCardHeight(doc)
			if clickedLineIdx < currentLine+cardHeight {
				return i
			}
			currentLine += cardHeight
		}
		return -1
	}

	// Multi-column: determine which column was clicked
	// Content starts at boxLeft + border(1) + padding(2) = boxLeft + 3
	contentLeft := boxLeft + 3
	relativeX := clickX - contentLeft

	// Each column is cardWidth+4 wide, with 2-char gap between
	colWidth := cardWidth + 4
	colGap := 2
	fullColWidth := colWidth + colGap

	clickedCol := relativeX / fullColWidth
	if clickedCol >= numCols {
		clickedCol = numCols - 1
	}
	if clickedCol < 0 {
		clickedCol = 0
	}

	// Check if click is in the gap between columns
	posInCol := relativeX % fullColWidth
	if posInCol >= colWidth {
		return -1 // Clicked in the gap
	}

	// Calculate docs per column
	docsPerCol := (len(docs) + numCols - 1) / numCols

	// Find which doc within this column based on Y position
	startDocIdx := clickedCol * docsPerCol
	endDocIdx := startDocIdx + docsPerCol
	if endDocIdx > len(docs) {
		endDocIdx = len(docs)
	}

	// Calculate line positions within this column
	currentLine := 0
	for i := startDocIdx; i < endDocIdx; i++ {
		cardHeight := m.getCardHeight(docs[i])
		if clickedLineIdx < currentLine+cardHeight {
			return i
		}
		currentLine += cardHeight
	}

	return -1
}

// updateAddDoc handles the add doc picker
func (m Model) updateAddDoc(msg tea.Msg) (tea.Model, tea.Cmd) {
	totalFiles := len(m.availableMdFiles)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.addingDoc = false
			m.selectedAddFiles = make(map[string]bool) // Clear selections
			return m, nil

		case "up", "k":
			if m.addDocCursor > 0 {
				m.addDocCursor--
				m.ensureAddDocVisible()
			}
			return m, nil

		case "down", "j":
			if m.addDocCursor < totalFiles-1 {
				m.addDocCursor++
				m.ensureAddDocVisible()
			}
			return m, nil

		case " ":
			// Toggle selection of current file for multi-add
			if m.addDocCursor < totalFiles {
				filePath := m.availableMdFiles[m.addDocCursor]
				if m.selectedAddFiles[filePath] {
					delete(m.selectedAddFiles, filePath)
					m.statusMessage = fmt.Sprintf("Deselected (%d total)", len(m.selectedAddFiles))
				} else {
					m.selectedAddFiles[filePath] = true
					m.statusMessage = fmt.Sprintf("Selected (%d total)", len(m.selectedAddFiles))
				}
				m.statusMessageTime = time.Now()
				return m, ClearStatusAfter(2 * time.Second)
			}
			return m, nil

		case "enter":
			// Determine which files to add
			var filesToAdd []string
			if len(m.selectedAddFiles) > 0 {
				// Add all selected files
				for path := range m.selectedAddFiles {
					filesToAdd = append(filesToAdd, path)
				}
			} else if m.addDocCursor < totalFiles {
				// Add single file at cursor
				filesToAdd = []string{m.availableMdFiles[m.addDocCursor]}
			}

			if len(filesToAdd) == 0 {
				return m, nil
			}

			// Initialize registry if needed
			if m.docRegistry == nil {
				m.docRegistry = &groups.ContextDocRegistry{
					Categories: groups.DefaultCategories(),
					Docs:       []groups.ContextDoc{},
					ByCategory: make(map[string][]groups.ContextDoc),
				}
			}

			// Add each file
			addedCount := 0
			incompleteCount := 0
			var lastError error
			for _, selectedPath := range filesToAdd {
				// Parse the doc
				doc, err := groups.ParseContextDoc(m.rootPath, selectedPath)
				if err != nil {
					lastError = err
					continue
				}

				// Validate and check staleness
				doc.ValidateKeyFiles(m.rootPath)
				doc.CheckStaleness(m.rootPath)

				// If file is missing required structure, insert tag
				if len(doc.MissingFields) > 0 {
					insertStructureTag(m.rootPath, selectedPath)
					incompleteCount++
				}

				// Add to registry
				m.docRegistry.Docs = append(m.docRegistry.Docs, *doc)

				// Update ByCategory map
				catID := strings.ToLower(strings.ReplaceAll(doc.Category, " ", "-"))
				if catID == "" {
					catID = "uncategorized"
				}
				m.docRegistry.ByCategory[catID] = append(m.docRegistry.ByCategory[catID], *doc)

				// If adding to uncategorized, ensure category exists in list
				if catID == "uncategorized" {
					hasUncategorized := false
					for _, cat := range m.docRegistry.Categories {
						if cat.ID == "uncategorized" {
							hasUncategorized = true
							break
						}
					}
					if !hasUncategorized {
						m.docRegistry.Categories = append([]groups.Category{{ID: "uncategorized", Name: "Uncategorized"}}, m.docRegistry.Categories...)
					}
				}
				addedCount++
			}

			// Save registry
			if err := groups.SaveContextDocRegistry(m.rootPath, m.docRegistry); err != nil {
				m.statusMessage = fmt.Sprintf("Error saving: %v", err)
			} else if addedCount == 0 && lastError != nil {
				m.statusMessage = fmt.Sprintf("Error: %v", lastError)
			} else if incompleteCount > 0 {
				m.statusMessage = fmt.Sprintf("Added %d (%d incomplete)! Press 'p' for structuring prompt", addedCount, incompleteCount)
			} else {
				m.statusMessage = fmt.Sprintf("Added %d doc(s)!", addedCount)
			}

			// Clear selections
			m.selectedAddFiles = make(map[string]bool)
			m.statusMessageTime = time.Now()
			m.addingDoc = false
			return m, ClearStatusAfter(5 * time.Second)
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Find clicked file and add it
			clickedIdx := m.findClickedAddDocFile(msg.X, msg.Y)
			if clickedIdx >= 0 && clickedIdx < totalFiles {
				selectedPath := m.availableMdFiles[clickedIdx]

				// Initialize registry if needed
				if m.docRegistry == nil {
					m.docRegistry = &groups.ContextDocRegistry{
						Categories: groups.DefaultCategories(),
						Docs:       []groups.ContextDoc{},
						ByCategory: make(map[string][]groups.ContextDoc),
					}
				}

				// Parse the doc
				doc, err := groups.ParseContextDoc(m.rootPath, selectedPath)
				if err != nil {
					m.statusMessage = fmt.Sprintf("Error: %v", err)
					m.statusMessageTime = time.Now()
					m.addingDoc = false
					return m, ClearStatusAfter(5 * time.Second)
				}

				// Validate and check staleness
				doc.ValidateKeyFiles(m.rootPath)
				doc.CheckStaleness(m.rootPath)

				// If file is missing required structure, insert tag
				if len(doc.MissingFields) > 0 {
					insertStructureTag(m.rootPath, selectedPath)
				}

				// Add to registry
				m.docRegistry.Docs = append(m.docRegistry.Docs, *doc)

				// Update ByCategory map
				catID := strings.ToLower(strings.ReplaceAll(doc.Category, " ", "-"))
				if catID == "" {
					catID = "uncategorized"
				}
				m.docRegistry.ByCategory[catID] = append(m.docRegistry.ByCategory[catID], *doc)

				// If adding to uncategorized, ensure category exists in list
				if catID == "uncategorized" {
					hasUncategorized := false
					for _, cat := range m.docRegistry.Categories {
						if cat.ID == "uncategorized" {
							hasUncategorized = true
							break
						}
					}
					if !hasUncategorized {
						m.docRegistry.Categories = append([]groups.Category{{ID: "uncategorized", Name: "Uncategorized"}}, m.docRegistry.Categories...)
					}
				}

				// Save registry
				if err := groups.SaveContextDocRegistry(m.rootPath, m.docRegistry); err != nil {
					m.statusMessage = fmt.Sprintf("Error saving: %v", err)
				} else if len(doc.MissingFields) > 0 {
					m.statusMessage = "Added (incomplete)! Press 'p' for structuring prompt"
				} else {
					m.statusMessage = fmt.Sprintf("Added %s!", doc.Name)
				}

				// Clear selections
				m.selectedAddFiles = make(map[string]bool)
				m.statusMessageTime = time.Now()
				m.addingDoc = false
				return m, ClearStatusAfter(5 * time.Second)
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelUp {
			m.addDocScroll--
			if m.addDocScroll < 0 {
				m.addDocScroll = 0
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.addDocScroll++
			maxScroll := totalFiles - (m.height - 12)
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.addDocScroll > maxScroll {
				m.addDocScroll = maxScroll
			}
			return m, nil
		}
	}
	return m, nil
}

// ensureAddDocVisible keeps the cursor visible in add doc picker
func (m *Model) ensureAddDocVisible() {
	maxHeight := m.height - 12
	if maxHeight < 5 {
		maxHeight = 5
	}

	if m.addDocCursor < m.addDocScroll {
		m.addDocScroll = m.addDocCursor
	} else if m.addDocCursor >= m.addDocScroll+maxHeight {
		m.addDocScroll = m.addDocCursor - maxHeight + 1
	}
}

// findClickedAddDocFile finds which file was clicked in the add doc overlay
func (m Model) findClickedAddDocFile(clickX, clickY int) int {
	totalFiles := len(m.availableMdFiles)
	if totalFiles == 0 {
		return -1
	}

	// Box dimensions (must match view.go renderAddDocOverlay)
	boxWidth := 70 + 6 // width + padding*2 + border*2

	// Calculate box position (centered)
	boxLeft := (m.width - boxWidth) / 2
	boxRight := boxLeft + boxWidth

	// Check X bounds
	if clickX < boxLeft || clickX > boxRight {
		return -1
	}

	// Calculate visible content (must match view.go logic)
	totalLines := 4 + totalFiles // 4 header lines + file lines
	maxHeight := m.height - 12
	if maxHeight < 5 {
		maxHeight = 5
	}

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

	endIdx := scrollOffset + maxHeight
	if endIdx > totalLines {
		endIdx = totalLines
	}
	visibleLines := endIdx - scrollOffset

	// Calculate actual content height
	// Content: more_above? + visible lines + more_below? + newline + footer
	contentLines := visibleLines + 2 // +2 for blank line and footer
	hasMoreAbove := scrollOffset > 0
	hasMoreBelow := endIdx < totalLines
	if hasMoreAbove {
		contentLines++
	}
	if hasMoreBelow {
		contentLines++
	}

	// Box height = content + border(2) + padding(2)
	boxHeight := contentLines + 4
	boxTop := (m.height - boxHeight) / 2

	// Click position in content area (after border + top padding)
	contentY := clickY - boxTop - 2

	if contentY < 0 {
		return -1
	}

	// Account for "more above" indicator
	if hasMoreAbove {
		if contentY == 0 {
			return -1 // clicked on "more above"
		}
		contentY-- // adjust for more_above indicator
	}

	// contentY is now position in the visible lines array portion
	// lines[scrollOffset + contentY] is the clicked line
	lineIdx := scrollOffset + contentY

	// Header is lines 0-3, files start at line 4
	fileIdx := lineIdx - 4

	if fileIdx < 0 || fileIdx >= totalFiles {
		return -1
	}

	return fileIdx
}

// getDocsForSelectedCategory returns docs for the currently selected category
func (m Model) getDocsForSelectedCategory() []groups.ContextDoc {
	if m.docRegistry == nil || len(m.docRegistry.Categories) == 0 {
		return nil
	}

	// Clamp selected category
	catIdx := m.selectedCategory
	if catIdx < 0 {
		catIdx = 0
	}
	if catIdx >= len(m.docRegistry.Categories) {
		catIdx = len(m.docRegistry.Categories) - 1
	}

	cat := m.docRegistry.Categories[catIdx]
	return m.docRegistry.ByCategory[cat.ID]
}

// getSelectedCategoryName returns the name of the currently selected category
func (m Model) getSelectedCategoryName() string {
	if m.docRegistry == nil || len(m.docRegistry.Categories) == 0 {
		return ""
	}

	catIdx := m.selectedCategory
	if catIdx < 0 {
		catIdx = 0
	}
	if catIdx >= len(m.docRegistry.Categories) {
		catIdx = len(m.docRegistry.Categories) - 1
	}

	return m.docRegistry.Categories[catIdx].Name
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

// stripContextDocMetadata removes contexTUI-specific metadata from a markdown file
func stripContextDocMetadata(rootPath, filePath string) error {
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
		if strings.HasPrefix(trimmed, "**Category:**") ||
			strings.HasPrefix(trimmed, "**Status:**") ||
			strings.HasPrefix(trimmed, "**Related:**") ||
			strings.Contains(trimmed, "<!-- contexTUI: structure-needed -->") {
			continue
		}
		newLines = append(newLines, line)
	}

	return os.WriteFile(fullPath, []byte(strings.Join(newLines, "\n")), 0644)
}

// moveDocInCategory swaps two docs within the current category
func (m *Model) moveDocInCategory(fromIdx, toIdx int) {
	if m.docRegistry == nil || len(m.docRegistry.Categories) == 0 {
		return
	}

	// Get current category
	catIdx := m.selectedCategory
	if catIdx < 0 || catIdx >= len(m.docRegistry.Categories) {
		return
	}
	cat := m.docRegistry.Categories[catIdx]
	docs := m.docRegistry.ByCategory[cat.ID]

	// Bounds check
	if fromIdx < 0 || fromIdx >= len(docs) || toIdx < 0 || toIdx >= len(docs) {
		return
	}

	// Get the file paths before swapping
	fromPath := docs[fromIdx].FilePath
	toPath := docs[toIdx].FilePath

	// Swap in ByCategory
	docs[fromIdx], docs[toIdx] = docs[toIdx], docs[fromIdx]
	m.docRegistry.ByCategory[cat.ID] = docs

	// Also swap in the master Docs slice so it persists when saved
	var fromGlobalIdx, toGlobalIdx int = -1, -1
	for i, d := range m.docRegistry.Docs {
		if d.FilePath == fromPath {
			fromGlobalIdx = i
		}
		if d.FilePath == toPath {
			toGlobalIdx = i
		}
		if fromGlobalIdx >= 0 && toGlobalIdx >= 0 {
			break
		}
	}

	if fromGlobalIdx >= 0 && toGlobalIdx >= 0 {
		m.docRegistry.Docs[fromGlobalIdx], m.docRegistry.Docs[toGlobalIdx] =
			m.docRegistry.Docs[toGlobalIdx], m.docRegistry.Docs[fromGlobalIdx]
	}
}

// saveRegistryAsync returns a command that saves the registry in the background
func (m *Model) saveRegistryAsync() tea.Cmd {
	registry := m.docRegistry // Capture current state
	rootPath := m.rootPath

	return func() tea.Msg {
		err := groups.SaveContextDocRegistry(rootPath, registry)
		return RegistrySavedMsg{Err: err}
	}
}
