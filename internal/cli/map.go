package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func mapCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Agent Map operations"}
	c.AddCommand(mapSyncCmd())
	return c
}

func mapSyncCmd() *cobra.Command {
	var all, dry, check bool
	c := &cobra.Command{
		Use:   "sync [project]",
		Args:  exclusiveAll(&all),
		Short: "Build or check the agent Map",
		Long: `Build the Map for a project (Current by default) or every
project with --all.

AGENTS.md is create-if-missing, never clobbered; PROJECT_MAP.md is
regenerated only when its content changed. --dry-run reports what
would change and writes nothing; --check reports and exits 5 when a
sync is needed, so CI can gate on it.`,
		RunE: withApp(func(a *service.App, args []string) error {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			if dry || check {
				return reportMapStatus(a, q, all, check)
			}
			if all {
				res, err := a.MapBulk()
				if err != nil {
					return err
				}
				for _, f := range res.Failed {
					fmt.Fprintf(os.Stderr, "%s: %v\n", f.Query, f.Err)
				}
				fmt.Printf("Map built for %d project(s)\n", len(res.Done))
				return bulkFail(res.Failed)
			}
			if err := a.Map(q); err != nil {
				return err
			}
			fmt.Println("Map built successfully")
			return nil
		}),
	}
	c.Flags().BoolVar(&all, "all", false, "Sync every project")
	c.Flags().BoolVar(&dry, "dry-run", false, "Report what would change, write nothing")
	c.Flags().BoolVar(&check, "check", false, "Exit 5 when any Map needs a sync")
	return c
}

// reportMapStatus prints per-project sync needs; --check turns needs
// into exit 5 (warnings) so CI can fail a stale Map.
func reportMapStatus(a *service.App, q string, all, check bool) error {
	sts, err := a.MapStatus(q, all)
	if err != nil {
		return err
	}
	var need []string
	for _, s := range sts {
		var changes []string
		if s.AgentsMissing {
			changes = append(changes, "create .metadata/agent/AGENTS.md")
		}
		if s.MapStale {
			changes = append(changes, "update .metadata/agent/PROJECT_MAP.md")
		}
		if len(changes) == 0 {
			if !check {
				fmt.Printf("%s: up to date\n", s.Project.Slug)
			}
			continue
		}
		need = append(need, s.Project.Slug)
		fmt.Printf("%s: %s\n", s.Project.Slug, strings.Join(changes, ", "))
	}
	if check && len(need) > 0 {
		return fmt.Errorf("%w: %d project(s) need agent sync: %s", ErrWarnings, len(need), strings.Join(need, ", "))
	}
	return nil
}
