# File Management

**Category:** Feature
**Status:** Active

## Description

contexTUI provides basic file management operations directly from the tree pane, allowing you to create, rename, and delete files and folders without leaving the application.

## Key Files

- internal/app/update_fileop.go - File operation logic and async handlers
- internal/app/update.go - Key bindings and message routing
- internal/app/view.go - File operation overlay rendering
- internal/app/types.go - FileOpMode and FileOpCompleteMsg types

---

## Operations

### Create File (`n`)

Creates a new file in the currently selected directory. If a file is selected, the new file is created in its parent directory.

1. Press `n` to open the create file overlay
2. Type the filename
3. Press `Enter` to create, `Esc` to cancel

### Create Folder (`N`)

Creates a new folder in the currently selected directory.

1. Press `N` (Shift+n) to open the create folder overlay
2. Type the folder name
3. Press `Enter` to create, `Esc` to cancel

### Rename (`r`)

Renames the selected file or folder.

1. Press `r` to open the rename overlay
2. The current name is pre-filled for editing
3. Press `Enter` to rename, `Esc` to cancel

### Delete (`d` or `x`)

Deletes the selected file or folder with confirmation.

1. Press `d` or `x` to open the delete overlay
2. Press `Enter` once to see the confirmation warning
3. Press `Enter` again (or `y`) to confirm deletion
4. Press `Esc` at any time to cancel

## Validation

File operations include validation:
- Names cannot be empty
- Names cannot contain path separators (`/` or `\`)
- Creating a file/folder that already exists shows an error
- Renaming to the same name is allowed (no-op)

## Auto-Refresh

After any file operation completes, the tree view automatically refreshes via the existing file watcher. The status bar shows a brief confirmation message.

## Error Handling

If an operation fails (e.g., permission denied), the error is displayed in the overlay. You can retry or press `Esc` to cancel.
