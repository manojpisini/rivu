package cli

import (
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/tui"
	"github.com/spf13/cobra"
)

func tuiCmd() *cobra.Command {
	return &cobra.Command{Use: "tui", Short: "Launch the terminal interface", RunE: withApp(func(a *service.App, _ []string) error {
		if a.Config.Workspace.AutoRescan {
			_, _ = a.Scan()
		}
		return launchTUI(a)
	})}
}

// launchTUI starts the terminal interface over the shared service
// layer; used by the root command and `rivu tui`. Without a TTY (pipes,
// CI) it prints a friendly notice on stderr and falls back to the
// printed Master Dashboard instead of writing escapes into the stream
// (P3.25).
func launchTUI(a *service.App) error {
	if !isTerminal(os.Stdout) {
		fmt.Fprintln(os.Stderr, "rivu: no terminal attached — printing the dashboard instead (rivu dashboard).")
		d, err := a.Dashboard()
		if err != nil {
			return fmt.Errorf("build dashboard: %w", err)
		}
		printDashboard(d, a.Config.Flow.StaleThresholdDays)
		return nil
	}
	return tui.Run(a, a.Config)
}

// isTerminal reports whether f is attached to a character device —
// stdlib check, no new dependency.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
