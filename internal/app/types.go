package app

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/connorleisz/contexTUI/internal/groups"
	"github.com/fsnotify/fsnotify"
)

// Pane identifies which pane is active
type Pane int

const (
	TreePane Pane = iota
	PreviewPane
)

// Model is the main application model implementing tea.Model
type Model struct {
	rootPath       string
	entries        []Entry
	cursor         int
	activePane     Pane
	tree           viewport.Model // Scrollable tree pane
	preview        viewport.Model
	previewContent string
	previewPath    string
	previewCache   map[string]CachedPreview // filepath -> cached rendered content
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
	searchResults []SearchResult
	searchCursor  int
	allFiles      []string // Flat list of all file paths for searching

	// Context docs (documentation-first)
	docRegistry        *groups.ContextDocRegistry // Doc-based context docs
	showingDocs        bool                       // True when docs overlay is visible
	selectedCategory   int                        // Index of selected category (for filtering)
	docCursor          int                        // Selected doc in current category view
	docsScrollOffset   int                        // Scroll offset for docs overlay
	selectedDocs       map[string]bool            // Selected docs for multi-copy (keyed by filepath)
	addingDoc          bool                       // True when in "add doc" mode
	availableMdFiles   []string                   // .md files available to add
	addDocCursor       int                        // Cursor in add doc picker
	addDocScroll       int                        // Scroll offset in add doc picker
	selectedAddFiles   map[string]bool            // Selected files for multi-add

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
	gitRepoRoot     string                    // Git repo root (may differ from rootPath)
	gitStatus       map[string]git.FileStatus // relPath -> status
	gitDirStatus    map[string]string         // dir relPath -> aggregated status indicator
	gitStatusMode   bool                      // True when showing git status view
	gitStatusCursor int                       // Cursor in git status view
	gitChanges      []git.FileStatus          // Flat list of all changes for git view
	gitList         viewport.Model            // Scrollable git file list viewport
	diffCache       map[DiffCacheKey]CachedDiff // Cache for diff content
	diffRequestID   int64                     // Current diff request ID for cancellation
	fullDiffLoading string                    // Path of file whose full diff is loading
	fullDiffStaged  bool                      // Whether the loading full diff is staged
	gitBranch       string                    // Current branch name
	gitAhead        int                       // Commits ahead of upstream
	gitBehind       int                       // Commits behind upstream
	gitHasUpstream  bool                      // Whether branch has upstream configured
	gitFetching     bool                      // True while fetch is in progress

	// Help overlay
	showingHelp bool // True when help overlay is visible

	// Status message (transient feedback)
	statusMessage     string
	statusMessageTime time.Time

	// Registry save state (for debounced background saves)
	registryDirty  bool // Whether registry needs saving
	registrySaving bool // Whether a save is in progress
}

// ScrollTickMsg is sent for continuous scroll tick
type ScrollTickMsg struct{}

// ClearStatusMsg is sent to clear the status message after delay
type ClearStatusMsg struct{}

// ClearStatusAfter returns a command that clears the status message after a delay
func ClearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return ClearStatusMsg{}
	})
}

// SearchResult represents a file search result
type SearchResult struct {
	Path         string
	DisplayName  string
	MatchedIndex int // Index into allFiles
}

// FileLoadedMsg is sent when file content is loaded
type FileLoadedMsg struct {
	Path    string
	Content string
	ModTime time.Time // For cache validation
}

// CachedPreview stores rendered preview content with modification time
type CachedPreview struct {
	Content string
	ModTime time.Time
}

// FsEventMsg is sent when filesystem changes
type FsEventMsg struct{}

// WatchNextMsg is sent to continue watching after an event
type WatchNextMsg struct{}

// GitFetchDoneMsg is sent when git fetch completes
type GitFetchDoneMsg struct {
	Err error
}

// QuickDiffLoadedMsg is sent when the quick (small context) diff is ready
type QuickDiffLoadedMsg struct {
	Path      string
	Content   string
	ModTime   time.Time
	Staged    bool
	RequestID int64 // To match against current request for cancellation
}

// FullDiffLoadedMsg is sent when the full (large context) diff is ready
type FullDiffLoadedMsg struct {
	Path      string
	Content   string
	ModTime   time.Time
	Staged    bool
	RequestID int64
}

// DiffCacheKey uniquely identifies a cached diff
type DiffCacheKey struct {
	Path        string
	Staged      bool
	ContextSize int // 10 for quick, 99999 for full
}

// CachedDiff stores diff content with metadata
type CachedDiff struct {
	Content     string
	ModTime     time.Time
	ContextSize int
}

// SaveRegistryMsg signals that the debounced save timer fired
type SaveRegistryMsg struct{}

// RegistrySavedMsg signals save completion
type RegistrySavedMsg struct {
	Err error
}

// ScheduleRegistrySave returns a command that fires after debounce delay
func ScheduleRegistrySave(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return SaveRegistryMsg{}
	})
}

// Entry represents a file or directory in the tree
type Entry struct {
	Name     string
	Path     string
	IsDir    bool
	Depth    int
	Expanded bool
	Children []Entry
}
