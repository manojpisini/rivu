package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func deltaCmd() *cobra.Command {
	var dry, yes bool
	c := &cobra.Command{Use: "delta [project]", Aliases: []string{"archive"}, Args: cobra.MaximumNArgs(1), Short: "Move a project to the Delta stage", RunE: withApp(func(a *service.App, args []string) error {
		q := ""
		if len(args) > 0 {
			q = args[0]
		}
		if err := requireYes(dry, yes); err != nil {
			return err
		}
		fr, err := a.Flow(q, "delta", false, dry)
		if err != nil {
			return err
		}
		printDelta(os.Stdout, dry, fr)
		return nil
	})}
	c.ValidArgsFunction = completeProjects
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview the move without touching anything")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the move")
	return c
}

// printDelta renders delta's own copy: the shared Plan preview for
// dry-run, the note for no-ops, delta wording for a real move.
func printDelta(out io.Writer, dry bool, fr service.FlowResult) {
	if dry {
		printFlowPlan(out, fr)
		return
	}
	if strings.Contains(fr.Note, "already") {
		fmt.Fprintln(out, fr.Note)
		return
	}
	fmt.Fprintf(out, "Deltaed %s — now at %s (project files untouched)\n", fr.Project.Name, fr.Project.Path)
}
