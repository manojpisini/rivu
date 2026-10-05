package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

var mapArgs []string

func mapCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Agent Map operations"}
	c.AddCommand(&cobra.Command{Use: "sync [project]", Args: cobra.MaximumNArgs(1), RunE: withApp(func(a *service.App) error {
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
