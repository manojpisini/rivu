package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// requireYes enforces the Plan → confirm → Apply gate (spec 1.4.3): a
// mutating command needs --yes; --dry-run previews instead (exit 4).
// Shared by flow, delta and the confluence rename/rm gates.
func requireYes(dry, yes bool) error {
	if !dry && !yes {
		return fmt.Errorf("this action requires --yes (or use --dry-run): %w", ErrNeedsConfirm)
	}
	return nil
}

func flowCmd() *cobra.Command {
	var to string
	var dry, yes, flatten bool
	c := &cobra.Command{
		Use:     "flow [project...]",
		Aliases: []string{"move"},
		Args:    cobra.MatchAll(requireFlag("to")),
		Short:   "Move one or more projects to another Flow stage",
		Long: `Move projects to another Flow stage.

Pass several projects: rivu flow a b c --to active --yes, or pipe a
newline-separated list on stdin (blank lines and # comments ignored).
With no projects, Current moves. --flatten drops intermediate folders
(Channel/Domain/proj -> Channel/proj). Failures never block the other
projects: every failure is reported and the exit code reflects it.`,
		RunE: withApp(func(a *service.App, args []string) error {
			if err := requireYes(dry, yes); err != nil {
				return err
			}
			queries := args
			if len(queries) == 0 && stdinHasInput() {
				queries = queryLines(os.Stdin)
			}
			if len(queries) == 0 {
				queries = []string{""} // Current
			}
			res, err := a.FlowBulk(queries, to, flatten, dry)
			if err != nil {
				return err
			}
			for _, fr := range res.Done {
				if dry {
					printFlowPlan(os.Stdout, fr)
				} else {
					fmt.Println(fr.Note)
				}
			}
			return bulkFail(res.Failed)
		}),
	}
	c.ValidArgsFunction = completeProjects
	c.Flags().StringVar(&to, "to", "", "Target: source|active|maintenance|research|delta")
	_ = c.MarkFlagRequired("to")
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview move")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm move")
	c.Flags().BoolVar(&flatten, "flatten", false, "Drop intermediate folders: Channel/Domain/proj -> Channel/proj")
	return c
}

// stdinHasInput reports whether stdin is a pipe or redirected file
// (interactive terminals are character devices and are left alone).
func stdinHasInput() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

// queryLines reads newline-separated project queries, skipping blanks
// and # comments.
func queryLines(r io.Reader) []string {
	var out []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}
