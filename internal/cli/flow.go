package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func flowCmd() *cobra.Command {
	var to string
	var dry, yes, flatten bool
	c := &cobra.Command{Use: "flow [project]", Aliases: []string{"move"}, Args: cobra.MaximumNArgs(1), Short: "Move a project to another Flow stage", RunE: withApp(func(a *service.App, args []string) error {
		q := ""
		if len(args) > 0 {
			q = args[0]
		}
		if !dry && !yes {
			return fmt.Errorf("flow changes require --yes (or use --dry-run)")
		}
		fr, e := a.Flow(q, to, flatten, dry)
		if e != nil {
			return e
		}
		fmt.Println(fr.Note)
		return nil
	})}
	c.Flags().StringVar(&to, "to", "", "Target: source|active|maintenance|research|delta")
	_ = c.MarkFlagRequired("to")
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview move")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm move")
	c.Flags().BoolVar(&flatten, "flatten", false, "Drop intermediate folders: Channel/Domain/proj -> Channel/proj")
	return c
}
