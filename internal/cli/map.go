package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func mapCmd() *cobra.Command {
	c := &cobra.Command{Use: "agent", Short: "Agent Map operations"}
	c.AddCommand(&cobra.Command{Use: "sync [project]", Args: cobra.MaximumNArgs(1), RunE: withApp(func(a *service.App, args []string) error {
		q := ""
		if len(args) > 0 {
			q = args[0]
		}
		if e := a.Map(q); e != nil {
			return e
		}
		fmt.Println("Map built successfully")
		return nil
	})})
	return c
}
