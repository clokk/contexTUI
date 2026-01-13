# Git Integration

**Category:** Feature
**Status:** Active

## Description

contexTUI provides deep git integration to help developers understand repository state at a glance while browsing files. Features include branch status with ahead/behind indicators, file status badges in the tree view, and a dedicated git status overlay.

## Key Files

- internal/git/git.go - Git operations (status, fetch, branch info)
- internal/app/types.go - GitFileStatus struct and related types
- internal/app/model.go - Git state initialization
- internal/app/view.go - Git status rendering (badges, branch bar, status view)

---

## Branch Status Bar

The footer displays the current branch with sync status:
- `main ✓` - Branch is in sync with remote
- `main ↑2` - 2 commits ahead of remote
- `main ↓3` - 3 commits behind remote
- `main ↑2 ↓3` - Both ahead and behind (needs rebase)
- `main ⟳` - Currently fetching from remote

Press `f` to fetch from remote and update the ahead/behind counts.

## File Status Badges

Files in the tree show git status badges:
- `M` (yellow) - Modified
- `A` (green) - Added/staged
- `D` (red) - Deleted
- `R` (blue) - Renamed
- `?` (gray) - Untracked
- `U` or `!` (red) - Conflict

Directories containing changed files show a `●` indicator.

## Git Status View

Press `s` to open the dedicated git status view showing:
- All changed files organized by status
- Side-by-side diff preview
- Quick navigation between changes

Press `esc` to return to normal file browsing.

## Implementation Notes

Git status is refreshed:
- On startup
- After fetch completes
- When file watcher detects changes in `.git` directory

The status uses `git status --porcelain` for file states and `git rev-list` for ahead/behind counts.
