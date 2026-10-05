package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// reconcileJSON is one entry of index --json's "reconciled" list.
type reconcileJSON struct {
	Slug string `json:"slug"`
	From string `json:"from"`
	To   string `json:"to"`
}

// indexJSON is the scripting contract for rivu index --json: the scan
// shape plus what was reconciled (documented in docs/cli.md).
type indexJSON struct {
	scanJSON
	Reconciled []reconcileJSON `json:"reconciled"`
}

func indexCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "index",
		Short: "Rebuild the registry index from disk",
		Long: `Rescan the workspace, then reconcile stage mismatches: a
project whose folder sits in a different channel than its flow stage
says gets the stage (and its project.toml) updated to match the
folder.

Unregistered folders stay for source/adopt and missing folders stay
reported — index never adopts or removes. --json prints
{"schema":1,...} with the scan fields plus "reconciled". A rebuild
with warnings exits 5.`,
		Args: cobra.NoArgs,
		RunE: withApp(func(a *service.App, _ []string) error {
			res, err := a.Index()
			if err != nil {
				return err
			}
			for _, w := range res.Scan.Warnings {
				warnf("%s", w)
			}
			for _, f := range res.Failures {
				fmt.Fprintf(os.Stderr, "%s: %v\n", f.Query, f.Err)
			}
			st, err := a.Registry.States()
			if err != nil {
				return err
			}
			if asJSON {
				out := indexJSON{
					scanJSON: scanJSON{
						Schema:        1,
						Root:          a.Config.Workspace.Root,
						Projects:      []projectJSON{}, // scripts get [], never null
						Warnings:      nonNil(res.Scan.Warnings),
						Missing:       slugsOf(st.Missing),
						Unregistered:  slugsOf(st.Unregistered),
						StageMismatch: slugsOf(st.StageMismatch),
					},
					Reconciled: []reconcileJSON{},
				}
				for _, r := range res.Reconciled {
					out.Reconciled = append(out.Reconciled, reconcileJSON{Slug: r.Slug, From: r.From, To: r.To})
				}
				for _, p := range res.Scan.Projects {
					out.Projects = append(out.Projects, projectToJSON(p))
				}
				if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
					return err
				}
			} else {
				for _, r := range res.Reconciled {
					fmt.Printf("reconciled %s: %s -> %s\n", r.Slug, r.From, r.To)
				}
				if n := len(st.Missing) + len(st.Unregistered) + len(st.StageMismatch); n > 0 {
					fmt.Printf("Attention: %d missing, %d unregistered, %d stage-mismatch\n", len(st.Missing), len(st.Unregistered), len(st.StageMismatch))
				}
				fmt.Printf("Indexed %d project(s) from %s\n", len(res.Scan.Projects), a.Config.Workspace.Root)
			}
			if len(res.Failures) > 0 {
				return bulkFail(res.Failures)
			}
			if n := len(res.Scan.Warnings); n > 0 {
				return fmt.Errorf("%w: %d scan warning(s)", ErrWarnings, n)
			}
			return nil
		}),
	}
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON with schema 1")
	return c
}
