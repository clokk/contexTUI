package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/muesli/reflow/wordwrap"
)

const (
	maxPreviewSize  = 512 * 1024 // 512KB - files larger than this are truncated
	maxPreviewLines = 2000       // Max lines to show for large files

	// Diff context sizes for progressive loading
	quickDiffContext = 10    // Quick initial load - small context
	fullDiffContext  = 99999 // Full load - entire file context
)

// UpdatePreview loads the preview for the currently selected entry
func (m Model) UpdatePreview() (Model, tea.Cmd) {
	flat := m.FlatEntries()
	if m.cursor >= len(flat) {
		return m, nil
	}

	e := flat[m.cursor]
	if e.IsDir {
		m.preview.SetContent("Directory: " + e.Name)
		m.loading = false
		return m, nil
	}

	// Check cache first
	if cached, ok := m.previewCache[e.Path]; ok {
		info, err := os.Stat(e.Path)
		if err == nil && info.ModTime().Equal(cached.ModTime) {
			// Cache hit - use cached content
			m.preview.SetContent(cached.Content)
			m.previewPath = e.Path
			m.previewLines = strings.Split(cached.Content, "\n")
			m.loading = false
			m.preview.GotoTop()
			return m, nil
		}
	}

	// Set loading state and trigger async load
	m.loading = true
	m.previewPath = e.Path
	m.preview.SetContent("Loading...")

	// Return command that loads file content
	previewWidth := m.preview.Width
	fileName := e.Name
	filePath := e.Path
	return m, func() tea.Msg {
		return LoadFileContent(filePath, fileName, previewWidth)
	}
}

// LoadFileContent loads and processes file content for preview
func LoadFileContent(filePath, fileName string, previewWidth int) FileLoadedMsg {
	// Get file info for cache validation and size check
	info, err := os.Stat(filePath)
	if err != nil {
		return FileLoadedMsg{Path: filePath, Content: "Error: " + err.Error()}
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
			return FileLoadedMsg{Path: filePath, Content: "Error: " + err.Error()}
		}
		defer f.Close()
		content = make([]byte, maxPreviewSize)
		n, _ := f.Read(content)
		content = content[:n]
	} else {
		content, err = os.ReadFile(filePath)
		if err != nil {
			return FileLoadedMsg{Path: filePath, Content: "Error: " + err.Error()}
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
				return FileLoadedMsg{Path: filePath, Content: rendered, ModTime: modTime}
			}
		}
	}

	// Syntax highlight code files with chroma
	highlighted := HighlightCode(text, fileName, previewWidth)
	return FileLoadedMsg{Path: filePath, Content: highlighted, ModTime: modTime}
}

// LoadFilePreview returns a command that loads file content asynchronously
func LoadFilePreview(e Entry, previewWidth int) tea.Cmd {
	return func() tea.Msg {
		return LoadFileContent(e.Path, e.Name, previewWidth)
	}
}

