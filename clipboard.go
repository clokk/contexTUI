package main

import (
	"errors"
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

// copyRawToClipboard copies raw text to clipboard without any formatting
func copyRawToClipboard(text string) error {
	if clipboard.Unsupported {
		return ErrClipboardUnavailable
	}
	return clipboard.WriteAll(text)
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

// copyDocGroupToClipboard copies the markdown content of a doc group to clipboard
// This copies the full documentation for use as AI context
func copyDocGroupToClipboard(group DocContextGroup) error {
	if clipboard.Unsupported {
		return ErrClipboardUnavailable
	}

	// Copy the raw markdown content of the documentation file
	return clipboard.WriteAll(group.RawContent)
}
