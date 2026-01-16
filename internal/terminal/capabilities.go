package terminal

import (
	"os"
	"strings"
)

// GraphicsProtocol represents the terminal's image display capability
type GraphicsProtocol int

const (
	ProtocolNone   GraphicsProtocol = iota
	ProtocolKitty                   // Ghostty, Kitty, WezTerm, Konsole
	ProtocolBlocks                  // Unicode half-block fallback
)

// Capabilities holds detected terminal capabilities
type Capabilities struct {
	Graphics  GraphicsProtocol
	TrueColor bool
}

// Detect probes the terminal environment to determine capabilities
func Detect() Capabilities {
	caps := Capabilities{
		Graphics:  ProtocolBlocks, // Default fallback
		TrueColor: detectTrueColor(),
	}

	// Check for Kitty graphics protocol support
	// Kitty, Ghostty, WezTerm, and Konsole all support Kitty protocol
	if isKittySupported() {
		caps.Graphics = ProtocolKitty
	}

	return caps
}

// isKittySupported checks if the terminal supports Kitty graphics protocol
func isKittySupported() bool {
	// Direct Kitty detection
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}

	// Check TERM_PROGRAM for known supporting terminals
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	switch termProgram {
	case "kitty", "ghostty", "wezterm":
		return true
	}

	// Check TERM for kitty
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "kitty") {
		return true
	}

	// WezTerm specific check
	if os.Getenv("WEZTERM_EXECUTABLE") != "" {
		return true
	}

	// Konsole detection (supports Kitty protocol)
	if os.Getenv("KONSOLE_VERSION") != "" {
		return true
	}

	return false
}

// detectTrueColor checks if terminal supports 24-bit color
func detectTrueColor() bool {
	colorTerm := os.Getenv("COLORTERM")
	if colorTerm == "truecolor" || colorTerm == "24bit" {
		return true
	}

	term := os.Getenv("TERM")
	if strings.Contains(term, "256color") || strings.Contains(term, "truecolor") {
		return true
	}

	return false
}

// String returns a human-readable name for the protocol
func (p GraphicsProtocol) String() string {
	switch p {
	case ProtocolKitty:
		return "Kitty"
	case ProtocolBlocks:
		return "Unicode Blocks"
	default:
		return "None"
	}
}