// UpdateGitStatusPreview loads the diff preview for the currently selected git change
// Uses progressive loading: quick diff first, then full diff in background
func (m Model) UpdateGitStatusPreview() (Model, tea.Cmd) {
	if m.gitStatusCursor >= len(m.gitChanges) {
		return m, nil
	}

	change := m.gitChanges[m.gitStatusCursor]
	fullPath := filepath.Join(m.gitRepoRoot, change.Path)

	// Untracked files - show file content (no diff exists)
	if change.Status == "?" {
		m.loading = true
		m.previewPath = fullPath
		m.preview.SetContent("Loading...")

		previewWidth := m.preview.Width
		fileName := filepath.Base(change.Path)
		return m, func() tea.Msg {
			return LoadFileContent(fullPath, fileName, previewWidth)
		}
	}

	// Generate unique request ID for cancellation
	m.diffRequestID = time.Now().UnixNano()
	requestID := m.diffRequestID

	// Capture values for async commands
	previewWidth := m.preview.Width
	repoRoot := m.gitRepoRoot
	staged := change.Staged
	relPath := change.Path

	// Initialize cache if needed
	if m.diffCache == nil {
		m.diffCache = make(map[DiffCacheKey]CachedDiff)
	}

	// Check cache for full diff first (best case - instant)
	fullKey := DiffCacheKey{Path: fullPath, Staged: staged, ContextSize: fullDiffContext}
	if cached, ok := m.diffCache[fullKey]; ok {
		m.preview.SetContent(cached.Content)
		m.previewPath = fullPath
		m.previewLines = strings.Split(cached.Content, "\n")
		m.loading = false
		m.preview.GotoTop()
		return m, nil
	}

	// Check cache for quick diff (show it, then load full in background)
	quickKey := DiffCacheKey{Path: fullPath, Staged: staged, ContextSize: quickDiffContext}
	if cached, ok := m.diffCache[quickKey]; ok {
		m.preview.SetContent(cached.Content)
		m.previewPath = fullPath
		m.previewLines = strings.Split(cached.Content, "\n")
		m.loading = false
		m.preview.GotoTop()

		// Trigger background full diff load
		m.fullDiffLoading = fullPath
		m.fullDiffStaged = staged
		return m, func() tea.Msg {
			return LoadFullDiff(repoRoot, relPath, staged, previewWidth, requestID)
		}
	}

	// No cache - load quick diff first
	m.loading = true
	m.previewPath = fullPath
	m.preview.SetContent("Loading...")

	return m, func() tea.Msg {
		return LoadQuickDiff(repoRoot, relPath, staged, previewWidth, requestID)
	}
}

// LoadGitDiff runs git diff and returns the diff output for a file (legacy, uses full context)
func LoadGitDiff(repoRoot, filePath string, staged bool, previewWidth int) FileLoadedMsg {
	diffText, err := git.LoadDiff(repoRoot, filePath, staged, fullDiffContext)
	if err != nil || diffText == "" {
		return FileLoadedMsg{
			Path:    filepath.Join(repoRoot, filePath),
			Content: "No diff available",
		}
	}

	// Apply diff syntax highlighting
	highlighted := HighlightDiff(diffText, previewWidth)

	return FileLoadedMsg{
		Path:    filepath.Join(repoRoot, filePath),
		Content: highlighted,
		ModTime: time.Now(),
	}
}

// LoadQuickDiff loads a diff with minimal context for fast initial display
func LoadQuickDiff(repoRoot, filePath string, staged bool, previewWidth int, requestID int64) QuickDiffLoadedMsg {
	diffText, err := git.LoadDiff(repoRoot, filePath, staged, quickDiffContext)
	if err != nil || diffText == "" {
		return QuickDiffLoadedMsg{
			Path:      filepath.Join(repoRoot, filePath),
			Content:   "No diff available",
			RequestID: requestID,
			Staged:    staged,
		}
	}

	highlighted := HighlightDiff(diffText, previewWidth)
	return QuickDiffLoadedMsg{
		Path:      filepath.Join(repoRoot, filePath),
		Content:   highlighted,
		ModTime:   time.Now(),
		RequestID: requestID,
		Staged:    staged,
	}
}

// LoadFullDiff loads a diff with complete context for seamless upgrade
func LoadFullDiff(repoRoot, filePath string, staged bool, previewWidth int, requestID int64) FullDiffLoadedMsg {
	diffText, err := git.LoadDiff(repoRoot, filePath, staged, fullDiffContext)
	if err != nil || diffText == "" {
		return FullDiffLoadedMsg{
			Path:      filepath.Join(repoRoot, filePath),
			Content:   "No diff available",
			RequestID: requestID,
			Staged:    staged,
		}
	}

	highlighted := HighlightDiff(diffText, previewWidth)
	return FullDiffLoadedMsg{
		Path:      filepath.Join(repoRoot, filePath),
		Content:   highlighted,
		ModTime:   time.Now(),
		RequestID: requestID,
		Staged:    staged,
	}
}

