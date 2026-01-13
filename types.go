package main

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

type pane int

const (
	treePane pane = iota
	previewPane
)

// GitFileStatus represents the status of a file in git
type GitFileStatus struct {
	Path    string // Relative path from repo root
	Status  string // "M", "A", "D", "R", "?", "!", etc.
	Staged  bool   // True if change is staged
	OldPath string // For renames, the original path
}

type model struct {
	rootPath       string
	entries        []entry
	cursor         int
	activePane     pane
	tree           viewport.Model // Scrollable tree pane
	preview        viewport.Model
	previewContent string
	previewPath    string
	previewCache   map[string]cachedPreview // filepath -> cached rendered content
	loading        bool
	width          int
	height         int
	ready          bool
	lastClickTime  time.Time
	lastClickIndex int

	// Pane resizing
	splitRatio    float64 // 0.2 to 0.8, left pane width ratio
	draggingSplit bool    // True when dragging the divider

	// Fuzzy finder
	searching     bool
	searchInput   textinput.Model
	searchResults []searchResult
	searchCursor  int
	allFiles      []string // Flat list of all file paths for searching

	// Context groups (documentation-first)
	docRegistry        *DocGroupRegistry   // Doc-based context groups
	showingGroups      bool                // True when groups overlay is visible
	selectedSupergroup int                 // Index of selected supergroup (for filtering)
	docGroupCursor     int                 // Selected group in current supergroup view
	groupsScrollOffset int                 // Scroll offset for groups overlay
	selectedGroups     map[string]bool     // Selected groups for multi-copy (keyed by filepath)
	addingGroup        bool                // True when in "add group" mode
	availableMdFiles   []string          // .md files available to add
	addGroupCursor     int               // Cursor in add group picker
	addGroupScroll     int               // Scroll offset in add group picker

	// File watcher
	watcher *fsnotify.Watcher

	// Copy mode with custom selection
	selectMode   bool
	isSelecting  bool     // True while mouse is being dragged
	selectStart  int      // Line where selection started
	selectEnd    int      // Line where selection currently ends
	previewLines []string // Content split by lines for selection/copy
	scrollDir    int      // -1 for up, 0 for none, 1 for down (for continuous scroll)

	// Git integration
	isGitRepo       bool
	gitRepoRoot     string                   // Git repo root (may differ from rootPath)
	gitStatus       map[string]GitFileStatus // relPath -> status
	gitDirStatus    map[string]string        // dir relPath -> aggregated status indicator
	gitStatusMode   bool                     // True when showing git status view
	gitStatusCursor int                      // Cursor in git status view
	gitChanges      []GitFileStatus          // Flat list of all changes for git view
	gitBranch       string                   // Current branch name
	gitAhead        int                      // Commits ahead of upstream
	gitBehind       int                      // Commits behind upstream
	gitHasUpstream  bool                     // Whether branch has upstream configured
	gitFetching     bool                     // True while fetch is in progress

	// Help overlay
	showingHelp bool // True when help overlay is visible

	// Status message (transient feedback)
	statusMessage     string
	statusMessageTime time.Time
}

// Message for continuous scroll tick
type scrollTickMsg struct{}

// Message to clear status message after delay
type clearStatusMsg struct{}

// clearStatusAfter returns a command that clears the status message after a delay
func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

type searchResult struct {
	path         string
	displayName  string
	matchedIndex int // Index into allFiles
}

// Message sent when file content is loaded
type fileLoadedMsg struct {
	path    string
	content string
	modTime time.Time // For cache validation
}

// cachedPreview stores rendered preview content with modification time
type cachedPreview struct {
	content string
	modTime time.Time
}

// Message sent when filesystem changes
type fsEventMsg struct{}

// Message to continue watching after an event
type watchNextMsg struct{}

// Message sent when git fetch completes
type gitFetchDoneMsg struct {
	err error
}

type entry struct {
	name     string
	path     string
	isDir    bool
	depth    int
	expanded bool
	children []entry
}

// Config represents user preferences saved per-project
type Config struct {
	SplitRatio float64 `json:"splitRatio,omitempty"`
}
