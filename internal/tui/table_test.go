package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/registry"
)

func TestColumnTiers(t *testing.T) {
	cases := []struct {
		inner int
		want  tableCols
	}{
		{90, tableCols{true, true, true}},
		{70, tableCols{true, true, true}},
		{69, tableCols{false, true, true}},
		{55, tableCols{false, true, true}},
		{54, tableCols{false, false, true}},
		{44, tableCols{false, false, true}},
		{43, tableCols{false, false, false}},
	}
	for _, c := range cases {
		if got := columnsFor(c.inner); got != c.want {
			t.Errorf("columnsFor(%d) = %+v, want %+v", c.inner, got, c.want)
		}
	}
}

// TestTableFitsDisplayWidth: rows must fit the panel using
// display-width truncation (CJK names, long paths) — no %-24s byte
// padding, no column overflow.
func TestTableFitsDisplayWidth(t *testing.T) {
	long := registry.Project{
		ID: "1", Name: "日本語のとても長いプロジェクト名", Language: "TypeScript",
		Channel: "active/research-projects", Path: `C:\Users\someone\projects\very-long-project-directory-name`,
		FlowStage: "active", HealthScore: 72,
	}
	for _, w := range []int{40, 48, 58, 74, 96} {
		t.Run(fmt.Sprintf("w%d", w), func(t *testing.T) {
			m := New([]registry.Project{long}, `C:\projects`)
			panel := m.projectPanel(w, 24)
			for _, line := range strings.Split(panel, "\n") {
				if n := lipgloss.Width(line); n > w {
					t.Errorf("line %q is %d cells, panel is %d", line, n, w)
				}
				if strings.ContainsRune(line, '�') {
					t.Errorf("line %q was cut mid-rune", line)
				}
			}
		})
	}
}

// TestVirtualisedRows: only the window around the cursor renders, so a
// huge list never materialises every row.
func TestVirtualisedRows(t *testing.T) {
	var ps []registry.Project
	for i := range 500 {
		ps = append(ps, registry.Project{ID: fmt.Sprint(i), Name: fmt.Sprintf("proj-%03d", i), FlowStage: "active"})
	}
	m := New(ps, `C:\ws`)
	m.Cursor = 0
	panel := m.projectPanel(80, 30)
	if !strings.Contains(panel, "proj-000") {
		t.Error("first row must be visible at cursor 0")
	}
	if strings.Contains(panel, "proj-400") {
		t.Error("row 400 must not render while the cursor is at 0")
	}
	m.Cursor = 499
	panel = m.projectPanel(80, 30)
	if !strings.Contains(panel, "proj-499") {
		t.Error("last row must be visible at the end")
	}
	if strings.Contains(panel, "proj-000") {
		t.Error("row 0 must scroll out of the window")
	}
}

// TestPathKeepsTail: paths truncate from the start so the project
// directory stays visible.
func TestPathKeepsTail(t *testing.T) {
	got := padStartCell(`C:\very\long\path\to\rivu`, 12)
	if !strings.HasSuffix(got, "rivu") {
		t.Errorf("path cell = %q, want the tail preserved", got)
	}
	if lipgloss.Width(got) != 12 {
		t.Errorf("path cell width = %d, want 12", lipgloss.Width(got))
	}
}
