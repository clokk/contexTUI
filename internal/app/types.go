package app

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/connorleisz/contexTUI/internal/groups"
	"github.com/connorleisz/contexTUI/internal/terminal"
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
	treeCache      TreeCache // Cached tree data for rendering optimization

	// Pane resizing
	splitRatio    float64 // 0.2 to 0.8, left pane width ratio
	draggingSplit bool    // True when dragging the divider

	// Fuzzy finder
	searching            bool
	searchInput          textinput.Model
	searchResults        []SearchResult
	searchCursor         int
	searchScrollOffset   int      // Scroll offset for search results viewport
	lastSearchQuery      string   // Previous query to detect changes
	pendingSearchQuery   string   // Query waiting for debounce
	searchDebounceActive bool     // Whether a debounce timer is pending
	allFiles             []string // Flat list of all file paths for searching

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
	showingHelp      bool // True when help overlay is visible
	helpScrollOffset int  // Scroll offset for help overlay

	// Dotfile visibility
	showDotfiles bool // True when dotfiles are visible in tree

	// Status message (transient feedback)
	statusMessage     string
	statusMessageTime time.Time

	// Registry save state (for debounced background saves)
	registryDirty  bool // Whether registry needs saving
	registrySaving bool // Whether a save is in progress

	// Async loading state (for non-blocking UI updates)
	loadingMessage string // Current loading message (empty = not loading)
	spinnerFrame   int    // Current spinner animation frame
	pendingLoads   int    // Number of async load operations in progress

	// File operations
	fileOpMode         FileOpMode      // Current file operation mode
	fileOpInput        textinput.Model // Text input for name entry
	fileOpTargetPath   string          // Path being operated on
	fileOpError        string          // Error message to display
	fileOpConfirm      bool            // True when showing delete confirmation
	fileOpScrollOffset int             // Scroll offset for long paths/errors
	fileOpSourcePath   string          // Source path for import operation

	// Terminal capabilities
	termCaps terminal.Capabilities

	// Image preview
	previewIsImage bool                    // True when previewing an image
	currentImage   *ImageLoadedMsg         // Current image preview data
	imageCache     map[string]CachedImage  // Path -> cached image render

	// Image overlay mode (full-screen Kitty rendering)
	imageOverlayMode bool   // Whether image overlay is active
	imageOverlayData string // Pre-rendered Kitty escape sequences
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

// SearchDebounceMsg is sent after debounce delay to trigger fuzzy search
type SearchDebounceMsg struct{}

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

// SpinnerTickMsg is sent to animate the loading spinner
type SpinnerTickMsg struct{}

// SpinnerTick returns a command that ticks the spinner animation
func SpinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// SpinnerChars are the braille dot characters for the spinner animation
var SpinnerChars = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// DebouncedFsEventMsg is sent after debounce delay to trigger actual reload
type DebouncedFsEventMsg struct{}

// ScheduleFsReload returns a command that fires after debounce delay
func ScheduleFsReload(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return DebouncedFsEventMsg{}
	})
}

// DirectoryLoadedMsg is sent when directory entries are loaded asynchronously
type DirectoryLoadedMsg struct {
	Entries []Entry
}

// AllFilesLoadedMsg is sent when all files list is collected asynchronously
type AllFilesLoadedMsg struct {
	Files []string
}

// RegistryLoadedMsg is sent when doc registry is loaded asynchronously
type RegistryLoadedMsg struct {
	Registry *groups.ContextDocRegistry
}

// GitStatusLoadedMsg is sent when git status is loaded asynchronously
type GitStatusLoadedMsg struct {
	Status      map[string]git.FileStatus
	Changes     []git.FileStatus
	DirStatus   map[string]string
	Branch      string
	Ahead       int
	Behind      int
	HasUpstream bool
}

// FileOpMode represents the current file operation
type FileOpMode int

const (
	FileOpNone FileOpMode = iota
	FileOpCreateFile
	FileOpCreateFolder
	FileOpRename
	FileOpDelete
	FileOpImport // Import file via drag-and-drop
)

// FileOpCompleteMsg is sent when a file operation completes
type FileOpCompleteMsg struct {
	Op      FileOpMode
	Success bool
	Error   error
	NewPath string // For create/rename, the resulting path
}

// ImageLoadedMsg is sent when an image is loaded and rendered
type ImageLoadedMsg struct {
	Path       string
	Width      int       // Original image width in pixels
	Height     int       // Original image height in pixels
	RenderW    int       // Rendered width in terminal cells
	RenderH    int       // Rendered height in terminal cells
	RenderData string    // Pre-rendered terminal escape sequences or block chars
	ModTime    time.Time
	Error      error
}

// CachedImage stores pre-rendered image data
type CachedImage struct {
	RenderData  string
	Width       int       // Original image width
	Height      int       // Original image height
	RenderW     int       // Rendered width in terminal cells
	RenderH     int       // Rendered height in terminal cells
	ViewportW   int       // Viewport width when cached (for invalidation)
	ViewportH   int       // Viewport height when cached (for invalidation)
	ModTime     time.Time
}

// Entry represents a file or directory in the tree
type Entry struct {
	Name     string
	Path     string
	IsDir    bool
	Depth    int
	Expanded bool
	Children []Entry
	RelPath  string // Cached relative path from root
}

// TreeCache stores pre-computed tree data to avoid recomputation on every render
type TreeCache struct {
	flatEntries []Entry // Cached flattened entries
	valid       bool    // Whether cache is valid
}
