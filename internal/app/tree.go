package app

import (
	"path/filepath"
	"strings"
)

// ToggleExpand expands or collapses a directory entry
func (m Model) ToggleExpand(path string) Model {
	m.entries = toggleExpandRecursive(m.entries, path, m.rootPath, m.showDotfiles)
	m.InvalidateTreeCache()
	return m
}

func toggleExpandRecursive(entries []Entry, path, rootPath string, showDotfiles bool) []Entry {
	for i, e := range entries {
		if e.Path == path && e.IsDir {
			if e.Expanded {
				entries[i].Expanded = false
				entries[i].Children = nil
			} else {
				entries[i].Expanded = true
				entries[i].Children = LoadDirectoryWithRoot(path, rootPath, e.Depth+1, showDotfiles)
			}
			return entries
		}
		if e.Expanded && len(e.Children) > 0 {
			entries[i].Children = toggleExpandRecursive(e.Children, path, rootPath, showDotfiles)
		}
	}
	return entries
}

// Collapse collapses a directory entry
func (m Model) Collapse(path string) Model {
	m.entries = collapseRecursive(m.entries, path)
	m.InvalidateTreeCache()
	return m
}

func collapseRecursive(entries []Entry, path string) []Entry {
	for i, e := range entries {
		if e.Path == path && e.IsDir && e.Expanded {
			entries[i].Expanded = false
			entries[i].Children = nil
			return entries
		}
		if e.Expanded && len(e.Children) > 0 {
			entries[i].Children = collapseRecursive(e.Children, path)
		}
	}
	return entries
}

// NavigateToFile expands parent directories and moves cursor to a file
func (m Model) NavigateToFile(relPath string) Model {
	parts := strings.Split(relPath, string(filepath.Separator))
	currentPath := m.rootPath

	// Expand each directory in the path
	for i := 0; i < len(parts)-1; i++ {
		currentPath = filepath.Join(currentPath, parts[i])
		m.entries = expandPath(m.entries, currentPath, m.rootPath, m.showDotfiles)
	}

	// Invalidate cache since we may have expanded directories
	m.InvalidateTreeCache()

	// Find the file in the flat list and set cursor
	fullPath := filepath.Join(m.rootPath, relPath)
	flat := m.FlatEntries()
	for i, e := range flat {
		if e.Path == fullPath {
			m.cursor = i
			break
		}
	}

	return m
}

func expandPath(entries []Entry, path, rootPath string, showDotfiles bool) []Entry {
	for i, e := range entries {
		if e.Path == path && e.IsDir && !e.Expanded {
			entries[i].Expanded = true
			entries[i].Children = LoadDirectoryWithRoot(path, rootPath, e.Depth+1, showDotfiles)
			return entries
		}
		if e.Expanded && len(e.Children) > 0 {
			entries[i].Children = expandPath(e.Children, path, rootPath, showDotfiles)
		}
	}
	return entries
}
