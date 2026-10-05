package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func openCmd() *cobra.Command {
	var editor string
	var dry bool
	c := &cobra.Command{
		Use:   "open [project]",
		Args:  cobra.MaximumNArgs(1),
		Short: "Open Current or named project",
		Long: `Open Current or a named project in an editor.

--editor overrides the configured editor for this run. --dry-run only
prints the command that would run — nothing is launched and Current
does not change.`,
		RunE: withApp(func(a *service.App, args []string) error {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			if dry {
				argv, err := a.OpenCommand(q, editor)
				if err != nil {
					return err
				}
				fmt.Println(joinArgv(argv))
				return nil
			}
			return a.OpenProject(q, editor)
		}),
	}
	c.Flags().StringVar(&editor, "editor", "", "Editor to use for this run (default: configured editor)")
	c.Flags().BoolVar(&dry, "dry-run", false, "Print the command instead of launching it")
	return c
}

// joinArgv renders a command line for copy-paste, quoting arguments
// that contain whitespace.
func joinArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if strings.ContainsAny(a, " \t") {
			parts[i] = strconv.Quote(a)
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}
