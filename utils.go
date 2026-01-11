package main

import (
	"bytes"
	"fmt"
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

const (
	maxPreviewSize  = 512 * 1024 // 512KB - files larger than this are truncated
	maxPreviewLines = 2000       // Max lines to show for large files
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

	// Check cache first
	if cached, ok := m.previewCache[e.path]; ok {
		info, err := os.Stat(e.path)
		if err == nil && info.ModTime().Equal(cached.modTime) {
			// Cache hit - use cached content
			m.preview.SetContent(cached.content)
			m.previewPath = e.path
			m.loading = false
			m.preview.GotoTop()
			return m, nil
		}
	}

	// Set loading state and trigger async load
	m.loading = true
	m.previewPath = e.path
	m.preview.SetContent("Loading...")

	// Return command that loads file content
	previewWidth := m.preview.Width
	fileName := e.name
	filePath := e.path
	return m, func() tea.Msg {
		// Get file info for cache validation and size check
		info, err := os.Stat(filePath)
		if err != nil {
			return fileLoadedMsg{path: filePath, content: "Error: " + err.Error()}
		}
		modTime := info.ModTime()

		var content []byte
		var truncated bool

		// Check file size and truncate if needed
		if info.Size() > maxPreviewSize {
			truncated = true
			// Read only first portion of large files
			f, err := os.Open(filePath)
			if err != nil {
				return fileLoadedMsg{path: filePath, content: "Error: " + err.Error()}
			}
			defer f.Close()
			content = make([]byte, maxPreviewSize)
			n, _ := f.Read(content)
			content = content[:n]
		} else {
			content, err = os.ReadFile(filePath)
			if err != nil {
				return fileLoadedMsg{path: filePath, content: "Error: " + err.Error()}
			}
		}

		// For large content, limit by lines
		text := string(content)
		if truncated || len(strings.Split(text, "\n")) > maxPreviewLines {
			lines := strings.Split(text, "\n")
			if len(lines) > maxPreviewLines {
				lines = lines[:maxPreviewLines]
				truncated = true
			}
			text = strings.Join(lines, "\n")
		}

		// Add truncation notice
		if truncated {
			text = fmt.Sprintf("--- File truncated (showing first %d lines of %s) ---\n\n%s",
				maxPreviewLines, humanSize(info.Size()), text)
		}

		// Render markdown files with glamour
		if strings.HasSuffix(fileName, ".md") {
			renderer, err := glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(previewWidth),
			)
			if err == nil {
				rendered, err := renderer.Render(text)
				if err == nil {
					return fileLoadedMsg{path: filePath, content: rendered, modTime: modTime}
				}
			}
		}

		// Syntax highlight code files with chroma
		highlighted := highlightCode(text, fileName, previewWidth)
		return fileLoadedMsg{path: filePath, content: highlighted, modTime: modTime}
	}
}

// humanSize formats bytes into a human-readable size string
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
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
