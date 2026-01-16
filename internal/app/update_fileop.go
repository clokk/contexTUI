package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// updateFileOp handles file operation overlay interactions
func (m Model) updateFileOp(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel and close overlay
			m.fileOpMode = FileOpNone
			m.fileOpInput.Blur()
			m.fileOpError = ""
			m.fileOpConfirm = false
			m.fileOpScrollOffset = 0
			m.fileOpSourcePath = "" // Clear import source
			return m, nil

		case "enter":
			if m.fileOpMode == FileOpDelete {
				if !m.fileOpConfirm {
					// First enter shows confirmation
					m.fileOpConfirm = true
					return m, nil
				}
				// Second enter executes delete
				return m, m.executeFileOp()
			}
			// For create/rename, validate and execute
			name := m.fileOpInput.Value()
			if err := m.validateFileName(name); err != nil {
				m.fileOpError = err.Error()
				return m, nil
			}
			return m, m.executeFileOp()

		case "y", "Y":
			// Quick confirm for delete
			if m.fileOpMode == FileOpDelete {
				return m, m.executeFileOp()
			}
		}

	case tea.MouseMsg:
		// Handle scroll for long paths
		if msg.Button == tea.MouseButtonWheelUp {
			if m.fileOpScrollOffset > 0 {
				m.fileOpScrollOffset--
			}
			return m, nil
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.fileOpScrollOffset++
			return m, nil
		}
	}

	// Update text input for create/rename
	if m.fileOpMode != FileOpDelete {
		var cmd tea.Cmd
		m.fileOpInput, cmd = m.fileOpInput.Update(msg)
		m.fileOpError = "" // Clear error on typing
		return m, cmd
	}

	return m, nil
}

// executeFileOp performs the actual file system operation asynchronously
func (m Model) executeFileOp() tea.Cmd {
	switch m.fileOpMode {
	case FileOpCreateFile:
		newPath := filepath.Join(m.fileOpTargetPath, m.fileOpInput.Value())
		return createFileAsync(newPath)
	case FileOpCreateFolder:
		newPath := filepath.Join(m.fileOpTargetPath, m.fileOpInput.Value())
		return createFolderAsync(newPath)
	case FileOpRename:
		dir := filepath.Dir(m.fileOpTargetPath)
		newPath := filepath.Join(dir, m.fileOpInput.Value())
		return renameAsync(m.fileOpTargetPath, newPath)
	case FileOpDelete:
		return deleteAsync(m.fileOpTargetPath)
	case FileOpImport:
		destPath := filepath.Join(m.fileOpTargetPath, m.fileOpInput.Value())
		return copyFileAsync(m.fileOpSourcePath, destPath)
	}
	return nil
}

// getTargetDirectory returns the directory for creating new files
// If cursor is on a directory, returns that directory
// If cursor is on a file, returns its parent directory
func (m Model) getTargetDirectory() string {
	flat := m.FlatEntries()
	if m.cursor < len(flat) {
		e := flat[m.cursor]
		if e.IsDir {
			return e.Path
		}
		return filepath.Dir(e.Path)
	}
	return m.rootPath
}

// validateFileName checks if a filename is valid
func (m Model) validateFileName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("name cannot contain path separators")
	}
	if strings.Contains(name, "\x00") {
		return fmt.Errorf("name contains invalid characters")
	}

	// For rename, check if renaming to same name (allow it as no-op)
	if m.fileOpMode == FileOpRename {
		currentName := filepath.Base(m.fileOpTargetPath)
		if name == currentName {
			return nil // Same name is fine
		}
	}

	// Check if file already exists (for create and rename to different name)
	var targetDir string
	if m.fileOpMode == FileOpRename {
		targetDir = filepath.Dir(m.fileOpTargetPath)
	} else {
		targetDir = m.fileOpTargetPath
	}
	fullPath := filepath.Join(targetDir, name)
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("'%s' already exists", name)
	}

	return nil
}

// Async file operations

func createFileAsync(path string) tea.Cmd {
	return func() tea.Msg {
		// Create parent directories if needed
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return FileOpCompleteMsg{Op: FileOpCreateFile, Success: false, Error: err}
		}
		f, err := os.Create(path)
		if err != nil {
			return FileOpCompleteMsg{Op: FileOpCreateFile, Success: false, Error: err}
		}
		f.Close()
		return FileOpCompleteMsg{Op: FileOpCreateFile, Success: true, NewPath: path}
	}
}

func createFolderAsync(path string) tea.Cmd {
	return func() tea.Msg {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return FileOpCompleteMsg{Op: FileOpCreateFolder, Success: false, Error: err}
		}
		return FileOpCompleteMsg{Op: FileOpCreateFolder, Success: true, NewPath: path}
	}
}

func renameAsync(oldPath, newPath string) tea.Cmd {
	return func() tea.Msg {
		// Check if renaming to same path (no-op)
		if oldPath == newPath {
			return FileOpCompleteMsg{Op: FileOpRename, Success: true, NewPath: newPath}
		}
		err := os.Rename(oldPath, newPath)
		if err != nil {
			return FileOpCompleteMsg{Op: FileOpRename, Success: false, Error: err}
		}
		return FileOpCompleteMsg{Op: FileOpRename, Success: true, NewPath: newPath}
	}
}

func deleteAsync(path string) tea.Cmd {
	return func() tea.Msg {
		err := os.RemoveAll(path)
		if err != nil {
			return FileOpCompleteMsg{Op: FileOpDelete, Success: false, Error: err}
		}
		return FileOpCompleteMsg{Op: FileOpDelete, Success: true}
	}
}

func copyFileAsync(src, dst string) tea.Cmd {
	return func() tea.Msg {
		srcFile, err := os.Open(src)
		if err != nil {
			return FileOpCompleteMsg{Op: FileOpImport, Success: false, Error: err}
		}
		defer srcFile.Close()

		srcInfo, err := srcFile.Stat()
		if err != nil {
			return FileOpCompleteMsg{Op: FileOpImport, Success: false, Error: err}
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return FileOpCompleteMsg{Op: FileOpImport, Success: false, Error: err}
		}

		dstFile, err := os.Create(dst)
		if err != nil {
			return FileOpCompleteMsg{Op: FileOpImport, Success: false, Error: err}
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return FileOpCompleteMsg{Op: FileOpImport, Success: false, Error: err}
		}

		// Preserve permissions (non-fatal if fails)
		os.Chmod(dst, srcInfo.Mode())

		return FileOpCompleteMsg{Op: FileOpImport, Success: true, NewPath: dst}
	}
}
