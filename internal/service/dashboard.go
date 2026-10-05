package service

import (
	"os"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

// Dashboard is the Master Dashboard snapshot (spec 3.3): header,
// portfolio metrics, the ranked triage queue and recent activity.
// Attention buckets are ordered by triage priority and only non-zero
// buckets appear.
type Dashboard struct {
	Root      string
	Roots     int
	LastScan  time.Time
	Stats     Stats
	Attention []Attention
	Recent    []registry.Activity
}

// Attention is one triage bucket. Keys follow spec 3.3 priority:
// mismatch_missing, unregistered, missing_git, missing_bank,
// missing_map, missing_readme, stale.
type Attention struct {
	Key   string
	Count int
}

// Dashboard returns the snapshot; recent holds the newest events.
func (a *App) Dashboard() (Dashboard, error) {
	st, err := a.Stats(0)
	if err != nil {
		return Dashboard{}, err
	}
	ps, err := a.List(Filter{})
	if err != nil {
		return Dashboard{}, err
	}
	counts := map[string]int{}
	for _, p := range ps {
		if !p.HasGit {
			counts["missing_git"]++
		}
		if !p.HasBank {
			counts["missing_bank"]++
		}
	}
	counts["mismatch_missing"] = st.MissingFolder
	counts["missing_map"] = st.MissingMap
	counts["missing_readme"] = st.MissingReadme
	counts["stale"] = st.Stale
	unregistered := 0
	for _, p := range ps {
		if !p.Registered {
			if _, err := os.Stat(p.Path); err == nil {
				unregistered++
			}
		}
	}
	counts["unregistered"] = unregistered

	d := Dashboard{
		Root:  a.Config.Workspace.Root,
		Roots: 1 + len(a.Config.Workspace.SecondaryRoots),
		Stats: st,
	}
	for _, p := range ps {
		if p.LastScannedAt.After(d.LastScan) {
			d.LastScan = p.LastScannedAt
		}
	}
	for _, key := range []string{"mismatch_missing", "unregistered", "missing_git", "missing_bank", "missing_map", "missing_readme", "stale"} {
		if counts[key] > 0 {
			d.Attention = append(d.Attention, Attention{Key: key, Count: counts[key]})
		}
	}
	if d.Recent, err = a.Registry.RecentActivity(5); err != nil {
		return Dashboard{}, err
	}
	return d, nil
}
