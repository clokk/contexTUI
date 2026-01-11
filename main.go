package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Default to current directory if no arg provided
	rootPath := "."
	if len(os.Args) > 1 {
		rootPath = os.Args[1]
	}

	p := tea.NewProgram(
		initialModel(rootPath),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
