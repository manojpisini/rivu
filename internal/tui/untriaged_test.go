package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
)

// TestUntriagedFilter (P4.12): u toggles the smart filter — only
// Source projects created past the SLA survive; toggling off restores
// the list and the status line.
func TestUntriagedFilter(t *testing.T) {
	m, _ := pickFixture(t)
	m.cfg = config.Default() // flow.source_sla_days = 14
	now := time.Now()
	m.Projects[0].CreatedAt = now.Add(-20 * 24 * time.Hour) // past SLA
	m.Projects[1].CreatedAt = now.Add(-2 * 24 * time.Hour)  // still fresh
	m.applyFilter()
	if len(m.Visible) != 2 {
		t.Fatalf("setup: Visible = %d, want 2", len(m.Visible))
	}

	m, cmd := updateC(t, m, keyR('u'))
	if cmd != nil || !m.Untriaged {
		t.Fatalf("u must toggle the filter: cmd=%v untriaged=%v", cmd, m.Untriaged)
	}
	if len(m.Visible) != 1 || m.Visible[0].Slug != "a" {
		t.Fatalf("Visible = %+v, want only the past-SLA project", m.Visible)
	}
	if !strings.Contains(m.Status, "Untriaged on") || !strings.Contains(m.Status, "14d") {
		t.Errorf("Status = %q, want the SLA notice", m.Status)
	}

	m, _ = updateC(t, m, keyR('u'))
	if m.Untriaged || len(m.Visible) != 2 || m.Status != "" {
		t.Fatalf("toggle off: untriaged=%v visible=%d status=%q", m.Untriaged, len(m.Visible), m.Status)
	}
}

// TestUntriagedNeedsSourceAndDate: a Delta-stage or undated project is
// never untriaged; the zero config still uses the 14-day default.
func TestUntriagedNeedsSourceAndDate(t *testing.T) {
	now := time.Now()
	old := now.Add(-90 * 24 * time.Hour)
	cases := []struct {
		name string
		p    registry.Project
		want bool
	}{
		{"source past SLA", registry.Project{FlowStage: "source", CreatedAt: old}, true},
		{"source fresh", registry.Project{FlowStage: "source", CreatedAt: now}, false},
		{"active past SLA", registry.Project{FlowStage: "active", CreatedAt: old}, false},
		{"source undated", registry.Project{FlowStage: "source"}, false},
	}
	for _, tc := range cases {
		if got := isUntriaged(tc.p, 14, now); got != tc.want {
			t.Errorf("%s: isUntriaged = %v, want %v", tc.name, got, tc.want)
		}
	}

	// zero cfg: applyFilter falls back to the 14-day default
	m, _ := pickFixture(t) // cfg is zero here
	m.Projects[0].CreatedAt = now.Add(-20 * 24 * time.Hour)
	m.Projects[1].CreatedAt = now.Add(-3 * 24 * time.Hour)
	m.applyFilter()
	m, _ = updateC(t, m, keyR('u'))
	if len(m.Visible) != 1 || m.Visible[0].Slug != "a" {
		t.Fatalf("zero cfg Visible = %+v, want the 14-day default result", m.Visible)
	}
}
