package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func doctorCmd() *cobra.Command {
	c := &cobra.Command{Use: "doctor [project]", Args: cobra.MaximumNArgs(1), Short: "Run project health checks", RunE: withApp(func(a *service.App, args []string) error {
		q := ""
		if len(args) > 0 {
			q = args[0]
		}
		rs, e := a.Doctor(q)
		if e != nil {
			return e
		}
		for _, r := range rs {
			fmt.Printf("\n%s — Health %d/100\n", r.Project.Name, r.Score)
			for _, c := range r.Checks {
				mark := "✓"
				if !c.OK {
					mark = "!"
				}
				fmt.Printf("  %s %-14s %s\n", mark, c.Name, c.Detail)
			}
		}
		return nil
	})}
	return c
}