// HighlightCode uses chroma to syntax highlight code based on filename
func HighlightCode(code, filename string, maxWidth int) string {
	// Calculate gutter width for line number adjustment
	lineCount := strings.Count(code, "\n") + 1
	gutterWidth := len(fmt.Sprintf("%d", lineCount))
	if gutterWidth < 4 {
		gutterWidth = 4
	}
	gutterTotal := gutterWidth + 3 // number + " │ "

	// Skip highlighting for certain file types that don't benefit from it
	skipExtensions := []string{".sum", ".lock", ".txt", ".log", ".csv", ".json"}
	for _, ext := range skipExtensions {
		if strings.HasSuffix(filename, ext) {
			wrapped := wrapLines(code, maxWidth-gutterTotal)
			return addLineNumbers(wrapped)
		}
	}

	var buf bytes.Buffer

	// Use filename to detect language, "terminal256" formatter for terminal colors
	err := quick.Highlight(&buf, code, filename, "terminal256", "monokai")
	if err != nil {
		// Fall back to plain text if highlighting fails
		wrapped := wrapLines(code, maxWidth-gutterTotal)
		return addLineNumbers(wrapped)
	}

	// Word wrap highlighted output and add line numbers
	wrapped := wrapLines(buf.String(), maxWidth-gutterTotal)
	return addLineNumbers(wrapped)
}

// HighlightDiff applies syntax highlighting to git diff output
func HighlightDiff(diffText string, maxWidth int) string {
	lines := strings.Split(diffText, "\n")

	// Calculate gutter width
	gutterWidth := len(fmt.Sprintf("%d", len(lines)))
	if gutterWidth < 4 {
		gutterWidth = 4
	}
	gutterTotal := gutterWidth + 3

	// Style definitions for diff output
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("118"))    // Green
	removeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // Red
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))    // Cyan
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226")) // Yellow

	var result strings.Builder
	for i, line := range lines {
		var styled string
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			styled = headerStyle.Render(line)
		case strings.HasPrefix(line, "@@"):
			styled = hunkStyle.Render(line)
		case strings.HasPrefix(line, "+"):
			styled = addStyle.Render(line)
		case strings.HasPrefix(line, "-"):
			styled = removeStyle.Render(line)
		default:
			styled = line
		}
		result.WriteString(styled)
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	// Wrap and add line numbers
	wrapped := wrapLines(result.String(), maxWidth-gutterTotal)
	return addLineNumbers(wrapped)
}

// addLineNumbers prepends line numbers to each line of content
func addLineNumbers(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Calculate gutter width based on total lines
	gutterWidth := len(fmt.Sprintf("%d", len(lines)))
	if gutterWidth < 4 {
		gutterWidth = 4 // Minimum 4 chars for alignment
	}

	// Use lipgloss for consistent styling that won't be affected by syntax highlighting
	gutterStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var result strings.Builder
	for i, line := range lines {
		lineNum := fmt.Sprintf("%*d", gutterWidth, i+1)
		// Render the gutter (number + separator) with lipgloss
		gutter := gutterStyle.Render(lineNum + " │ ")
		result.WriteString(gutter)
		result.WriteString(line)
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}
	return result.String()
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

// wrapLines wraps text at word boundaries to fit within maxWidth
func wrapLines(content string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	// Leave buffer for padding and border
	maxWidth = maxWidth - 4

	// Use ANSI-aware word wrapping from muesli/reflow
	return wordwrap.String(content, maxWidth)
}

// StripLineNumbers removes the line number prefix from a line
// Handles format: "   5 │ code" -> "code"
func StripLineNumbers(line string) string {
	// Find the separator "│ " and return everything after it
	if idx := strings.Index(line, "│ "); idx != -1 {
		return line[idx+len("│ "):]
	}
	return line
}

// ScrollTick returns a command that sends a scroll tick after a delay
func ScrollTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return ScrollTickMsg{}
	})
}
