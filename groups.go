package main

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// loadContextGroups parses .context-groups.md and returns layers, groups, and mappings
// If the file doesn't exist, it creates a default template
func loadContextGroups(rootPath string) ([]Layer, map[string][]ContextGroup, []ContextGroup, map[string][]string) {
	layers := []Layer{}
	layerGroups := make(map[string][]ContextGroup)
	groups := []ContextGroup{}
	fileToGroups := make(map[string][]string)

	groupsFile := filepath.Join(rootPath, ".context-groups.md")
	content, err := os.ReadFile(groupsFile)
	if err != nil {
		// Create default template if file doesn't exist
		if os.IsNotExist(err) {
			createDefaultContextGroupsFile(groupsFile)
		}
		return layers, layerGroups, groups, fileToGroups
	}

	lines := strings.Split(string(content), "\n")
	var currentGroup *ContextGroup
	var descLines []string
	parsingLayers := false
	passedSeparator := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect layers: block start
		if trimmed == "layers:" {
			parsingLayers = true
			continue
		}

		// Parse layer definitions (  - id: Name)
		if parsingLayers && strings.HasPrefix(trimmed, "- ") {
			layerDef := strings.TrimPrefix(trimmed, "- ")
			parts := strings.SplitN(layerDef, ":", 2)
			if len(parts) == 2 {
				layers = append(layers, Layer{
					ID:   strings.TrimSpace(parts[0]),
					Name: strings.TrimSpace(parts[1]),
				})
			}
			continue
		}

		// End of layers block (separator or new section)
		if parsingLayers && (trimmed == "---" || strings.HasPrefix(trimmed, "#")) {
			parsingLayers = false
			if trimmed == "---" {
				passedSeparator = true
				continue
			}
		}

		// New group heading (## name)
		if strings.HasPrefix(trimmed, "## ") {
			// Save previous group if exists
			if currentGroup != nil {
				currentGroup.Description = strings.TrimSpace(strings.Join(descLines, " "))
				groups = append(groups, *currentGroup)
			}
			// Start new group
			groupName := strings.TrimPrefix(trimmed, "## ")
			currentGroup = &ContextGroup{Name: groupName}
			descLines = nil
			continue
		}

		// Skip if no current group
		if currentGroup == nil {
			continue
		}

		// Parse metadata fields (layer:, parent:, tags:, contains:)
		if strings.HasPrefix(trimmed, "layer:") {
			currentGroup.Layer = strings.TrimSpace(strings.TrimPrefix(trimmed, "layer:"))
			continue
		}
		if strings.HasPrefix(trimmed, "parent:") {
			currentGroup.Parent = strings.TrimSpace(strings.TrimPrefix(trimmed, "parent:"))
			continue
		}
		if strings.HasPrefix(trimmed, "tags:") {
			tagsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "tags:"))
			currentGroup.Tags = parseListField(tagsStr)
			continue
		}
		if strings.HasPrefix(trimmed, "contains:") {
			containsStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "contains:"))
			currentGroup.Contains = parseListField(containsStr)
			continue
		}

		// File entry (- path/to/file)
		if strings.HasPrefix(trimmed, "- ") {
			filePath := strings.TrimPrefix(trimmed, "- ")
			filePath = strings.TrimSpace(filePath)
			if filePath != "" {
				currentGroup.Files = append(currentGroup.Files, filePath)
				// Add to reverse mapping
				fileToGroups[filePath] = append(fileToGroups[filePath], currentGroup.Name)
			}
			continue
		}

		// Description text (non-empty, non-heading, non-file, non-metadata lines)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "---") {
			descLines = append(descLines, trimmed)
		}
	}

	// Save last group
	if currentGroup != nil {
		currentGroup.Description = strings.TrimSpace(strings.Join(descLines, " "))
		groups = append(groups, *currentGroup)
	}

	// Build layerGroups map, defaulting to "misc" layer
	miscLayerNeeded := false
	for _, g := range groups {
		layerID := g.Layer
		if layerID == "" {
			layerID = "misc"
			miscLayerNeeded = true
		}
		layerGroups[layerID] = append(layerGroups[layerID], g)
	}

	// Add misc layer if needed and not already defined
	if miscLayerNeeded {
		hasMisc := false
		for _, l := range layers {
			if l.ID == "misc" {
				hasMisc = true
				break
			}
		}
		if !hasMisc {
			layers = append(layers, Layer{ID: "misc", Name: "Miscellaneous"})
		}
	}

	// Ignore passedSeparator warning
	_ = passedSeparator

	return layers, layerGroups, groups, fileToGroups
}

