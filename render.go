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

	// Tree pane - use dynamic width from splitRatio
	leftWidth := m.leftPaneWidth()
	rightWidth := m.rightPaneWidth()
	paneHeight := m.height - 4 // header(1) + footer(1) + borders(2)

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

	// Preview pane - use dynamic width
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

	// Footer - minimal, single line
	footerStyle := lipgloss.NewStyle().Faint(true)
	footer := footerStyle.Render(" [tab] switch  [j/k] nav  [←/→] resize  [c] copy  [/] search  [g] groups  [q] quit")

	// Compose layout
	body := lipgloss.JoinHorizontal(lipgloss.Top, tree, preview)
	mainView := header + "\n" + body + "\n" + footer

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

	badgeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("205")).
		Faint(true)

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

		// Add group badges for files
		if !e.isDir {
			relPath, _ := filepath.Rel(m.rootPath, e.path)
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
