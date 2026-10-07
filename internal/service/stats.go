package service

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

// Stats is the portfolio summary behind `rivu stats` and the TUI Stats
// screen (spec 3.6, P2.20). Every count is over the range window;
// Stale uses the configured stale threshold.
type Stats struct {
	Range         string  `json:"range"`
	Total         int     `json:"total"`
	AvgHealth     int     `json:"avg_health"`
	MedianHealth  int     `json:"median_health"`
	Stale         int     `json:"stale"`
	MissingFolder int     `json:"missing_folder"`
	MissingMap    int     `json:"missing_map"`
	MissingReadme int     `json:"missing_readme"`
	ByFlow        []Count `json:"by_flow"`
	ByLanguage    []Count `json:"by_language"`
	ByHealth      []Count `json:"by_health"`
}

// Count is one named bucket of a breakdown, ordered for stable text,
// CSV and JSON output.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// healthBands are the spec 3.6 buckets, worst to best display order.
var healthBands = []string{"90-100", "70-89", "50-69", "0-49"}

// Stats returns portfolio metrics over projects with activity (open or
// create) within the last days; days <= 0 means all time. Missing
// counts check the filesystem live, so a vanished folder shows up
// before the next scan.
func (a *App) Stats(days int) (Stats, error) {
	ps, err := a.List(Filter{})
	if err != nil {
		return Stats{}, err
	}
	rangeLabel := "all"
	if days > 0 {
		rangeLabel = fmt.Sprintf("%dd", days)
		cutoff := time.Now().AddDate(0, 0, -days)
		ps = slices.DeleteFunc(ps, func(p registry.Project) bool { return activity(p).Before(cutoff) })
	}
	out := Stats{
		Range:    rangeLabel,
		Total:    len(ps),
		ByFlow:   make([]Count, 0, len(Stages)),
		ByHealth: make([]Count, 0, len(healthBands)),
	}
	langCounts := map[string]int{}
	healths := make([]int, 0, len(ps))
	sum := 0
	threshold := time.Now().AddDate(0, 0, -a.Config.Flow.StaleThresholdDays)
	for _, p := range ps {
		healths = append(healths, p.HealthScore)
		sum += p.HealthScore
		if !activity(p).After(threshold) {
			out.Stale++
		}
		if _, err := os.Stat(p.Path); err != nil {
			out.MissingFolder++
		} else {
			if !p.HasMap {
				out.MissingMap++
			}
			if _, err := os.Stat(filepath.Join(p.Path, "README.md")); err != nil {
				out.MissingReadme++
			}
		}
		lang := p.Language
		if lang == "" {
			lang = "unknown"
		}
		langCounts[lang]++
	}
	if out.Total > 0 {
		out.AvgHealth = sum / out.Total
		out.MedianHealth = median(healths)
	}
	for _, stage := range Stages {
		n := 0
		for _, p := range ps {
			if p.FlowStage == stage {
				n++
			}
		}
		out.ByFlow = append(out.ByFlow, Count{Name: stage, Count: n})
	}
	for name, n := range langCounts {
		out.ByLanguage = append(out.ByLanguage, Count{Name: name, Count: n})
	}
	slices.SortFunc(out.ByLanguage, func(x, y Count) int {
		if x.Count != y.Count {
			return y.Count - x.Count
		}
		return strings.Compare(x.Name, y.Name)
	})
	for _, band := range healthBands {
		n := 0
		for _, h := range healths {
			if healthBand(h) == band {
				n++
			}
		}
		out.ByHealth = append(out.ByHealth, Count{Name: band, Count: n})
	}
	return out, nil
}

// activity is the timestamp range filters key on: the most recent of
// create or open.
func activity(p registry.Project) time.Time {
	if p.LastOpenedAt.After(p.CreatedAt) {
		return p.LastOpenedAt
	}
	return p.CreatedAt
}

// median health for an even count is the mean of the two middles.
func median(healths []int) int {
	if len(healths) == 0 {
		return 0
	}
	sorted := slices.Clone(healths)
	slices.Sort(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

func healthBand(h int) string {
	switch {
	case h >= 90:
		return "90-100"
	case h >= 70:
		return "70-89"
	case h >= 50:
		return "50-69"
	default:
		return "0-49"
	}
}

// StatsJSON writes the schema-1 document shared by `rivu stats --json`
// and the Stats screen export (P4.20). ByLanguage is never null so
// scripts always get an array.
func StatsJSON(w io.Writer, st Stats) error {
	if st.ByLanguage == nil {
		st.ByLanguage = []Count{}
	}
	return json.NewEncoder(w).Encode(struct {
		Schema int `json:"schema"`
		Stats
	}{Schema: 1, Stats: st})
}

// StatsCSV writes the metric,value table shared by `rivu stats --csv`
// and the Stats screen export (P4.20).
func StatsCSV(w io.Writer, st Stats) error {
	cw := csv.NewWriter(w)
	rows := [][]string{
		{"metric", "value"},
		{"range", st.Range},
		{"projects", strconv.Itoa(st.Total)},
		{"avg_health", strconv.Itoa(st.AvgHealth)},
		{"median_health", strconv.Itoa(st.MedianHealth)},
		{"stale", strconv.Itoa(st.Stale)},
		{"missing_folder", strconv.Itoa(st.MissingFolder)},
		{"missing_map", strconv.Itoa(st.MissingMap)},
		{"missing_readme", strconv.Itoa(st.MissingReadme)},
	}
	for _, c := range st.ByFlow {
		rows = append(rows, []string{"flow_" + c.Name, strconv.Itoa(c.Count)})
	}
	for _, c := range st.ByLanguage {
		rows = append(rows, []string{"language_" + c.Name, strconv.Itoa(c.Count)})
	}
	for _, c := range st.ByHealth {
		rows = append(rows, []string{"health_" + strings.ReplaceAll(c.Name, "-", "_"), strconv.Itoa(c.Count)})
	}
	cw.WriteAll(rows)
	return cw.Error()
}

// ActivityDaily exposes the registry's per-day event counts for the
// Stats screen sparkline (P4.19); it is deliberately not part of the
// frozen stats --json schema.
func (a *App) ActivityDaily(days int) ([]int, error) {
	return a.Registry.ActivityDaily(days)
}
