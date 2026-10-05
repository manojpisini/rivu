package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// scanJSON is the scripting contract for rivu scan --json (documented
// in docs/cli.md).
type scanJSON struct {
	Schema        int           `json:"schema"`
	Root          string        `json:"root"`
	Projects      []projectJSON `json:"projects"`
	Warnings      []string      `json:"warnings"`
	Missing       []string      `json:"missing"`
	Unregistered  []string      `json:"unregistered"`
	StageMismatch []string      `json:"stage_mismatch"`
}

func scanCmd() *cobra.Command {
	var root string
	var asJSON bool
	c := &cobra.Command{
		Use:   "scan",
		Short: "Scan workspace and reconcile registry",
		Long: `Scan the workspace, register what it finds, and report mismatches.

Warnings go to stderr and mismatches (missing, unregistered, stage
mismatch) to stdout; a scan with warnings exits 5. --root scans only
that folder for this run without changing the config. --json prints
{"schema":1,...} with projects, warnings and the mismatch lists.`,
		Args: cobra.MaximumNArgs(0),
		RunE: withApp(func(a *service.App, _ []string) error {
			if root != "" {
				a.Config.Workspace.Root = config.Expand(root)
				a.Config.Workspace.SecondaryRoots = nil // explicit root means this root only
			}
			res, e := a.Scan()
			if e != nil {
				return e
			}
			for _, w := range res.Warnings {
				warnf("%s", w)
			}
			st, e := a.Registry.States()
			if e != nil {
				return e
			}
			if asJSON {
				out := scanJSON{
					Schema:        1,
					Root:          a.Config.Workspace.Root,
					Projects:      []projectJSON{}, // scripts get [], never null
					Warnings:      nonNil(res.Warnings),
					Missing:       slugsOf(st.Missing),
					Unregistered:  slugsOf(st.Unregistered),
					StageMismatch: slugsOf(st.StageMismatch),
				}
				for _, p := range res.Projects {
					out.Projects = append(out.Projects, projectToJSON(p))
				}
				if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
					return err
				}
			} else {
				if n := len(st.Missing) + len(st.Unregistered) + len(st.StageMismatch); n > 0 {
					fmt.Printf("Attention: %d missing, %d unregistered, %d stage-mismatch\n", len(st.Missing), len(st.Unregistered), len(st.StageMismatch))
				}
				fmt.Printf("Mapped %d project(s) from %s\n", len(res.Projects), a.Config.Workspace.Root)
			}
			if n := len(res.Warnings); n > 0 {
				return fmt.Errorf("%w: %d scan warning(s)", ErrWarnings, n)
			}
			return nil
		}),
	}
	c.Flags().StringVar(&root, "root", "", "scan only this folder for this run (config unchanged)")
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON with schema 1")
	return c
}

func slugsOf(ps []registry.Project) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Slug)
	}
	return out
}

func nonNil(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}
