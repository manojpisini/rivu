package main

import (
	"fmt"
	"github.com/manojpisini/rivu/internal/app"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/tui"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

var version = "1.0.1"
var commit = "dev"
var date = "unknown"

func main() {
	if err := root().Execute(); err != nil {
		os.Exit(1)
	}
}
func withApp(fn func(*app.App) error) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		a, e := app.Open()
		if e != nil {
			return e
		}
		defer a.Close()
		return fn(a)
	}
}
func root() *cobra.Command {
	r := &cobra.Command{Use: "rivu", Short: "Your project filesystem, mapped and flowing", Version: fmt.Sprintf("%s (%s, %s)", version, commit, date), SilenceUsage: true}
	r.RunE = withApp(func(a *app.App) error {
		ps, e := a.Registry.List()
		if e != nil {
			return e
		}
		return tui.Run(ps, a.Config.Workspace.Root)
	})
	r.AddCommand(tuiCmd(), scanCmd(), sourceCmd(), openCmd(), flowCmd(), doctorCmd(), mapCmd(), listCmd(), statsCmd(), configCmd(), dashboardCmd())
	return r
}
func tuiCmd() *cobra.Command {
	return &cobra.Command{Use: "tui", Short: "Launch the terminal interface", RunE: withApp(func(a *app.App) error {
		if a.Config.Workspace.AutoRescan {
			_, _ = a.Scan()
		}
		ps, e := a.Registry.List()
		if e != nil {
			return e
		}
		return tui.Run(ps, a.Config.Workspace.Root)
	})}
}
func scanCmd() *cobra.Command {
	return &cobra.Command{Use: "scan", Short: "Scan workspace and reconcile registry", RunE: withApp(func(a *app.App) error {
		ps, e := a.Scan()
		if e != nil {
			return e
		}
		fmt.Printf("Mapped %d project(s) from %s\n", len(ps), a.Config.Workspace.Root)
		return nil
	})}
}
func sourceCmd() *cobra.Command {
	var flow string
	var git, dry bool
	c := &cobra.Command{Use: "source <name>", Aliases: []string{"new"}, Args: cobra.ExactArgs(1), Short: "Source a structured project", RunE: withApp(func(a *app.App) error {
		p, e := a.Source(argsName, flow, git, dry)
		if e != nil {
			return e
		}
		if dry {
			fmt.Printf("DRY RUN: create %s at %s\n", p.Name, p.Path)
		} else {
			fmt.Printf("Sourced %s [%s] at %s\n", p.Name, p.FlowStage, p.Path)
		}
		return nil
	})}
	c.PreRun = func(_ *cobra.Command, args []string) { argsName = args[0] }
	c.Flags().StringVar(&flow, "flow", "source", "Initial flow stage")
	c.Flags().BoolVar(&git, "git", true, "Initialize git")
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview without writing")
	return c
}

var argsName string

func openCmd() *cobra.Command {
	return &cobra.Command{Use: "open [project]", Args: cobra.MaximumNArgs(1), Short: "Open Current or named project", RunE: func(_ *cobra.Command, args []string) error {
		a, e := app.Open()
		if e != nil {
			return e
		}
		defer a.Close()
		q := ""
		if len(args) > 0 {
			q = args[0]
		}
		return a.OpenProject(q)
	}}
}
func flowCmd() *cobra.Command {
	var to string
	var dry, yes bool
	c := &cobra.Command{Use: "flow [project]", Aliases: []string{"move"}, Args: cobra.MaximumNArgs(1), Short: "Move a project to another Flow stage", RunE: withApp(func(a *app.App) error {
		q := ""
		if len(flowArgs) > 0 {
			q = flowArgs[0]
		}
		if !dry && !yes {
			return fmt.Errorf("flow changes require --yes (or use --dry-run)")
		}
		p, d, e := a.Flow(q, to, dry)
		if e != nil {
			return e
		}
		fmt.Printf("%s: %s -> %s\n", map[bool]string{true: "DRY RUN", false: "Flowed"}[dry], p.Name, d)
		return nil
	})}
	c.PreRun = func(_ *cobra.Command, args []string) { flowArgs = args }
	c.Flags().StringVar(&to, "to", "", "Target: source|active|maintenance|research|delta")
	_ = c.MarkFlagRequired("to")
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview move")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm move")
	return c
}

