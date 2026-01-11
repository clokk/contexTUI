package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type pane int

const (
	treePane pane = iota
	previewPane
)

type model struct {
	rootPath    string
	entries     []entry
	cursor      int
	activePane  pane
	preview     viewport.Model
	previewContent string
	width       int
	height      int
	ready       bool
}

type entry struct {
	name     string
	path     string
	isDir    bool
	depth    int
	expanded bool
	children []entry
}

func initialModel(rootPath string) model {
	absPath, _ := filepath.Abs(rootPath)
	entries := loadDirectory(absPath, 0)

	return model{
		rootPath: absPath,
		entries:  entries,
		cursor:   0,
		activePane: treePane,
	}
}

func loadDirectory(path string, depth int) []entry {
	var entries []entry

	files, err := os.ReadDir(path)
	if err != nil {
		return entries
	}

	for _, f := range files {
		// Skip hidden files and common ignores
		if strings.HasPrefix(f.Name(), ".") {
			continue
		}
		if f.Name() == "node_modules" || f.Name() == "vendor" || f.Name() == "__pycache__" {
			continue
		}

		e := entry{
			name:  f.Name(),
			path:  filepath.Join(path, f.Name()),
			isDir: f.IsDir(),
			depth: depth,
		}
		entries = append(entries, e)
	}

	return entries
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "tab":
			if m.activePane == treePane {
				m.activePane = previewPane
			} else {
				m.activePane = treePane
			}

		case "j", "down":
			if m.activePane == treePane {
				if m.cursor < len(m.flatEntries())-1 {
					m.cursor++
					m = m.updatePreview()
				}
			} else {
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				cmds = append(cmds, cmd)
			}

		case "k", "up":
			if m.activePane == treePane {
				if m.cursor > 0 {
					m.cursor--
					m = m.updatePreview()
				}
			} else {
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				cmds = append(cmds, cmd)
			}

		case "enter", "l", "right":
			if m.activePane == treePane {
				flat := m.flatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.isDir {
						m = m.toggleExpand(e.path)
					}
				}
			}

		case "h", "left":
			if m.activePane == treePane {
				flat := m.flatEntries()
				if m.cursor < len(flat) {
					e := flat[m.cursor]
					if e.isDir {
						m = m.collapse(e.path)
					}
				}
			}

		case "c":
			// Copy current file to clipboard
			flat := m.flatEntries()
			if m.cursor < len(flat) {
				e := flat[m.cursor]
				if !e.isDir {
					copyToClipboard(e.path)
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 3
		footerHeight := 2
		previewWidth := m.width/2 - 2
		previewHeight := m.height - headerHeight - footerHeight

		if !m.ready {
			m.preview = viewport.New(previewWidth, previewHeight)
			m.preview.SetContent("Select a file to preview")
			m.ready = true
		} else {
			m.preview.Width = previewWidth
			m.preview.Height = previewHeight
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) flatEntries() []entry {
	return flattenEntries(m.entries)
}

func flattenEntries(entries []entry) []entry {
	var flat []entry
	for _, e := range entries {
		flat = append(flat, e)
		if e.isDir && e.expanded {
			flat = append(flat, flattenEntries(e.children)...)
		}
	}
	return flat
}

func (m model) toggleExpand(path string) model {
	m.entries = toggleExpandRecursive(m.entries, path)
	return m
}

func toggleExpandRecursive(entries []entry, path string) []entry {
	for i, e := range entries {
		if e.path == path && e.isDir {
			if e.expanded {
				entries[i].expanded = false
				entries[i].children = nil
			} else {
				entries[i].expanded = true
				entries[i].children = loadDirectory(path, e.depth+1)
			}
			return entries
		}
		if e.expanded && len(e.children) > 0 {
			entries[i].children = toggleExpandRecursive(e.children, path)
		}
	}
	return entries
}

func (m model) collapse(path string) model {
	m.entries = collapseRecursive(m.entries, path)
	return m
}

func collapseRecursive(entries []entry, path string) []entry {
	for i, e := range entries {
		if e.path == path && e.isDir && e.expanded {
			entries[i].expanded = false
			entries[i].children = nil
			return entries
		}
		if e.expanded && len(e.children) > 0 {
			entries[i].children = collapseRecursive(e.children, path)
		}
	}
	return entries
}

func (m model) updatePreview() model {
	flat := m.flatEntries()
	if m.cursor >= len(flat) {
		return m
	}

	e := flat[m.cursor]
	if e.isDir {
		m.preview.SetContent("Directory: " + e.name)
		return m
	}

	content, err := os.ReadFile(e.path)
	if err != nil {
		m.preview.SetContent("Error reading file: " + err.Error())
		return m
	}

	// Render markdown files with glamour
	if strings.HasSuffix(e.name, ".md") {
		renderer, _ := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.preview.Width),
		)
		rendered, err := renderer.Render(string(content))
		if err == nil {
			m.preview.SetContent(rendered)
			return m
		}
	}

	m.preview.SetContent(string(content))
	m.previewContent = string(content)
	return m
}

func (m model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205")).
		Padding(0, 1)

	header := headerStyle.Render("contexTUI") +
		lipgloss.NewStyle().Faint(true).Render(" " + m.rootPath)

	// Tree pane
	treeWidth := m.width/2 - 1
	treeStyle := lipgloss.NewStyle().
		Width(treeWidth).
		Height(m.height - 5).
		Padding(0, 1)

	if m.activePane == treePane {
		treeStyle = treeStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	} else {
		treeStyle = treeStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
	}

	treeContent := m.renderTree()
	tree := treeStyle.Render(treeContent)

	// Preview pane
	previewStyle := lipgloss.NewStyle().
		Width(m.width/2 - 1).
		Height(m.height - 5).
		Padding(0, 1)

	if m.activePane == previewPane {
		previewStyle = previewStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	} else {
		previewStyle = previewStyle.BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
	}

	preview := previewStyle.Render(m.preview.View())

	// Footer
	footerStyle := lipgloss.NewStyle().Faint(true).Padding(0, 1)
	footer := footerStyle.Render("[tab] switch pane  [j/k] navigate  [enter] expand  [c] copy  [q] quit")

	// Compose layout
	body := lipgloss.JoinHorizontal(lipgloss.Top, tree, preview)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m model) renderTree() string {
	var b strings.Builder
	flat := m.flatEntries()

	for i, e := range flat {
		indent := strings.Repeat("  ", e.depth)

		icon := "  "
		if e.isDir {
			if e.expanded {
				icon = "v "
			} else {
				icon = "> "
			}
		}

		line := indent + icon + e.name

		if i == m.cursor {
			style := lipgloss.NewStyle().
				Background(lipgloss.Color("205")).
				Foreground(lipgloss.Color("0"))
			line = style.Render(line)
		} else if e.isDir {
			style := lipgloss.NewStyle().Bold(true)
			line = style.Render(line)
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

func copyToClipboard(path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	// Format for Claude
	ext := filepath.Ext(path)
	formatted := fmt.Sprintf("```%s\n// %s\n%s\n```",
		strings.TrimPrefix(ext, "."),
		path,
		string(content))

	// Use pbcopy on macOS
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(formatted)
	cmd.Run()
}
