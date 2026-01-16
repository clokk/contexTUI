package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/connorleisz/contexTUI/internal/git"
	"github.com/connorleisz/contexTUI/internal/groups"
)

// loadDirectoryAsync returns a command that loads directory entries in the background
func (m Model) loadDirectoryAsync() tea.Cmd {
	rootPath := m.rootPath
	showDotfiles := m.showDotfiles
	return func() tea.Msg {
		entries := LoadDirectoryWithRoot(rootPath, rootPath, 0, showDotfiles)
		return DirectoryLoadedMsg{Entries: entries}
	}
}

// loadAllFilesAsync returns a command that collects all file paths in the background
func (m Model) loadAllFilesAsync() tea.Cmd {
	rootPath := m.rootPath
	showDotfiles := m.showDotfiles
	return func() tea.Msg {
		files := CollectAllFiles(rootPath, showDotfiles)
		return AllFilesLoadedMsg{Files: files}
	}
}

// loadRegistryAsync returns a command that loads the doc registry in the background
func (m Model) loadRegistryAsync() tea.Cmd {
	rootPath := m.rootPath
	return func() tea.Msg {
		registry, _ := groups.LoadContextDocRegistry(rootPath)
		return RegistryLoadedMsg{Registry: registry}
	}
}

// loadGitStatusAsync returns a command that loads git status in the background
func (m Model) loadGitStatusAsync() tea.Cmd {
	if !m.isGitRepo {
		return nil
	}
	repoRoot := m.gitRepoRoot
	return func() tea.Msg {
		status, changes := git.LoadStatus(repoRoot)
		dirStatus := git.ComputeDirStatus(status)
		branch := git.GetBranch(repoRoot)
		ahead, behind, hasUpstream := git.GetAheadBehind(repoRoot)
		return GitStatusLoadedMsg{
			Status:      status,
			Changes:     changes,
			DirStatus:   dirStatus,
			Branch:      branch,
			Ahead:       ahead,
			Behind:      behind,
			HasUpstream: hasUpstream,
		}
	}
}

// checkLoadingComplete decrements the pending load counter and clears loading state when done
func (m *Model) checkLoadingComplete() {
	if m.pendingLoads > 0 {
		m.pendingLoads--
	}
	if m.pendingLoads == 0 {
		m.loadingMessage = ""
	}
}
