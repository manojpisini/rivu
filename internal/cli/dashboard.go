package cli

import (
	"fmt"
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func dashboardCmd() *cobra.Command {
	return &cobra.Command{Use: "dashboard", Short: "Print a dashboard snapshot", RunE: withApp(func(a *service.App, _ []string) error {
		ps, e := a.List(service.Filter{})
		if e != nil {
			return e
		}
		fmt.Println("RIVU DASHBOARD")
		fmt.Println(strings.Repeat("─", 64))
		for _, p := range ps {
			fmt.Printf("%-22s [%-11s] H:%3d  %s\n", p.Name, p.FlowStage, p.HealthScore, p.Path)
		}
		return nil
	})}
}
