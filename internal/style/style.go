// Package style centralises Rivu's colour and spacing tokens for the TUI.
// Screens compose from a Theme and never hard-code lipgloss colours
// (release plan line 253); a test in internal/tui enforces that.
//
// Colours are AdaptiveColor pairs so a theme reads correctly on light and
// dark terminals. NO_COLOR and --no-color reach these styles through
// termenv: NO_COLOR flips EnvColorProfile to Ascii, and the CLI calls
// lipgloss.SetColorProfile(termenv.Ascii) for --no-color, after which
// every render below strips colour.
package style

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
	// Bg is the application background.
	Bg lipgloss.AdaptiveColor
	// Panel is the panel background, one step up from Bg.
	Panel lipgloss.AdaptiveColor
	// Border is the subtle gray panel border.
	Border lipgloss.AdaptiveColor
	// Text is the primary text colour.
	Text lipgloss.AdaptiveColor
	// Muted is the secondary text colour.
	Muted lipgloss.AdaptiveColor
	// Accent is violet, Rivu's brand colour.
	Accent lipgloss.AdaptiveColor
	// Good is the green success colour.
	Good lipgloss.AdaptiveColor
	// Warn is the yellow warning colour.
	Warn lipgloss.AdaptiveColor
	// Bad is the red failure colour.
	Bad lipgloss.AdaptiveColor
}

// GraphiteViolet is the default theme (config: theme = "graphite-violet",
// spec 3.10 visual direction): dark-first graphite with a light-terminal
// variant so text keeps contrast where the terminal shows through.
var GraphiteViolet = Theme{
	Bg:     lipgloss.AdaptiveColor{Dark: "#1b1b1e", Light: "#f5f5f7"},
	Panel:  lipgloss.AdaptiveColor{Dark: "#232327", Light: "#ffffff"},
	Border: lipgloss.AdaptiveColor{Dark: "#3c3c41", Light: "#d4d4da"},
	Text:   lipgloss.AdaptiveColor{Dark: "#e4e4e8", Light: "#26262b"},
	Muted:  lipgloss.AdaptiveColor{Dark: "#8a8a90", Light: "#6a6a72"},
	Accent: lipgloss.AdaptiveColor{Dark: "#8f6dff", Light: "#6d46e0"},
	Good:   lipgloss.AdaptiveColor{Dark: "#4ec97a", Light: "#137333"},
	Warn:   lipgloss.AdaptiveColor{Dark: "#e0b34e", Light: "#9a6700"},
	Bad:    lipgloss.AdaptiveColor{Dark: "#e05a5a", Light: "#cf222e"},
}

// Mono is the no-hue theme: grayscale pairs, identical on purpose so the
// look does not depend on the terminal background.
var Mono = Theme{
	Bg:     lipgloss.AdaptiveColor{Dark: "#101014", Light: "#101014"},
	Panel:  lipgloss.AdaptiveColor{Dark: "#1a1a1e", Light: "#1a1a1e"},
	Border: lipgloss.AdaptiveColor{Dark: "#3c3c41", Light: "#3c3c41"},
	Text:   lipgloss.AdaptiveColor{Dark: "#d8d8dc", Light: "#d8d8dc"},
	Muted:  lipgloss.AdaptiveColor{Dark: "#8a8a90", Light: "#8a8a90"},
	Accent: lipgloss.AdaptiveColor{Dark: "#ffffff", Light: "#ffffff"},
	Good:   lipgloss.AdaptiveColor{Dark: "#b8b8bc", Light: "#b8b8bc"},
	Warn:   lipgloss.AdaptiveColor{Dark: "#969699", Light: "#969699"},
	Bad:    lipgloss.AdaptiveColor{Dark: "#e8e8ec", Light: "#e8e8ec"},
}

// Light forces the light palette on every terminal — an explicit user
// choice, not an adaptation, so both pairs are the same.
var Light = Theme{
	Bg:     lipgloss.AdaptiveColor{Dark: "#f5f5f7", Light: "#f5f5f7"},
	Panel:  lipgloss.AdaptiveColor{Dark: "#ffffff", Light: "#ffffff"},
	Border: lipgloss.AdaptiveColor{Dark: "#d4d4da", Light: "#d4d4da"},
	Text:   lipgloss.AdaptiveColor{Dark: "#26262b", Light: "#26262b"},
	Muted:  lipgloss.AdaptiveColor{Dark: "#6a6a72", Light: "#6a6a72"},
	Accent: lipgloss.AdaptiveColor{Dark: "#6d46e0", Light: "#6d46e0"},
	Good:   lipgloss.AdaptiveColor{Dark: "#137333", Light: "#137333"},
	Warn:   lipgloss.AdaptiveColor{Dark: "#9a6700", Light: "#9a6700"},
	Bad:    lipgloss.AdaptiveColor{Dark: "#cf222e", Light: "#cf222e"},
}

// ByName returns the named theme for config appearance.theme; "" selects
// the default and names are matched case-insensitively.
func ByName(name string) (Theme, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "graphite-violet":
		return GraphiteViolet, nil
	case "mono":
		return Mono, nil
	case "light":
		return Light, nil
	}
	return Theme{}, fmt.Errorf("unknown theme %q: want graphite-violet, mono, or light", name)
}
