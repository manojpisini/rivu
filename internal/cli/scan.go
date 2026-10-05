package cli

import (
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func scanCmd() *cobra.Command {
	return &cobra.Command{Use: "scan", Short: "Scan workspace and reconcile registry", RunE: withApp(func(a *service.App, _ []string) error {
		res, e := a.Scan()
		if e != nil {
			return e
		}
		for _, w := range res.Warnings {
			fmt.Fprintln(os.Stderr, "warning: "+w)
		}
		st, e := a.Registry.States()
		if e != nil {
			return e
		}
		if n := len(st.Missing) + len(st.Unregistered) + len(st.StageMismatch); n > 0 {
			fmt.Printf("Attention: %d missing, %d unregistered, %d stage-mismatch\n", len(st.Missing), len(st.Unregistered), len(st.StageMismatch))
		}
		fmt.Printf("Mapped %d project(s) from %s\n", len(res.Projects), a.Config.Workspace.Root)
		if n := len(res.Warnings); n > 0 {
			return fmt.Errorf("%w: %d scan warning(s)", ErrWarnings, n)
		}
		return nil
	})}
}
