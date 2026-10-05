package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/manojpisini/rivu/internal/config"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Inspect or initialize configuration"}
	c.AddCommand(&cobra.Command{Use: "path", RunE: func(*cobra.Command, []string) error {
		p, e := config.Path()
		if e == nil {
			fmt.Println(p)
		}
		return e
	}}, &cobra.Command{Use: "show", RunE: func(*cobra.Command, []string) error {
		cfg, warns, e := config.Load()
		if e != nil {
			return e
		}
		for _, w := range warns {
			fmt.Fprintln(os.Stderr, "warning: "+w)
		}
		fmt.Printf("workspace: %s\ndatabase: %s\neditor: %s\n", filepath.Clean(cfg.Workspace.Root), filepath.Clean(cfg.Data.DBPath), cfg.Editors.Default)
		return nil
	}})
	return c
}
