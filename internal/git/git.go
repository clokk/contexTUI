package git

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// FileStatus represents the status of a file in git
type FileStatus struct {
	Path    string // Relative path from repo root
	Status  string // "M", "A", "D", "R", "?", "!", etc.
	Staged  bool   // True if change is staged
	OldPath string // For renames, the original path
}

// IsRepo checks if the path is inside a git repository
// Returns (isRepo, repoRoot)
func IsRepo(path string) (bool, string) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}
	return true, strings.TrimSpace(string(output))
}

// LoadStatus runs git status and returns file statuses
func LoadStatus(repoRoot string) (map[string]FileStatus, []FileStatus) {
	statusMap := make(map[string]FileStatus)
	var changes []FileStatus

	// Run git status --porcelain=v1 for machine-readable output
	cmd := exec.Command("git", "-C", repoRoot, "status", "--porcelain=v1")
	output, err := cmd.Output()
	if err != nil {
		return statusMap, changes
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}

		// Format: XY PATH or XY OLDPATH -> NEWPATH
		// X = index status, Y = working tree status
		indexStatus := line[0]
		workStatus := line[1]
		path := line[3:] // Skip "XY "

		// Handle renames: "R  oldpath -> newpath"
		oldPath := ""
		if strings.Contains(path, " -> ") {
			parts := strings.Split(path, " -> ")
			oldPath = parts[0]
			path = parts[1]
		}

		status := FileStatus{
			Path:    path,
			OldPath: oldPath,
		}

		// Determine display status and staged flag
		// Priority: show staged status if staged, otherwise working tree status
		if indexStatus != ' ' && indexStatus != '?' {
			status.Staged = true
			status.Status = string(indexStatus)
		} else if workStatus != ' ' {
			status.Staged = false
			status.Status = string(workStatus)
		} else if indexStatus == '?' {
			status.Staged = false
			status.Status = "?"
		}

		if status.Status != "" {
			statusMap[path] = status
			changes = append(changes, status)
		}
	}

	return statusMap, changes
}

// ComputeDirStatus aggregates file statuses to parent directories
func ComputeDirStatus(statusMap map[string]FileStatus) map[string]string {
	dirStatus := make(map[string]string)

	// Priority: ! > M > A > D > R > ?
	priority := map[string]int{"!": 6, "U": 5, "M": 4, "A": 3, "D": 2, "R": 1, "?": 0}

	for path, status := range statusMap {
		dir := filepath.Dir(path)
		for dir != "." && dir != "" && dir != "/" {
			current := dirStatus[dir]
			if priority[status.Status] > priority[current] {
				dirStatus[dir] = status.Status
			}
			dir = filepath.Dir(dir)
		}
	}

	return dirStatus
}

// GetBranch returns the current branch name
func GetBranch(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetAheadBehind returns commits ahead and behind upstream
// Returns (ahead, behind, hasUpstream)
func GetAheadBehind(repoRoot string) (int, int, bool) {
	// Check if upstream exists
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "@{upstream}")
	if _, err := cmd.Output(); err != nil {
		return 0, 0, false // No upstream configured
	}

	// Get ahead/behind counts
	cmd = exec.Command("git", "-C", repoRoot, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, false
	}

	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return 0, 0, false
	}

	behind, _ := strconv.Atoi(parts[0])
	ahead, _ := strconv.Atoi(parts[1])
	return ahead, behind, true
}

// Fetch runs git fetch for the current branch's upstream
func Fetch(repoRoot string) error {
	cmd := exec.Command("git", "-C", repoRoot, "fetch")
	return cmd.Run()
}

// LoadDiff runs git diff and returns the diff output for a file
func LoadDiff(repoRoot, filePath string, staged bool) (string, error) {
	var args []string
	if staged {
		args = []string{"-C", repoRoot, "diff", "--cached", "--", filePath}
	} else {
		args = []string{"-C", repoRoot, "diff", "--", filePath}
	}

	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil || len(output) == 0 {
		return "", err
	}

	return string(output), nil
}
