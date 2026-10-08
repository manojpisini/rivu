package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// doctorJSON is the scripting contract for rivu doctor --json
// (documented in docs/cli.md).
type doctorJSON struct {
	Schema  int          `json:"schema"`
	Reports []reportJSON `json:"reports"`
}

type reportJSON struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Score int    `json:"score"`
	// Missing appears only when the project folder is gone (D-07):
	// an additive signal, existing consumers keep working.
	Missing bool        `json:"missing,omitempty"`
	Checks  []checkJSON `json:"checks"`
}

type checkJSON struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Weight int    `json:"weight"`
	Detail string `json:"detail"`
}

func doctorCmd() *cobra.Command {
	var all, asJSON, dry, yes bool
	var minScore int
	var fixes []string
	c := &cobra.Command{
		Use:   "doctor [project]",
		Args:  doctorArgs(&all, &minScore, &fixes),
		Short: "Run project health checks",
		Long: `Run health checks for a project, or for every project with no
argument or --all.

--json prints {"schema":1,"reports":[...]}. --min-score N exits 5 when
any report scores below N, so CI can gate on health.

--fix ID (readme, map) applies the safe remedies: creating a missing
README and rebuilding machine-owned Bank/Map files. It shows the plan
first; --dry-run previews and --yes confirms (spec 4.4).`,
		RunE: withApp(func(a *service.App, args []string) error {
			q := ""
			if len(args) > 0 {
				q = args[0]
			}
			rs, e := a.Doctor(q)
			if e != nil {
				return e
			}
			if len(fixes) > 0 {
				plan := service.DoctorFixPlan(rs, fixes)
				if len(plan) == 0 {
					fmt.Fprintln(os.Stderr, "nothing to fix — every requested remedy already holds")
					return nil
				}
				fmt.Println("Safe fixes (create-if-missing only, project folders never touched):")
				for _, l := range plan {
					fmt.Println("  " + l)
				}
				if dry {
					return nil
				}
				if err := requireYes(dry, yes); err != nil {
					return err
				}
				applied, err := a.ApplyDoctorFixes(rs, fixes)
				if applied > 0 {
					warnf("applied %d fix(es)", applied)
				}
				if err != nil {
					return err
				}
				if rs, e = a.Doctor(q); e != nil {
					return e
				}
			}
			if asJSON {
				if err := printDoctorJSON(os.Stdout, rs); err != nil {
					return err
				}
			} else {
				for _, r := range rs {
					header := fmt.Sprintf("%s — Health %d/100", r.Project.Name, r.Score)
					if r.Missing {
						header = r.Project.Name + " — MISSING (folder not found)"
					}
					fmt.Printf("\n%s\n", header)
					for _, chk := range r.Checks {
						if chk.OK {
							fmt.Printf("  ✓ %-14s %s\n", chk.Name, chk.Detail)
							continue
						}
						mark := "!"
						if chk.Severity == "error" {
							mark = "✗"
						}
						fmt.Printf("  %s %-14s %s\n", mark, chk.Name, chk.Finding)
						if chk.Remedy != "" {
							fmt.Printf("      fix: %s\n", strings.ReplaceAll(chk.Remedy, "{slug}", r.Project.Slug))
						}
					}
				}
			}
			if minScore > 0 {
				var low []string
				for _, r := range rs {
					if r.Score < minScore {
						low = append(low, fmt.Sprintf("%s=%d", r.Project.Slug, r.Score))
					}
				}
				if len(low) > 0 {
					return fmt.Errorf("%w: %d project(s) below --min-score %d: %s", ErrWarnings, len(low), minScore, strings.Join(low, ", "))
				}
			}
			return nil
		}),
	}
	c.ValidArgsFunction = completeProjects
	c.Flags().BoolVar(&all, "all", false, "Check every project (same as omitting the argument)")
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON with schema 1")
	c.Flags().IntVar(&minScore, "min-score", 0, "Exit 5 when any score is below this (0 disables)")
	c.Flags().StringSliceVar(&fixes, "fix", nil, "Apply safe fixes: readme, map")
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview the fix plan without applying")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm applying the fix plan")
	return c
}

// doctorArgs validates flag combinations during the args stage, so
// bad values exit 2 (usage) before any checks run.
func doctorArgs(all *bool, minScore *int, fixes *[]string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if *all && len(args) > 0 {
			return fmt.Errorf("give either a project or --all, not both")
		}
		if *minScore < 0 || *minScore > 100 {
			return fmt.Errorf("--min-score must be between 0 and 100")
		}
		for _, id := range *fixes {
			switch id {
			case "readme", "map":
			default:
				return fmt.Errorf("unknown --fix id %q (want readme or map)", id)
			}
		}
		return nil
	}
}

func printDoctorJSON(w *os.File, rs []doctor.Report) error {
	out := doctorJSON{Schema: 1, Reports: make([]reportJSON, 0, len(rs))}
	for _, r := range rs {
		rj := reportJSON{
			Slug: r.Project.Slug, Name: r.Project.Name, Path: r.Project.Path,
			Score: r.Score, Missing: r.Missing,
			Checks: make([]checkJSON, 0, len(r.Checks)),
		}
		for _, c := range r.Checks {
			rj.Checks = append(rj.Checks, checkJSON{Name: c.Name, OK: c.OK, Weight: c.Weight, Detail: c.Detail})
		}
		out.Reports = append(out.Reports, rj)
	}
	return json.NewEncoder(w).Encode(out)
}
