package cli

import (
	"fmt"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var root string
	var force bool
	c := &cobra.Command{Use: "init", Args: requireFlag("root"), Short: "Create config and database for a workspace", RunE: func(cmd *cobra.Command, _ []string) error {
		res, err := service.Init(root, force)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Initialized Rivu: config %s, database %s, workspace %s\n", res.ConfigPath, res.DBPath, res.Root)
		fmt.Fprintln(cmd.OutOrStdout(), "Next: rivu scan")
		return nil
	}}
	c.Flags().StringVar(&root, "root", "", "workspace root directory to manage")
	_ = c.MarkFlagRequired("root")
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing config (keeps other settings)")
	return c
}
