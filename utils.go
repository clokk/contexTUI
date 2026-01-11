package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2/quick"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/muesli/reflow/wordwrap"
	"github.com/sahilm/fuzzy"
)

func (m model) updatePreview() (model, tea.Cmd) {
	flat := m.flatEntries()
	if m.cursor >= len(flat) {
		return m, nil
	}

	e := flat[m.cursor]
	if e.isDir {
		m.preview.SetContent("Directory: " + e.name)
		m.loading = false
		return m, nil
	}

	// Set loading state and trigger async load
	m.loading = true
	m.previewPath = e.path
	m.preview.SetContent("Loading...")

	// Return command that loads file content
	previewWidth := m.preview.Width
	fileName := e.name
	return m, func() tea.Msg {
		content, err := os.ReadFile(e.path)
		if err != nil {
			return fileLoadedMsg{path: e.path, content: "Error: " + err.Error()}
		}

		// Render markdown files with glamour
		if strings.HasSuffix(fileName, ".md") {
			renderer, _ := glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(previewWidth),
			)
			rendered, err := renderer.Render(string(content))
			if err == nil {
				return fileLoadedMsg{path: e.path, content: rendered}
			}
		}

		// Syntax highlight code files with chroma
		highlighted := highlightCode(string(content), fileName, previewWidth)
		return fileLoadedMsg{path: e.path, content: highlighted}
	}
}

// highlightCode uses chroma to syntax highlight code based on filename
func highlightCode(code, filename string, maxWidth int) string {
	// Skip highlighting for certain file types that don't benefit from it
	skipExtensions := []string{".sum", ".lock", ".txt", ".log", ".csv", ".json"}
	for _, ext := range skipExtensions {
		if strings.HasSuffix(filename, ext) {
			return wrapLines(code, maxWidth)
		}
	}

	var buf bytes.Buffer

	// Use filename to detect language, "terminal256" formatter for terminal colors
	err := quick.Highlight(&buf, code, filename, "terminal256", "monokai")
	if err != nil {
		// Fall back to plain text if highlighting fails
		return wrapLines(code, maxWidth)
	}

	// Word wrap highlighted output
	return wrapLines(buf.String(), maxWidth)
}

// wrapLines wraps text at word boundaries to fit within maxWidth
// Uses ANSI-aware wrapping to handle syntax highlighting escape codes
func wrapLines(content string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	// Leave buffer for padding and border
	maxWidth = maxWidth - 4

	// Use ANSI-aware word wrapping from muesli/reflow
	return wordwrap.String(content, maxWidth)
}

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel search
			m.searching = false
			m.searchInput.Blur()
			return m, nil

		case "enter":
			// Select the current result
			if len(m.searchResults) > 0 && m.searchCursor < len(m.searchResults) {
				result := m.searchResults[m.searchCursor]
				m.searching = false
				m.searchInput.Blur()
				// Navigate to the file
				m = m.navigateToFile(result.path)
				var cmd tea.Cmd
				m, cmd = m.updatePreview()
				return m, cmd
			}
			m.searching = false
			return m, nil

		case "up", "ctrl+p":
			if m.searchCursor > 0 {
				m.searchCursor--
			}
			return m, nil

		case "down", "ctrl+n":
			if m.searchCursor < len(m.searchResults)-1 {
				m.searchCursor++
			}
			return m, nil
		}
	}

	// Update the text input
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	cmds = append(cmds, cmd)

	// Update search results based on current query
	query := m.searchInput.Value()
	if query == "" {
		m.searchResults = nil
	} else {
		matches := fuzzy.Find(query, m.allFiles)
		m.searchResults = make([]searchResult, 0, len(matches))
		for _, match := range matches {
			if len(m.searchResults) >= 10 { // Limit results
				break
			}
			m.searchResults = append(m.searchResults, searchResult{
				path:        m.allFiles[match.Index],
				displayName: m.allFiles[match.Index],
			})
		}
		// Reset cursor if it's out of bounds
		if m.searchCursor >= len(m.searchResults) {
			m.searchCursor = 0
		}
	}

	return m, tea.Batch(cmds...)
}

func copyToClipboard(path string) {
	// Use @filepath format for Claude Code context
	formatted := "@" + path

	// Use pbcopy on macOS
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(formatted)
	cmd.Run()
}

func copyGroupToClipboard(rootPath string, group ContextGroup) {
	var refs []string
	for _, relPath := range group.Files {
		fullPath := filepath.Join(rootPath, relPath)
		refs = append(refs, "@"+fullPath)
	}

	// Use pbcopy on macOS
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(strings.Join(refs, " "))
	cmd.Run()
}
