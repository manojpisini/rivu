package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func dbCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "db",
		Short: "Back up and restore the registry as JSON",
		Long: `Back up and restore the registry database as JSON.

rivu db export writes every registry row (projects, confluences,
membership, health history, activity, Current) as JSON — to stdout,
or to a file when you give a path. rivu db import replaces the
registry with such a file after showing what it contains; it needs
--yes to apply (Plan -> confirm -> Apply) and snapshots the current
database to a .bak file first. Import never touches project folders
(safety rule 1).`,
	}
	c.AddCommand(dbExportCmd(), dbImportCmd())
	return c
}

func dbExportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export [file]",
		Short: "Write the registry as a JSON backup",
		Args:  cobra.MaximumNArgs(1),
		RunE: withApp(func(a *service.App, args []string) error {
			d, err := a.Export()
			if err != nil {
				return err
			}
			b, err := json.MarshalIndent(d, "", "  ")
			if err != nil {
				return fmt.Errorf("encode backup: %w", err)
			}
			b = append(b, '\n')
			if len(args) == 0 {
				_, err = os.Stdout.Write(b)
				return err
			}
			if err := os.WriteFile(args[0], b, 0600); err != nil {
				return fmt.Errorf("write backup %s: %w", args[0], err)
			}
			fmt.Fprintf(os.Stderr, "wrote %s (%d projects, %d confluences)\n", args[0], len(d.Projects), len(d.Confluences))
			return nil
		}),
	}
}

func dbImportCmd() *cobra.Command {
	var dry, yes bool
	c := &cobra.Command{
		Use:   "import <file>",
		Short: "Restore the registry from a JSON backup",
		Args:  cobra.ExactArgs(1),
		RunE: withApp(func(a *service.App, args []string) error {
			b, err := os.ReadFile(args[0])
			if err != nil {
				return fmt.Errorf("read backup %s: %w", args[0], err)
			}
			d, err := registry.ParseDump(b)
			if err != nil {
				return err
			}
			if err := requireYes(dry, yes); err != nil {
				return err
			}
			// The Plan (spec 4.4 rule 2): exactly what the file holds.
			fmt.Printf("Restore %s\n", args[0])
			fmt.Printf("  projects %d · confluences %d · membership %d\n", len(d.Projects), len(d.Confluences), len(d.Links))
			fmt.Printf("  snapshots %d · activity %d · settings %d\n", len(d.Snapshots), len(d.Activity), len(d.Settings))
			if dry {
				return nil
			}
			res, err := a.Import(d)
			if err != nil {
				return err
			}
			fmt.Printf("Restored from %s — previous registry backed up to %s\n", args[0], res.Backup)
			return nil
		}),
	}
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview what the file contains")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the restore")
	return c
}
