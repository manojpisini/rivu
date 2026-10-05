package cli

import (
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func sourceCmd() *cobra.Command {
	var flow string
	var git, dry, adopt bool
	c := &cobra.Command{Use: "source <name>", Aliases: []string{"new"}, Args: cobra.ExactArgs(1), Short: "Source a structured project", RunE: withApp(func(a *service.App, args []string) error {
		sr, e := a.Source(args[0], flow, git, adopt, dry)
		if e != nil {
			return e
		}
		if dry {
			fmt.Printf("DRY RUN: create %s at %s\n", sr.Project.Name, sr.Project.Path)
		} else {
			fmt.Printf("Sourced %s [%s] at %s\n", sr.Project.Name, sr.Project.FlowStage, sr.Project.Path)
		}
		for _, w := range sr.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}
		return nil
	})}
	c.Flags().StringVar(&flow, "flow", "source", "Initial flow stage")
	c.Flags().BoolVar(&git, "git", true, "Initialize git")
	c.Flags().BoolVar(&adopt, "adopt", false, "Register an existing directory as-is (Bank only, files untouched)")
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview without writing")
	return c
}
