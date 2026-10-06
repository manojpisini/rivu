// Package style centralises Rivu's colour and spacing tokens for the TUI.
// Screens compose from a Theme and never hard-code lipgloss colours
// (release plan line 253); a test in internal/tui enforces that.
package style

import "github.com/charmbracelet/lipgloss"

// Spacing tokens, in cells.
const (
	// Pad is the inner horizontal padding of a panel.
	Pad = 1
	// Gap is the space between adjacent panes.
	Gap = 1
)

// Theme is the named token set a screen renders with, per spec 2.11:
// graphite background, violet accent, off-white text, and the three
// status colours for badges and health output.
type Theme struct {
	// Bg is the near-black graphite application background.
	Bg lipgloss.Color
	// Panel is the panel background, one step up from Bg.
	Panel lipgloss.Color
	// Border is the subtle gray panel border.
	Border lipgloss.Color
	// Text is the off-white primary text colour.
	Text lipgloss.Color
	// Muted is the gray secondary text colour.
	Muted lipgloss.Color
	// Accent is violet, Rivu's brand colour.
	Accent lipgloss.Color
	// Good is the green success colour.
	Good lipgloss.Color
	// Warn is the yellow warning colour.
	Warn lipgloss.Color
	// Bad is the red failure colour.
	Bad lipgloss.Color
}

// GraphiteViolet is the default theme (config: theme = "graphite-violet",
// spec 3.10 visual direction).
var GraphiteViolet = Theme{
	Bg:     lipgloss.Color("#1b1b1e"),
	Panel:  lipgloss.Color("#232327"),
	Border: lipgloss.Color("#3c3c41"),
	Text:   lipgloss.Color("#e4e4e8"),
	Muted:  lipgloss.Color("#8a8a90"),
	Accent: lipgloss.Color("#8f6dff"),
	Good:   lipgloss.Color("#4ec97a"),
	Warn:   lipgloss.Color("#e0b34e"),
	Bad:    lipgloss.Color("#e05a5a"),
}
