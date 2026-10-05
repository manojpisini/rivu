package cli

import (
	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func openCmd() *cobra.Command {
	return &cobra.Command{Use: "open [project]", Args: cobra.MaximumNArgs(1), Short: "Open Current or named project", RunE: func(_ *cobra.Command, args []string) error {
		a, e := service.Open()
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
