package cli

import (
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

// launchTUI starts the terminal interface over the current project list;
// shared by the root command and `rivu tui`.
func launchTUI(a *service.App) error {
	ps, e := a.List()
	if e != nil {
		return e
	}
	return tui.Run(ps, a.Config.Workspace.Root)
}
