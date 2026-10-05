// Package cli builds the cobra command tree: one file per command,
// shared app-opening helper, stdout for data and stderr for messages.
// cmd/rivu/main.go only wires version info and executes the root.
package cli

import (
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/tui"
	"github.com/spf13/cobra"
)

// Root assembles the full command tree. version/commit/date come from
// main, where the ldflags targets live.
func Root(version, commit, date string) *cobra.Command {
	r := &cobra.Command{Use: "rivu", Short: "Your project filesystem, mapped and flowing", Version: fmt.Sprintf("%s (%s, %s)", version, commit, date), SilenceUsage: true}
	var home, cfgFile string
	r.PersistentFlags().StringVar(&home, "home", "", "set Rivu home directory (overrides RIVU_HOME)")
	r.PersistentFlags().StringVar(&cfgFile, "config", "", "set config file path (overrides RIVU_CONFIG)")
	r.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		if home != "" {
			if err := os.Setenv("RIVU_HOME", home); err != nil {
				return fmt.Errorf("apply --home: %w", err)
			}
		}
		if cfgFile != "" {
			if err := os.Setenv("RIVU_CONFIG", cfgFile); err != nil {
				return fmt.Errorf("apply --config: %w", err)
			}
		}
		return nil
	}
	r.RunE = withApp(func(a *service.App) error {
		ps, e := a.List()
		if e != nil {
			return e
		}
		return tui.Run(ps, a.Config.Workspace.Root)
	})
	r.AddCommand(tuiCmd(), scanCmd(), sourceCmd(), openCmd(), flowCmd(), doctorCmd(), mapCmd(), listCmd(), statsCmd(), configCmd(), dashboardCmd())
	return r
}

// withApp opens the service, prints config warnings to stderr, runs fn
// and closes the app.
func withApp(fn func(*service.App) error) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, _ []string) error {
		a, e := service.Open()
		if e != nil {
			return e
		}
		defer a.Close()
		for _, w := range a.ConfigWarnings {
			fmt.Fprintln(os.Stderr, "warning: "+w)
		}
		return fn(a)
	}
}