var flowArgs []string

func doctorCmd() *cobra.Command {
	c := &cobra.Command{Use: "doctor [project]", Args: cobra.MaximumNArgs(1), Short: "Run project health checks", RunE: withApp(func(a *app.App) error {
		q := ""
		if len(docArgs) > 0 {
			q = docArgs[0]
		}
		rs, e := a.Doctor(q)
		if e != nil {
			return e
		}
		for _, r := range rs {
			fmt.Printf("\n%s — Health %d/100\n", r.Project.Name, r.Score)
			for _, c := range r.Checks {
				mark := "✓"
				if !c.OK {
					mark = "!"
				}
				fmt.Printf("  %s %-14s %s\n", mark, c.Name, c.Detail)
			}
		}
		return nil
	})}
	c.PreRun = func(_ *cobra.Command, args []string) { docArgs = args }
	return c
}

var docArgs []string

func mapCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Agent Map operations"}
	c.AddCommand(&cobra.Command{Use: "sync [project]", Args: cobra.MaximumNArgs(1), RunE: withApp(func(a *app.App) error {
		q := ""
		if len(mapArgs) > 0 {
			q = mapArgs[0]
		}
		if e := a.Map(q); e != nil {
			return e
		}
		fmt.Println("Map built successfully")
		return nil
	})})
	c.Commands()[0].PreRun = func(_ *cobra.Command, args []string) { mapArgs = args }
	return c
}

var mapArgs []string

func listCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"index"}, Short: "List registered projects", RunE: withApp(func(a *app.App) error {
		ps, e := a.Registry.List()
		if e != nil {
			return e
		}
		for _, p := range ps {
			fmt.Printf("%-24s %-12s %-12s %s\n", p.Slug, p.FlowStage, p.Language, p.Path)
		}
		return nil
	})}
}
func statsCmd() *cobra.Command {
	return &cobra.Command{Use: "stats", Short: "Show portfolio metrics", RunE: withApp(func(a *app.App) error {
		ps, e := a.Registry.List()
		if e != nil {
			return e
		}
		counts := map[string]int{}
		sum := 0
		for _, p := range ps {
			counts[p.FlowStage]++
			sum += p.HealthScore
		}
		avg := 0
		if len(ps) > 0 {
			avg = sum / len(ps)
		}
		fmt.Printf("Projects: %d · Average health: %d\n", len(ps), avg)
		for _, f := range []string{"source", "active", "maintenance", "research", "delta"} {
			fmt.Printf("%-12s %d\n", f, counts[f])
		}
		return nil
	})}
}
func dashboardCmd() *cobra.Command {
	return &cobra.Command{Use: "dashboard", Short: "Print a dashboard snapshot", RunE: withApp(func(a *app.App) error {
		ps, e := a.Registry.List()
		if e != nil {
			return e
		}
		fmt.Println("RIVU DASHBOARD")
		fmt.Println(strings.Repeat("─", 64))
		for _, p := range ps {
			fmt.Printf("%-22s [%-11s] H:%3d  %s\n", p.Name, p.FlowStage, p.HealthScore, p.Path)
		}
		return nil
	})}
}
func configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Inspect or initialize configuration"}
	c.AddCommand(&cobra.Command{Use: "path", RunE: func(*cobra.Command, []string) error {
		p, e := config.Path()
		if e == nil {
			fmt.Println(p)
		}
		return e
	}}, &cobra.Command{Use: "show", RunE: func(*cobra.Command, []string) error {
		cfg, e := config.Load()
		if e != nil {
			return e
		}
		fmt.Printf("workspace: %s\ndatabase: %s\neditor: %s\n", filepath.Clean(cfg.Workspace.Root), filepath.Clean(cfg.Data.DBPath), cfg.Editors.Default)
		return nil
	}})
	return c
}
