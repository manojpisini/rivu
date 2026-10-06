package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/registry"
)

func scrollModel(count int) Model {
	var ps []registry.Project
	for i := range count {
		ps = append(ps, registry.Project{ID: fmt.Sprint(i), Name: fmt.Sprintf("proj-%03d", i), FlowStage: "active"})
	}
	return New(ps, `C:\ws`)
}

func titleLine(t *testing.T, panel string) string {
	t.Helper()
	for _, line := range strings.Split(panel, "\n") {
		if strings.Contains(line, "PROJECTS") {
			return line
		}
	}
	t.Fatal("no title line found")
	return ""
}

// TestPositionAndScrollIndicators: the title carries n/N position and
// arrows whenever rows exist above or below the window (P3.10).
func TestPositionAndScrollIndicators(t *testing.T) {
	m := scrollModel(500)
	const h = 30
	avail := h - 6

	// Cursor at the top: 1/500, ↓ for everything below, no ↑.
	line := titleLine(t, m.projectPanel(80, h))
	if !regexp.MustCompile(`1/500`).MatchString(line) {
		t.Errorf("top position missing: %q", line)
	}
	if strings.Contains(line, "↑") {
		t.Errorf("no ↑ expected at the top: %q", line)
	}
	if !strings.Contains(line, fmt.Sprintf("↓%d", 500-avail)) {
		t.Errorf("↓ count missing at the top: %q", line)
	}

	// Middle: both arrows.
	m.Cursor = 250
	line = titleLine(t, m.projectPanel(80, h))
	if !regexp.MustCompile(`251/500`).MatchString(line) {
		t.Errorf("middle position missing: %q", line)
	}
	if !strings.Contains(line, "↑") || !strings.Contains(line, "↓") {
		t.Errorf("middle needs both indicators: %q", line)
	}

	// Bottom: n/N at the end, ↑ only.
	m.Cursor = 499
	line = titleLine(t, m.projectPanel(80, h))
	if !regexp.MustCompile(`500/500`).MatchString(line) {
		t.Errorf("end position missing: %q", line)
	}
	if strings.Contains(line, "↓") {
		t.Errorf("no ↓ expected at the end: %q", line)
	}
	if !strings.Contains(line, "↑") {
		t.Errorf("↑ expected at the end: %q", line)
	}
}

// TestPositionEmptyList: no division by zero on an empty view.
func TestPositionEmptyList(t *testing.T) {
	m := New(nil, `C:\ws`)
	line := titleLine(t, m.projectPanel(80, 20))
	if !strings.Contains(line, "0/0") {
		t.Errorf("empty list must show 0/0: %q", line)
	}
}

// TestIndicatorsMatchRenderedWindow: the ↑ count equals the number of
// hidden rows above the first rendered row.
func TestIndicatorsMatchRenderedWindow(t *testing.T) {
	m := scrollModel(100)
	m.Cursor = 30
	panel := m.projectPanel(80, 20) // available = 14, start = 17
	if strings.Contains(panel, "proj-016") {
		t.Error("row 16 must be scrolled out above")
	}
	if !strings.Contains(panel, "proj-017") {
		t.Error("row 17 must be the first rendered row")
	}
	line := titleLine(t, panel)
	if !strings.Contains(line, "↑17") {
		t.Errorf("↑17 expected, got %q", line)
	}
}
