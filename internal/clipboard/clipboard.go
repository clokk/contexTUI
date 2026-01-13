package clipboard

import (
	"errors"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// ErrUnavailable indicates no clipboard utility was found
var ErrUnavailable = errors.New("clipboard unavailable - install xclip, xsel, or wl-clipboard")

// IsAvailable returns true if clipboard operations are supported
func IsAvailable() bool {
	return !clipboard.Unsupported
}

// CopyFilePath copies a single file path to clipboard with @ prefix
func CopyFilePath(path string) error {
	if clipboard.Unsupported {
		return ErrUnavailable
	}
	formatted := "@" + path
	return clipboard.WriteAll(formatted)
}

// CopyRaw copies raw text to clipboard without any formatting
func CopyRaw(text string) error {
	if clipboard.Unsupported {
		return ErrUnavailable
	}
	return clipboard.WriteAll(text)
}

// CopyLines copies lines from a slice, stripping ANSI codes and line numbers
// start and end are inclusive indices
func CopyLines(lines []string, start, end int, stripLineNumbers func(string) string) error {
	if clipboard.Unsupported {
		return ErrUnavailable
	}

	if len(lines) == 0 || start < 0 || end < 0 {
		return nil // Nothing to copy, not an error
	}

	if start > end {
		start, end = end, start
	}

	// Clamp to valid range
	if start < 0 {
		start = 0
	}
	if end >= len(lines) {
		end = len(lines) - 1
	}

	// Extract selected lines, stripping ANSI codes and line numbers
	var cleanLines []string
	for i := start; i <= end; i++ {
		line := lines[i]
		// Strip ANSI codes first
		clean := ansi.Strip(line)
		// Strip line number prefix if function provided
		if stripLineNumbers != nil {
			clean = stripLineNumbers(clean)
		}
		cleanLines = append(cleanLines, clean)
	}

	return clipboard.WriteAll(strings.Join(cleanLines, "\n"))
}
