package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func statsCmd() *cobra.Command {
	var asJSON, asCSV bool
	var rng string
	c := &cobra.Command{
		Use:   "stats",
		Args:  statsArgs(&asJSON, &asCSV),
		Short: "Show portfolio metrics",
		Long: `Portfolio metrics: totals, average and median health, stale
(45d+ by default) and missing counts, plus by-flow, by-language and
by-health breakdowns.

--range limits to projects active within the window (all, 7d, 30d,
90d, 365d). --json prints {"schema":1,...}; --csv prints a
metric,value table. The same numbers feed the Stats screen (spec 3.6).`,
		RunE: withApp(func(a *service.App, _ []string) error {
			days := 0
			if rng != "all" {
				days, _ = strconv.Atoi(strings.TrimSuffix(rng, "d"))
			}
			st, err := a.Stats(days)
			if err != nil {
				return err
			}
			switch {
			case asJSON:
				if st.ByLanguage == nil {
					st.ByLanguage = []service.Count{} // scripts get [], never null
				}
				return json.NewEncoder(os.Stdout).Encode(struct {
					Schema int `json:"schema"`
					service.Stats
				}{Schema: 1, Stats: st})
			case asCSV:
				return printStatsCSV(os.Stdout, st)
			default:
				printStatsText(st, a.Config.Flow.StaleThresholdDays)
				return nil
			}
		}),
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON with schema 1")
	c.Flags().BoolVar(&asCSV, "csv", false, "Print a metric,value CSV table")
	c.Flags().StringVar(&rng, "range", "all", "Window: all|7d|30d|90d|365d")
	return c
}

// statsArgs validates format conflicts and the --range window in the
// args stage (exit 2).
func statsArgs(asJSON, asCSV *bool) cobra.PositionalArgs {
	return cobra.MatchAll(
		cobra.MaximumNArgs(0),
		enumFlag("range", "all", "7d", "30d", "90d", "365d"),
		func(_ *cobra.Command, _ []string) error {
			if *asJSON && *asCSV {
				return fmt.Errorf("--json and --csv are mutually exclusive")
			}
			return nil
		},
	)
}

func printStatsText(st service.Stats, staleDays int) {
	fmt.Printf("Projects: %d · Average health: %d · Median health: %d\n", st.Total, st.AvgHealth, st.MedianHealth)
	fmt.Printf("Stale (%dd+): %d · Missing folder: %d · Missing map: %d · Missing README: %d\n",
		staleDays, st.Stale, st.MissingFolder, st.MissingMap, st.MissingReadme)
	for _, c := range st.ByFlow {
		fmt.Printf("%-12s %d\n", c.Name, c.Count)
	}
	fmt.Println("By language:")
	for _, c := range st.ByLanguage {
		fmt.Printf("  %-14s %d\n", c.Name, c.Count)
	}
	fmt.Println("By health:")
	for _, c := range st.ByHealth {
		fmt.Printf("  %-8s %d\n", c.Name, c.Count)
	}
	if st.Range != "all" {
		fmt.Printf("Range: %s\n", st.Range)
	}
}

func printStatsCSV(w *os.File, st service.Stats) error {
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
