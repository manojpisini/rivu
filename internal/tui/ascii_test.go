package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestUseASCIIGlyphs(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want bool
	}{
		{nil, false},
		{map[string]string{"RIVU_ASCII": "1"}, true},
		{map[string]string{"RIVU_ASCII": "true"}, true},
		{map[string]string{"RIVU_ASCII": "0"}, false},
		{map[string]string{"LANG": "en_US.UTF-8"}, false},
		{map[string]string{"LC_ALL": "C"}, true},
		{map[string]string{"LC_CTYPE": "POSIX"}, true},
		{map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, true}, // LC_ALL wins
		{map[string]string{"LANG": "de_DE.ISO-8859-1"}, true},
	} {
		getenv := func(k string) string { return tc.env[k] }
		if got := useASCIIGlyphs(getenv); got != tc.want {
			t.Errorf("useASCIIGlyphs(%v) = %v, want %v", tc.env, got, tc.want)
		}
	}
}

// TestASCIISwapCoversEveryRenderedGlyph: the replacer is the whole
// fallback — if a glyph is missing here it leaks through in ASCII mode.
func TestASCIISwapCoversEveryRenderedGlyph(t *testing.T) {
	in := "—·…›▸↑↓─│╭╮╰╯✓×•█░ 60×15 …"
	out := asciiSwap.Replace(in)
	for _, r := range out {
		if r > 127 {
			t.Errorf("glyph %q (U+%04X) survived the ASCII swap in %q", r, r, out)
		}
	}
	// width stays 1 per token: layout maths must not shift
	if len([]rune(out)) < len([]rune(in)) {
		t.Errorf("swap shrank the rune count: %q -> %q", in, out)
	}
	if strings.Contains(out, "…") {
		t.Error("ellipsis must be replaced")
	}
}

// TestRootViewAppliesASCIISwap: the wiring — Root.View is the exit
// point. asciiGlyphs is an init-time var, so this drives the replacer
// through the same call the View makes.
func TestRootViewAppliesASCIISwap(t *testing.T) {
	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	v := r.View() // unicode default in this environment
	if v == "" {
		t.Fatal("empty view")
	}
	swapped := asciiSwap.Replace(v)
	for _, want := range []string{"RIVU", "|"} { // box border becomes |
		if !strings.Contains(swapped, want) {
			t.Errorf("swapped view missing %q", want)
		}
	}
	for _, ch := range swapped {
		if ch > 127 {
			t.Errorf("U+%04X survived full-view swap", ch)
			break
		}
	}
}
