package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/service"
)

func TestConfirmPlanRendersSourcePlan(t *testing.T) {
	r, _ := rootOf(t)
	yes := 0
	plan := service.SourcePlan{
		Name: "demo", Channel: "01_Active",
		Create:   []string{"01_Active/demo"},
		Write:    []string{"01_Active/demo/README.md"},
		Run:      []string{"git init 01_Active/demo"},
		Registry: []string{"insert demo"},
		Bank:     []string{".metadata/project.toml"},
	}
	r, _ = upd(t, r, ConfirmPlan("Source demo?", sourcePlanLines(plan), func() tea.Cmd {
		yes++
		return nil
	})())
	v := r.View()
	for _, want := range []string{
		"Source demo into 01_Active",
		"Will create 01_Active/demo",
		"Will write 01_Active/demo/README.md",
		"Will run git init 01_Active/demo",
		"Registry: insert demo",
		"Bank: .metadata/project.toml",
		"n/esc/enter no (default)",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("modal missing %q in %q", want, v)
		}
	}
	// default No: n closes without acting
	r, _ = upd(t, r, keyR('n'))
	if yes != 0 || len(r.confirms) != 0 {
		t.Fatalf("n must decline, yes=%d stack=%d", yes, len(r.confirms))
	}
}

func TestConfirmPlanRendersFlowPlan(t *testing.T) {
	r, _ := rootOf(t)
	plan := service.FlowPlan{
		Query: "demo", FromStage: "source", ToStage: "active",
		Move:     []string{"demo -> 01_Active/demo"},
		Registry: []string{"update demo flow_stage"},
		Bank:     []string{"update .metadata/project.toml"},
	}
	r, _ = upd(t, r, ConfirmPlan("Flow demo?", flowPlanLines(plan), nil)())
	v := r.View()
	for _, want := range []string{
		"Flow demo: source -> active",
		"Will move demo -> 01_Active/demo",
		"Registry: update demo flow_stage",
		"Bank: update .metadata/project.toml",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("modal missing %q in %q", want, v)
		}
	}
}

func TestConfirmPlanScrolls(t *testing.T) {
	r, _ := rootOf(t)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i+1)
	}
	r, _ = upd(t, r, ConfirmPlan("Big plan?", lines, nil)())
	v := r.View()
	if !strings.Contains(v, "line 1") || strings.Contains(v, "line 30") {
		t.Fatalf("first page should show line 1, not line 30: %q", v)
	}
	if !strings.Contains(v, "1-12 of 30") {
		t.Fatalf("position indicator missing: %q", v)
	}

	for range 25 {
		r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyDown})
	}
	v = r.View()
	if !strings.Contains(v, "line 30") {
		t.Fatalf("scrolled page should show the end: %q", v)
	}
	if !strings.Contains(v, "19-30 of 30") {
		t.Fatalf("scroll must clamp at the last page: %q", v)
	}
	if r.confirms[0].scroll != 18 {
		t.Fatalf("scroll = %d, want clamped 18", r.confirms[0].scroll)
	}

	for range 50 {
		r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyUp})
	}
	if r.confirms[0].scroll != 0 {
		t.Fatalf("up must clamp at 0, got %d", r.confirms[0].scroll)
	}
	if !strings.Contains(r.View(), "1-12 of 30") {
		t.Fatal("first page must come back")
	}

	// y accepts after scrolling; scroll never swallows the decision keys
	r, cmd := upd(t, r, keyR('y'))
	if len(r.confirms) != 0 {
		t.Fatal("y must close the modal")
	}
	if cmd != nil {
		t.Fatal("OnYes was nil, no command expected")
	}
}
