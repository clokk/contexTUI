package styles

import "github.com/charmbracelet/lipgloss"

// Color constants used throughout the UI
var (
	// Primary colors
	Accent      = lipgloss.Color("205") // Pink/Magenta - primary accent
	AccentAlt   = lipgloss.Color("141") // Purple - secondary accent
	Success     = lipgloss.Color("118") // Green - success states
	SuccessBold = lipgloss.Color("82")  // Bright green - strong success
	Warning     = lipgloss.Color("214") // Orange - warnings
	Error       = lipgloss.Color("196") // Red - errors, deletions
	Info        = lipgloss.Color("75")  // Blue - informational

	// Neutral colors
	TextNormal   = lipgloss.Color("252") // Light gray - normal text
	TextMuted    = lipgloss.Color("250") // Lighter gray - descriptions
	TextFaint    = lipgloss.Color("244") // Gray - faint/disabled text
	TextOnAccent = lipgloss.Color("0")   // Black - text on accent background

	// Border colors
	BorderActive   = lipgloss.Color("205") // Pink - active borders
	BorderInactive = lipgloss.Color("240") // Dark gray - inactive borders

	// Git status colors
	GitModified  = lipgloss.Color("226") // Yellow
	GitAdded     = lipgloss.Color("118") // Green
	GitDeleted   = lipgloss.Color("196") // Red
	GitRenamed   = lipgloss.Color("75")  // Blue
	GitUntracked = lipgloss.Color("244") // Gray
	GitConflict  = lipgloss.Color("196") // Red
)

// Common style components
var (
	// Headers and titles
	Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(Accent)

	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(Accent)

	SectionHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(AccentAlt)

	// Text styles
	Normal = lipgloss.NewStyle().
		Foreground(TextNormal)

	Muted = lipgloss.NewStyle().
		Foreground(TextMuted)

	Faint = lipgloss.NewStyle().
		Faint(true)

	// Selection and highlighting
	Selected = lipgloss.NewStyle().
			Background(Accent).
			Foreground(TextOnAccent)

	Highlight = lipgloss.NewStyle().
			Background(Accent).
			Foreground(TextOnAccent)

	// Status indicators
	StatusSuccess = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	StatusWarning = lipgloss.NewStyle().
			Foreground(Warning)

	StatusError = lipgloss.NewStyle().
			Foreground(Error)

	// Keys in help text
	Key = lipgloss.NewStyle().
		Foreground(GitModified)

	// Branch display
	Branch = lipgloss.NewStyle().
		Foreground(AccentAlt).
		Bold(true)
)

// Border styles for panes
func ActiveBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(BorderActive)
}

func InactiveBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(BorderInactive)
}

// GitStatusStyles returns styles for git status indicators
func GitStatusStyles() map[string]lipgloss.Style {
	return map[string]lipgloss.Style{
		"M": lipgloss.NewStyle().Foreground(GitModified).Bold(true),
		"A": lipgloss.NewStyle().Foreground(GitAdded).Bold(true),
		"D": lipgloss.NewStyle().Foreground(GitDeleted).Bold(true),
		"R": lipgloss.NewStyle().Foreground(GitRenamed).Bold(true),
		"?": lipgloss.NewStyle().Foreground(GitUntracked),
		"U": lipgloss.NewStyle().Foreground(GitConflict).Bold(true),
		"!": lipgloss.NewStyle().Foreground(GitConflict).Bold(true),
	}
}
