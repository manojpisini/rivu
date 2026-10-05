package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func statsCmd() *cobra.Command {
	return &cobra.Command{Use: "stats", Short: "Show portfolio metrics", RunE: withApp(func(a *service.App, _ []string) error {
		ps, e := a.List()
		if e != nil {
			return e
		}
		counts := map[string]int{}
		sum := 0
		for _, p := range ps {
			counts[p.FlowStage]++
			sum += p.HealthScore
		}
		avg := 0
		if len(ps) > 0 {
			avg = sum / len(ps)
		}
		fmt.Printf("Projects: %d · Average health: %d\n", len(ps), avg)
		for _, f := range []string{"source", "active", "maintenance", "research", "delta"} {
			fmt.Printf("%-12s %d\n", f, counts[f])
		}
		return nil
	})}
}
