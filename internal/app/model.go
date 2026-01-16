package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/config"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/fsnotify/fsnotify"
)

// NewModel creates and initializes a new application model
// Heavy loading is deferred to Init() for async execution
func NewModel(rootPath string) Model {
	absPath, _ := filepath.Abs(rootPath)

	// Load user config (fast, local file)
	cfg := config.Load(absPath)

	// Determine split ratio (config or default)
	splitRatio := 0.5
	if cfg.SplitRatio >= 0.2 && cfg.SplitRatio <= 0.8 {
		splitRatio = cfg.SplitRatio
	}

	// Determine dotfile visibility (config or default)
	showDotfiles := cfg.ShowDotfiles

	// Set up search input
	ti := textinput.New()
	ti.Placeholder = "Search files..."
	ti.CharLimit = 100
	ti.Width = 40

	// Set up file operation input
	foInput := textinput.New()
	foInput.Placeholder = "filename"
	foInput.CharLimit = 255
	foInput.Width = 40 // Will be adjusted dynamically based on overlay width

	// Check for git repository (fast check)
	isGit, gitRoot := git.IsRepo(absPath)

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

	// Calculate pending loads count
	pendingLoads := 3 // directory, allFiles, registry
	if isGit {
		pendingLoads = 4 // + git status
	}

	return Model{
		rootPath:     absPath,
		entries:      nil, // Loaded async in Init()
		cursor:       0,
		activePane:   TreePane,
		splitRatio:   splitRatio,
		previewCache: make(map[string]CachedPreview),
		searchInput:  ti,
		allFiles:     nil, // Loaded async in Init()
		watcher:      watcher,
		// Context docs - loaded async in Init()
		docRegistry:      nil,
		selectedDocs:     make(map[string]bool),
		selectedAddFiles: make(map[string]bool),
		// Git integration - loaded async in Init()
		isGitRepo:    isGit,
		gitRepoRoot:  gitRoot,
		gitStatus:    make(map[string]git.FileStatus),
		gitDirStatus: make(map[string]string),
		diffCache:    make(map[DiffCacheKey]CachedDiff),
		// Dotfile visibility
		showDotfiles: showDotfiles,
		// File operations
		fileOpInput: foInput,
		// Start with loading state
		loadingMessage: "Starting up...",
		pendingLoads:   pendingLoads,
	}
}

// CollectAllFiles recursively collects all file paths from a directory
func CollectAllFiles(root string, showDotfiles bool) []string {
	var files []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		name := info.Name()
		// Handle dotfiles
		if strings.HasPrefix(name, ".") {
			// .git is always hidden
			if name == ".git" {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// .context-docs.md is always visible
			if name == ".context-docs.md" {
				// continue to add it
			} else if !showDotfiles {
				// Skip other dotfiles/dirs unless toggle is on
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		// Always skip common package/build directories
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
// rootPath is used to compute relative paths for caching
func LoadDirectory(path string, depth int, showDotfiles bool) []Entry {
	return LoadDirectoryWithRoot(path, path, depth, showDotfiles)
}

// LoadDirectoryWithRoot loads directory entries with root path for relative path computation
func LoadDirectoryWithRoot(path, rootPath string, depth int, showDotfiles bool) []Entry {
	var entries []Entry

	files, err := os.ReadDir(path)
	if err != nil {
		return entries
	}

	for _, f := range files {
		name := f.Name()
		// Handle dotfiles
		if strings.HasPrefix(name, ".") {
			// .git is always hidden
			if name == ".git" {
				continue
			}
			// .context-docs.md is always visible
			if name == ".context-docs.md" {
				// continue to add it
			} else if !showDotfiles {
				// Skip other dotfiles unless toggle is on
				continue
			}
		}
		// Always skip common package/build directories
		if name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}

		fullPath := filepath.Join(path, name)
		relPath, _ := filepath.Rel(rootPath, fullPath)
		e := Entry{
			Name:    name,
			Path:    fullPath,
			IsDir:   f.IsDir(),
			Depth:   depth,
			RelPath: relPath,
		}
		entries = append(entries, e)
	}

	return entries
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	// Start async loading of all heavy operations
	cmds := []tea.Cmd{
		m.loadDirectoryAsync(),
		m.loadAllFilesAsync(),
		m.loadRegistryAsync(),
		SpinnerTick(),
		m.waitForFsEvent(),
	}
	if m.isGitRepo {
		cmds = append(cmds, m.loadGitStatusAsync())
	}
	return tea.Batch(cmds...)
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
// Uses cache when available for performance
func (m Model) FlatEntries() []Entry {
	if m.treeCache.valid && m.treeCache.flatEntries != nil {
		return m.treeCache.flatEntries
	}
	return flattenEntries(m.entries)
}

// FlatEntriesCached returns cached flat entries, rebuilding cache if needed
func (m *Model) FlatEntriesCached() []Entry {
	if !m.treeCache.valid || m.treeCache.flatEntries == nil {
		m.treeCache.flatEntries = flattenEntries(m.entries)
		m.treeCache.valid = true
	}
	return m.treeCache.flatEntries
}

// InvalidateTreeCache marks the tree cache as stale
func (m *Model) InvalidateTreeCache() {
	m.treeCache.valid = false
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
