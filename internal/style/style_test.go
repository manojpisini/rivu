package style

import (
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var themes = map[string]Theme{
	"graphite-violet": GraphiteViolet,
	"mono":            Mono,
	"light":           Light,
}

func TestEveryThemeDefinesEveryToken(t *testing.T) {
	for name, th := range themes {
		v := reflect.ValueOf(th)
		for i := range v.NumField() {
			fv := v.Field(i)
			field := v.Type().Field(i).Name
			for _, pair := range []string{"Dark", "Light"} {
				if got := fv.FieldByName(pair).String(); strings.TrimSpace(got) == "" {
					t.Errorf("%s.%s.%s is empty", name, field, pair)
				}
			}
		}
	}
	if Pad < 1 || Gap < 1 {
		t.Errorf("spacing tokens must be positive: Pad=%d Gap=%d", Pad, Gap)
	}
}

func TestByName(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    Theme
		wantErr bool
	}{
		{"", GraphiteViolet, false},
		{"graphite-violet", GraphiteViolet, false},
		{"Graphite-Violet ", GraphiteViolet, false},
		{"mono", Mono, false},
		{"LIGHT", Light, false},
		{"neon", Theme{}, true},
	} {
		got, err := ByName(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ByName(%q) = %#v, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ByName(%q): %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("ByName(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

// TestNoColorEnv renders with a fresh renderer under NO_COLOR and must
// get termenv's Ascii profile (the no-color.org contract).
func TestNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	r := lipgloss.NewRenderer(io.Discard)
	if p := r.ColorProfile(); p != termenv.Ascii {
		t.Errorf("profile under NO_COLOR = %v, want Ascii", p)
	}
	out := r.NewStyle().Foreground(GraphiteViolet.Accent).Render("RIVU")
	if strings.Contains(out, "\x1b") {
		t.Errorf("NO_COLOR output contains escapes: %q", out)
	}
}

// TestNoColorFlag covers the --no-color path: the CLI flips the default
// renderer's profile to Ascii and every token must strip.
func TestNoColorFlag(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	lipgloss.SetColorProfile(termenv.Ascii)
	for _, s := range []lipgloss.AdaptiveColor{
		GraphiteViolet.Bg, GraphiteViolet.Accent, Mono.Accent, Light.Bad,
	} {
		out := lipgloss.NewStyle().Foreground(s).Render("x")
		if strings.Contains(out, "\x1b") {
			t.Errorf("Ascii profile leaked escapes for %#v: %q", s, out)
		}
	}
}
