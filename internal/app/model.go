package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/config"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/connorleisz/contexTUI/internal/groups"
	"github.com/fsnotify/fsnotify"
)

// NewModel creates and initializes a new application model
func NewModel(rootPath string) Model {
	absPath, _ := filepath.Abs(rootPath)

	// Load user config
	cfg := config.Load(absPath)

	// Determine split ratio (config or default)
	splitRatio := 0.5
	if cfg.SplitRatio >= 0.2 && cfg.SplitRatio <= 0.8 {
		splitRatio = cfg.SplitRatio
	}

	entries := LoadDirectory(absPath, 0)

	// Set up search input
	ti := textinput.New()
	ti.Placeholder = "Search files..."
	ti.CharLimit = 100
	ti.Width = 40

	// Collect all files for searching
	allFiles := CollectAllFiles(absPath)

	// Load doc-based context docs
	docRegistry, _ := groups.LoadContextDocRegistry(absPath)

	// Check for git repository and load git status
	isGit, gitRoot := git.IsRepo(absPath)
	var gitStatus map[string]git.FileStatus
	var gitDirStatus map[string]string
	var gitChanges []git.FileStatus
	var gitBranch string
	var gitAhead, gitBehind int
	var gitHasUpstream bool
	if isGit {
		gitStatus, gitChanges = git.LoadStatus(gitRoot)
		gitDirStatus = git.ComputeDirStatus(gitStatus)
		gitBranch = git.GetBranch(gitRoot)
		gitAhead, gitBehind, gitHasUpstream = git.GetAheadBehind(gitRoot)
	}

	// Set up file watcher
	watcher, _ := fsnotify.NewWatcher()
	if watcher != nil {
		// Watch root and all subdirectories
		filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				// Skip hidden and common ignore dirs
				if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
				watcher.Add(path)
			}
			return nil
		})
		// Explicitly watch .context-docs.md for auto-reload
		contextDocsPath := filepath.Join(absPath, ".context-docs.md")
		watcher.Add(contextDocsPath)
	}

	return Model{
		rootPath:     absPath,
		entries:      entries,
		cursor:       0,
		activePane:   TreePane,
		splitRatio:   splitRatio,
		previewCache: make(map[string]CachedPreview),
		searchInput:  ti,
		allFiles:     allFiles,
		watcher:      watcher,
		// Context docs
		docRegistry:  docRegistry,
		selectedDocs: make(map[string]bool),
		// Git integration
		isGitRepo:      isGit,
		gitRepoRoot:    gitRoot,
		gitStatus:      gitStatus,
		gitDirStatus:   gitDirStatus,
		gitChanges:     gitChanges,
		gitBranch:      gitBranch,
		gitAhead:       gitAhead,
		gitBehind:      gitBehind,
		gitHasUpstream: gitHasUpstream,
	}
}

// CollectAllFiles recursively collects all file paths from a directory
func CollectAllFiles(root string) []string {
	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden files/dirs and common ignores (except .context-docs.md)
		name := info.Name()
		if strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			// Always show .context-docs.md - it's part of contexTUI workflow
			if name != ".context-docs.md" {
				return nil
			}
		}
		if name == "node_modules" || name == "vendor" || name == "__pycache__" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			// Store relative path for display
			relPath, _ := filepath.Rel(root, path)
			files = append(files, relPath)
		}
		return nil
	})
	return files
}

// LoadDirectory loads directory entries at the specified depth
func LoadDirectory(path string, depth int) []Entry {
	var entries []Entry

	files, err := os.ReadDir(path)
	if err != nil {
		return entries
	}

	for _, f := range files {
		// Skip hidden files and common ignores (except .context-docs.md)
		if strings.HasPrefix(f.Name(), ".") && f.Name() != ".context-docs.md" {
			continue
		}
		if f.Name() == "node_modules" || f.Name() == "vendor" || f.Name() == "__pycache__" {
			continue
		}

		e := Entry{
			Name:  f.Name(),
			Path:  filepath.Join(path, f.Name()),
			IsDir: f.IsDir(),
			Depth: depth,
		}
		entries = append(entries, e)
	}

	return entries
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return m.waitForFsEvent()
}

// waitForFsEvent returns a command that waits for the next filesystem event
func (m Model) waitForFsEvent() tea.Cmd {
	if m.watcher == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case _, ok := <-m.watcher.Events:
			if !ok {
				return nil
			}
			// Debounce: wait a bit for rapid changes to settle
			// Drain any additional events that came in
			for {
				select {
				case <-m.watcher.Events:
				default:
					return FsEventMsg{}
				}
			}
		case <-m.watcher.Errors:
			return nil
		}
	}
}

// FlatEntries returns a flat list of all visible entries
func (m Model) FlatEntries() []Entry {
	return flattenEntries(m.entries)
}

func flattenEntries(entries []Entry) []Entry {
	var flat []Entry
	for _, e := range entries {
		flat = append(flat, e)
		if e.IsDir && e.Expanded {
			flat = append(flat, flattenEntries(e.Children)...)
		}
	}
	return flat
}

// LeftPaneWidth returns the width of the left (tree) pane
func (m Model) LeftPaneWidth() int {
	// Total usable width minus borders and gap
	usable := m.width - 4 // 2 for each pane's border
	return int(float64(usable) * m.splitRatio)
}

// RightPaneWidth returns the width of the right (preview) pane
func (m Model) RightPaneWidth() int {
	usable := m.width - 4
	return usable - m.LeftPaneWidth()
}

// DividerX returns the X position of the divider between panes
func (m Model) DividerX() int {
	return m.LeftPaneWidth() + 2 // +2 for left pane border
}

// HandlePaneResize adjusts the split ratio between left and right panes
func (m *Model) HandlePaneResize(direction string) {
	switch direction {
	case "left":
		if m.splitRatio > 0.2 {
			m.splitRatio -= 0.05
		}
	case "right":
		if m.splitRatio < 0.8 {
			m.splitRatio += 0.05
		}
	}
	m.tree.Width = m.LeftPaneWidth() - 2
	m.preview.Width = m.RightPaneWidth() - 2
	m.tree.SetContent(m.RenderTree())
	config.Save(m.rootPath, config.Config{SplitRatio: m.splitRatio})
}

// HandlePreviewScroll scrolls the preview pane
func (m *Model) HandlePreviewScroll(direction string) {
	switch direction {
	case "up":
		m.preview.LineUp(1)
	case "down":
		m.preview.LineDown(1)
	case "half-up":
		m.preview.HalfViewUp()
	case "half-down":
		m.preview.HalfViewDown()
	case "top":
		m.preview.GotoTop()
	case "bottom":
		m.preview.GotoBottom()
	}
}
