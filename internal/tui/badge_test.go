package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestBadgeFormats pins the badge vocabulary (P10.04: Nerd glyph +
// label, colour carries state): ACTIVE GO Git Bank:OK Map:Missing
// Health:72.
func TestBadgeFormats(t *testing.T) {
	cases := []struct{ got, want string }{
		{stageBadge("active"), iconActive + " ACTIVE"},
		{stageBadge("source"), iconSource + " SOURCE"},
		{stageBadge("research"), iconResearch + " RESEARCH"},
		{stageBadge("maintenance"), iconMaint + " MAINTENANCE"},
		{stageBadge("delta"), iconDelta + " DELTA"},
		{languageBadge("go"), "GO"},
		{languageBadge(""), "UNKNOWN"},
		{gitBadge(true), iconGit + " Git"},
		{gitBadge(false), iconGit + " Git"},
		{bankBadge(true), iconBank + " Bank:OK"},
		{bankBadge(false), iconBank + " Bank:missing"},
		{mapBadge(true), iconMap + " Map:OK"},
		{mapBadge(false), iconMap + " Map:Missing"},
		{healthBadge(72), iconHealth + " Health:72"},
		{healthBadge(0), iconHealth + " Health:0"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("badge = %q, want %q", c.got, c.want)
		}
	}
}

// TestBadgeColours: with colour enabled, distinct semantics get
// distinct escape sequences and every badge carries one.
func TestBadgeColours(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	good := healthBadge(90)
	bad := healthBadge(20)
	if !strings.Contains(good, "\x1b[") || !strings.Contains(bad, "\x1b[") {
		t.Fatalf("badges must carry colour escapes: %q %q", good, bad)
	}
	if good == bad {
		t.Error("good and bad health must render differently")
	}
	if !strings.Contains(good, "Health:90") {
		t.Errorf("colour must not swallow the label: %q", good)
	}
}
