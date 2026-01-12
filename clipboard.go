package main

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// ErrClipboardUnavailable indicates no clipboard utility was found
var ErrClipboardUnavailable = errors.New("clipboard unavailable - install xclip, xsel, or wl-clipboard")

// IsClipboardAvailable returns true if clipboard operations are supported
func IsClipboardAvailable() bool {
	return !clipboard.Unsupported
}

// copyToClipboard copies a single file path to clipboard with @ prefix
func copyToClipboard(path string) error {
	if clipboard.Unsupported {
		return ErrClipboardUnavailable
	}
	formatted := "@" + path
	return clipboard.WriteAll(formatted)
}

// copyGroupToClipboard copies multiple file paths to clipboard
// Each path is prefixed with @ and separated by spaces
func copyGroupToClipboard(rootPath string, group ContextGroup) error {
	if clipboard.Unsupported {
		return ErrClipboardUnavailable
	}

	var refs []string
	for _, relPath := range group.Files {
		fullPath := filepath.Join(rootPath, relPath)
		refs = append(refs, "@"+fullPath)
	}

	return clipboard.WriteAll(strings.Join(refs, " "))
}

// copySelection copies the selected lines from preview to clipboard
// Strips ANSI codes and line numbers before copying
func (m model) copySelection() error {
	if clipboard.Unsupported {
		return ErrClipboardUnavailable
	}

	if len(m.previewLines) == 0 || m.selectStart < 0 || m.selectEnd < 0 {
		return nil // Nothing to copy, not an error
	}

	start, end := m.selectStart, m.selectEnd
	if start > end {
		start, end = end, start
	}

	// Clamp to valid range
	if start < 0 {
		start = 0
	}
	if end >= len(m.previewLines) {
		end = len(m.previewLines) - 1
	}

	// Extract selected lines, stripping ANSI codes and line numbers
	var cleanLines []string
	for i := start; i <= end; i++ {
		line := m.previewLines[i]
		// Strip ANSI codes first
		clean := ansi.Strip(line)
		// Strip line number prefix
		clean = stripLineNumbers(clean)
		cleanLines = append(cleanLines, clean)
	}

	return clipboard.WriteAll(strings.Join(cleanLines, "\n"))
}
