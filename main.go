package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/connorleisz/contexTUI/internal/app"
	"github.com/muesli/termenv"
)

func main() {
	// Respect NO_COLOR environment variable (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	// Default to current directory if no arg provided
	rootPath := "."
	if len(os.Args) > 1 {
		rootPath = os.Args[1]
	}

	p := tea.NewProgram(
		app.NewModel(rootPath),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
