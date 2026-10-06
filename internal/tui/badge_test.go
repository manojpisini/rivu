package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// TestBadgeFormats pins the badge vocabulary from P3.11:
// [ACTIVE] [GO] [Git] [Bank:OK] [Map:Missing] [Health:72].
func TestBadgeFormats(t *testing.T) {
	cases := []struct{ got, want string }{
		{stageBadge("active"), "[ACTIVE]"},
		{stageBadge("source"), "[SOURCE]"},
		{stageBadge("research"), "[RESEARCH]"},
		{stageBadge("maintenance"), "[MAINTENANCE]"},
		{stageBadge("delta"), "[DELTA]"},
		{languageBadge("go"), "[GO]"},
		{languageBadge(""), "[UNKNOWN]"},
		{gitBadge(true), "[Git]"},
		{gitBadge(false), "[Git]"},
		{bankBadge(true), "[Bank:OK]"},
		{bankBadge(false), "[Bank:missing]"},
		{mapBadge(true), "[Map:OK]"},
		{mapBadge(false), "[Map:Missing]"},
		{healthBadge(72), "[Health:72]"},
		{healthBadge(0), "[Health:0]"},
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
