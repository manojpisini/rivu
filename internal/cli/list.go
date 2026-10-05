package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"index"}, Short: "List registered projects", RunE: withApp(func(a *service.App, _ []string) error {
		ps, e := a.List()
		if e != nil {
			return e
		}
		for _, p := range ps {
			fmt.Printf("%-24s %-12s %-12s %s\n", p.Slug, p.FlowStage, p.Language, p.Path)
		}
		return nil
	})}
}
