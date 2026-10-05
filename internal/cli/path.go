package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func pathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path [project]",
		Short: "Print a project path to stdout",
		Long: `Print a project path to stdout and nothing else, so shell wrappers can
use it directly:

  rcd() { cd "$(rivu path "$@")"; }

With no argument the Current project's path is printed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: withApp(func(a *service.App, args []string) error {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			p, err := a.Registry.Resolve(q)
			if err != nil {
				return err
			}
			fmt.Println(p.Path)
			return nil
		}),
	}
}