// parseListField parses "[item1, item2]" or "item1, item2" into a slice
func parseListField(s string) []string {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	result := []string{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// createDefaultContextGroupsFile creates a starter template for new projects
func createDefaultContextGroupsFile(path string) {
	template := `# Context Groups

Organize your codebase into logical groups for AI-assisted development.
This file is used by contexTUI to provide quick context loading for Claude and other AI tools.

## Format

` + "```markdown" + `
## group-name
layer: <layer-id>        # Which architectural layer (ui, feature, data, integration)
parent: <parent-group>   # Optional: nest under another group
tags: [tag1, tag2]       # Optional: cross-cutting concerns

Description of what this group contains and when to use it.

- path/to/file1.ts
- path/to/file2.tsx
` + "```" + `

## Usage

**In contexTUI:**
- Files show ` + "`[group-name]`" + ` badges in the tree view
- Press ` + "`g`" + ` to open the swimlane view (groups organized by layer)
- Navigate: ` + "`h/l`" + ` switch layers, ` + "`j/k`" + ` switch groups
- Press ` + "`enter`" + ` or ` + "`c`" + ` to copy all files as @filepath references

**For AI Agents:**
When working on a feature, copy the relevant context group to provide Claude with
the necessary file context. Groups are designed to be self-contained units of
related functionality.

**Git:** This file should be committed to your repository so team members share
the same context group definitions.

---

layers:
  - ui: UI Layer
  - feature: Feature Layer
  - data: Data Layer
  - integration: Integration Layer

---

## example
layer: feature
tags: [starter]

This is an example context group. Replace this with your own groups.
Add files that are commonly edited together or provide context for a feature.

- README.md
`
	os.WriteFile(path, []byte(template), 0644)
}

func (m model) updateGroups(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Get flat ordered list of all groups for linear navigation
	allGroups := m.getFlatGroupList()
	totalGroups := len(allGroups)

	// Convert current 2D position to flat index
	flatIndex := m.getFlatGroupIndex()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "g":
			m.showingGroups = false
			return m, nil

		case "up", "k":
			// Move up through all groups (cross layer boundaries)
			if flatIndex > 0 {
				m.setGroupFromFlatIndex(flatIndex - 1)
				m.ensureGroupSelectionVisible()
			}
			return m, nil

		case "down", "j":
			// Move down through all groups (cross layer boundaries)
			if flatIndex < totalGroups-1 {
				m.setGroupFromFlatIndex(flatIndex + 1)
				m.ensureGroupSelectionVisible()
			}
			return m, nil

		case "enter", "c":
			// Copy the selected group to clipboard
			selectedGroup := m.getSelectedGroup()
			if selectedGroup != nil {
				copyGroupToClipboard(m.rootPath, *selectedGroup)
				m.showingGroups = false
			}
			return m, nil
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Check if click is on a group line and copy it
			clickedGroup := m.getGroupAtClick(msg.X, msg.Y)
			if clickedGroup != nil {
				copyGroupToClipboard(m.rootPath, *clickedGroup)
				m.showingGroups = false
			}
		} else if msg.Button == tea.MouseButtonWheelUp {
			// Scroll view up (don't move selection)
			m.groupsScrollOffset -= 3
			if m.groupsScrollOffset < 0 {
				m.groupsScrollOffset = 0
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			// Scroll view down (don't move selection)
			m.groupsScrollOffset += 3
			// Clamp to max scroll
			maxContentHeight := m.height - 16
			if maxContentHeight < 8 {
				maxContentHeight = 8
			}
			totalLines := m.getTotalGroupLines()
			maxScroll := totalLines - maxContentHeight
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

// getGroupCountForCurrentLayer returns the number of groups in the current layer
func (m model) getGroupCountForCurrentLayer() int {
	if len(m.layers) == 0 {
		return len(m.contextGroups)
	}
	if m.layerCursor >= len(m.layers) {
		return 0
	}
	layer := m.layers[m.layerCursor]
	return len(m.getOrderedGroupsForLayer(layer.ID))
}

// getFlatGroupList returns all groups in display order (for linear navigation)
func (m model) getFlatGroupList() []ContextGroup {
	if len(m.layers) == 0 {
		return m.contextGroups
	}

	var result []ContextGroup
	for _, layer := range m.layers {
		groups := m.getOrderedGroupsForLayer(layer.ID)
		result = append(result, groups...)
	}
	return result
}

// getFlatGroupIndex converts current 2D cursor position to flat index
func (m model) getFlatGroupIndex() int {
	if len(m.layers) == 0 {
		return m.groupCursor
	}

	flatIdx := 0
	for i := 0; i < m.layerCursor; i++ {
		flatIdx += len(m.getOrderedGroupsForLayer(m.layers[i].ID))
	}
	flatIdx += m.groupCursor
	return flatIdx
}

// setGroupFromFlatIndex sets layerCursor and groupCursor from a flat index
func (m *model) setGroupFromFlatIndex(flatIdx int) {
	if len(m.layers) == 0 {
		m.groupCursor = flatIdx
		return
	}

	remaining := flatIdx
	for i, layer := range m.layers {
		groupCount := len(m.getOrderedGroupsForLayer(layer.ID))
		if remaining < groupCount {
			m.layerCursor = i
			m.groupCursor = remaining
			return
		}
		remaining -= groupCount
	}
}

// ensureGroupSelectionVisible updates groupsScrollOffset to keep the selected group visible
func (m *model) ensureGroupSelectionVisible() {
	// Calculate max content height (same as renderGroupsOverlay)
	maxContentHeight := m.height - 16
	if maxContentHeight < 8 {
		maxContentHeight = 8
	}

	// Calculate the line index of the selected item
	selectedLineIdx := m.getSelectedGroupLineIndex()
	totalLines := m.getTotalGroupLines()

	// Clamp scroll offset to valid range
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

	// Ensure selected line is visible
	if selectedLineIdx < m.groupsScrollOffset {
		m.groupsScrollOffset = selectedLineIdx
	} else if selectedLineIdx >= m.groupsScrollOffset+maxContentHeight {
		m.groupsScrollOffset = selectedLineIdx - maxContentHeight + 1
	}
}

// getSelectedGroupLineIndex returns the line index for the currently selected group
func (m model) getSelectedGroupLineIndex() int {
	lineIdx := 1 // Start at 1 (after title)

	if len(m.layers) == 0 {
		// Simple list mode: title + blank line + groups
		return lineIdx + 1 + m.groupCursor // +1 for blank line after title
	}

	// Swimlane mode - count lines through layers/groups
	flatIdx := m.getFlatGroupIndex()
	currentFlatIdx := 0

	for _, layer := range m.layers {
		groups := m.layerGroups[layer.ID]
		if len(groups) == 0 {
			continue
		}

		lineIdx += 2 // separator + layer name

		orderedGroups := m.getOrderedGroupsForLayer(layer.ID)
		for range orderedGroups {
			if currentFlatIdx == flatIdx {
				return lineIdx
			}
			lineIdx++
			currentFlatIdx++
		}
	}

	return lineIdx
}

// getTotalGroupLines returns the total number of lines in the groups overlay content
func (m model) getTotalGroupLines() int {
	lineCount := 1 // Title

	if len(m.contextGroups) == 0 {
		return lineCount + 3 // empty message lines
	}

	if len(m.layers) == 0 {
		return lineCount + 1 + len(m.contextGroups) // blank line + groups
	}

	// Swimlane mode
	for _, layer := range m.layers {
		groups := m.layerGroups[layer.ID]
		if len(groups) == 0 {
			continue
		}
		lineCount += 2 // separator + layer name
		lineCount += len(m.getOrderedGroupsForLayer(layer.ID))
	}

	return lineCount
}

// getGroupAtClick returns the group at the clicked screen position, or nil
func (m model) getGroupAtClick(x, y int) *ContextGroup {
	// Calculate overlay position (centered)
	boxWidth := 55
	boxHeight := m.height - 16 + 10 // content + fixed elements
	if boxHeight > m.height-4 {
		boxHeight = m.height - 4
	}

	boxLeft := (m.width - boxWidth) / 2
	boxTop := (m.height - boxHeight) / 2

	// Check if click is within the box
	if x < boxLeft || x > boxLeft+boxWidth || y < boxTop || y > boxTop+boxHeight {
		return nil
	}

	// Calculate relative Y position within content area
	// Account for border(1) + padding(1) + title(1) = 3 lines before content
	contentY := y - boxTop - 3

	// Build the line-to-group mapping
	lineToGroup := make(map[int]*ContextGroup)
	currentLine := 0

	if len(m.layers) == 0 {
		for i := range m.contextGroups {
			lineToGroup[currentLine] = &m.contextGroups[i]
			currentLine++
		}
	} else {
		for _, layer := range m.layers {
			groups := m.layerGroups[layer.ID]
			if len(groups) == 0 {
				continue
			}
			currentLine += 2 // separator + layer name

			orderedGroups := m.getOrderedGroupsForLayer(layer.ID)
			for i := range orderedGroups {
				lineToGroup[currentLine] = &orderedGroups[i]
				currentLine++
			}
		}
	}

	// Adjust for scroll offset
	maxContentHeight := m.height - 16
	if maxContentHeight < 8 {
		maxContentHeight = 8
	}

	flatIndex := m.getFlatGroupIndex()
	allGroups := m.getFlatGroupList()
	selectedLineIdx := 0
	currentLine = 0
	for i, g := range allGroups {
		if len(m.layers) > 0 {
			// Account for layer headers
			for _, layer := range m.layers {
				if len(m.layerGroups[layer.ID]) > 0 {
					if i == 0 || m.getLayerForGroup(allGroups[i-1].Name) != layer.ID {
						if m.getLayerForGroup(g.Name) == layer.ID {
							currentLine += 2
							break
						}
					}
				}
			}
		}
		if i == flatIndex {
			selectedLineIdx = currentLine
		}
		currentLine++
	}

	scrollOffset := 0
	totalLines := currentLine
	if totalLines > maxContentHeight {
		scrollOffset = selectedLineIdx - maxContentHeight/2
		if scrollOffset < 0 {
			scrollOffset = 0
		}
		if scrollOffset > totalLines-maxContentHeight {
			scrollOffset = totalLines - maxContentHeight
		}
	}

	// Account for scroll indicator at top
	if scrollOffset > 0 {
		contentY--
	}

	actualLine := contentY + scrollOffset
	if group, ok := lineToGroup[actualLine]; ok {
		return group
	}

	return nil
}

// getLayerForGroup returns the layer ID for a given group name
func (m model) getLayerForGroup(groupName string) string {
	for _, g := range m.contextGroups {
		if g.Name == groupName {
			return g.Layer
		}
	}
	return ""
}

// getSelectedGroup returns the currently selected group in swimlane view
func (m model) getSelectedGroup() *ContextGroup {
	if len(m.layers) == 0 {
		if m.groupCursor < len(m.contextGroups) {
			return &m.contextGroups[m.groupCursor]
		}
		return nil
	}

	if m.layerCursor >= len(m.layers) {
		return nil
	}

	layer := m.layers[m.layerCursor]

	// Build ordered list matching render order (parents first, then children)
	orderedGroups := m.getOrderedGroupsForLayer(layer.ID)
	if m.groupCursor < len(orderedGroups) {
		return &orderedGroups[m.groupCursor]
	}
	return nil
}

// getOrderedGroupsForLayer returns groups in render order (parents first, children indented)
func (m model) getOrderedGroupsForLayer(layerID string) []ContextGroup {
	groups := m.layerGroups[layerID]
	if len(groups) == 0 {
		return nil
	}

	// Build parent->children map
	childrenOf := make(map[string][]string)
	for _, g := range m.contextGroups {
		if g.Parent != "" {
			childrenOf[g.Parent] = append(childrenOf[g.Parent], g.Name)
		}
	}

	var ordered []ContextGroup
	rendered := make(map[string]bool)

	// First pass: parents and their children
	for _, group := range groups {
		if rendered[group.Name] || group.Parent != "" {
			continue
		}
		ordered = append(ordered, group)
		rendered[group.Name] = true

		// Add children
		for _, childName := range childrenOf[group.Name] {
			for _, g := range groups {
				if g.Name == childName {
					ordered = append(ordered, g)
					rendered[childName] = true
					break
				}
			}
		}
	}

	// Second pass: orphan children
	for _, group := range groups {
		if !rendered[group.Name] {
			ordered = append(ordered, group)
		}
	}

	return ordered
}
