// Package cli builds the cobra command tree: one file per command,
// shared app-opening helper, stdout for data and stderr for messages.
// cmd/rivu/main.go only wires version info and executes the root.
package cli

import (
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// Root assembles the full command tree. version/commit/date come from
// main, where the ldflags targets live.
func Root(version, commit, date string) *cobra.Command {
	return newRoot(version, commit, date, new(bool))
}

// Run executes the command tree with args and returns the process exit
// code (X-03: 0 ok, 1 error, 2 usage, 3 not found, 4 needs --yes,
// 5 warnings).
func Run(version, commit, date string, args []string) int {
	ran := new(bool)
	r := newRoot(version, commit, date, ran)
	r.SetArgs(args)
	return ExitCode(r.Execute(), *ran)
}

func newRoot(version, commit, date string, ran *bool) *cobra.Command {
	r := &cobra.Command{Use: "rivu", Short: "Your project filesystem, mapped and flowing", Version: fmt.Sprintf("%s (%s, %s)", version, commit, date), SilenceUsage: true}
	var home, cfgFile string
	r.PersistentFlags().StringVar(&home, "home", "", "set Rivu home directory (overrides RIVU_HOME)")
	r.PersistentFlags().StringVar(&cfgFile, "config", "", "set config file path (overrides RIVU_CONFIG)")
	r.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		// Marks that validation passed: failures before this point are
		// usage errors (exit 2), after it they are real errors (exit 1+).
		*ran = true
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
	r.RunE = withApp(func(a *service.App, _ []string) error {
		return launchTUI(a)
	})
	r.AddCommand(tuiCmd(), scanCmd(), sourceCmd(), openCmd(), flowCmd(), doctorCmd(), mapCmd(), listCmd(), statsCmd(), configCmd(), dashboardCmd())
	return r
}

// withApp opens the service, prints config warnings to stderr, runs fn
// with the command's positional args, and closes the app. No globals —
// every command reads its args from here (X-01).
func withApp(fn func(*service.App, []string) error) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		a, e := service.Open()
		if e != nil {
			return e
		}
		defer a.Close()
		for _, w := range a.ConfigWarnings {
			fmt.Fprintln(os.Stderr, "warning: "+w)
		}
		if err := fn(a, args); err != nil {
			return err
		}
		if n := len(a.ConfigWarnings); n > 0 {
			return fmt.Errorf("%w: %d config warning(s)", ErrWarnings, n)
		}
		return nil
	}
}
