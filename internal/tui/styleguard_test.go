package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoInlineLipglossColours enforces release plan line 253: screens
// compose from the internal/style tokens and never hard-code colours.
func TestNoInlineLipglossColours(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "lipgloss.Color(") {
			t.Errorf("%s hard-codes a colour; use the tokens from internal/style", name)
		}
	}
}
