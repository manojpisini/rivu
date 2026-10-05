package service

import (
	"slices"
	"strings"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

// Filter narrows and orders the project list for CLI and TUI alike
// (P2.12; the TUI's search syntax builds on the same fields). The zero
// Filter returns every project in registry lifecycle order.
type Filter struct {
	Flow       string // exact stage: source|active|maintenance|research|delta
	Lang       string // exact language, case-insensitive
	Unhealthy  bool   // health score below unhealthyBelow
	Stale      bool   // no activity within the configured stale threshold
	Confluence string // member of this confluence (name or id)
	Sort       string // name|health|opened|created; "" keeps lifecycle order
}

// unhealthyBelow is the health cutoff for --unhealthy (bands: <60 needs
// attention, 60-79 ok, 80+ healthy).
const unhealthyBelow = 60

// List returns projects narrowed and ordered by f. The zero filter is
// the plain lifecycle listing used by launchTUI, stats and dashboard.
func (a *App) List(f Filter) ([]registry.Project, error) {
	ps, err := a.Registry.List()
	if err != nil {
		return nil, err
	}
	if f.Confluence != "" {
		ids, err := a.Registry.ConfluenceProjectIDs(f.Confluence)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool, len(ids))
		for _, id := range ids {
			set[id] = true
		}
		ps = slices.DeleteFunc(ps, func(p registry.Project) bool { return !set[p.ID] })
	}
	if f.Flow != "" {
		ps = slices.DeleteFunc(ps, func(p registry.Project) bool { return p.FlowStage != f.Flow })
	}
	if f.Lang != "" {
		lang := strings.ToLower(f.Lang)
		ps = slices.DeleteFunc(ps, func(p registry.Project) bool { return strings.ToLower(p.Language) != lang })
	}
	if f.Unhealthy {
		ps = slices.DeleteFunc(ps, func(p registry.Project) bool { return p.HealthScore >= unhealthyBelow })
	}
	if f.Stale {
		threshold := time.Now().AddDate(0, 0, -a.Config.Flow.StaleThresholdDays)
		ps = slices.DeleteFunc(ps, func(p registry.Project) bool {
			last := p.LastOpenedAt
			if last.IsZero() {
				last = p.CreatedAt
			}
			return last.After(threshold)
		})
	}
	sortProjects(ps, f.Sort)
	return ps, nil
}

// sortProjects orders in place; an empty key keeps registry order.
func sortProjects(ps []registry.Project, key string) {
	switch key {
	case "name":
		slices.SortFunc(ps, func(a, b registry.Project) int {
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})
	case "health": // worst first — attention at the top
		slices.SortFunc(ps, func(a, b registry.Project) int { return a.HealthScore - b.HealthScore })
	case "opened": // most recent first; never-opened last
		slices.SortFunc(ps, func(a, b registry.Project) int { return b.LastOpenedAt.Compare(a.LastOpenedAt) })
	case "created": // newest first
		slices.SortFunc(ps, func(a, b registry.Project) int { return b.CreatedAt.Compare(a.CreatedAt) })
	}
}
