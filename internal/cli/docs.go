package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func docsCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{
		Use:    "docs",
		Hidden: true,
		Args:   cobra.NoArgs,
		Short:  "Generate man pages and markdown help",
		Long: `Write a man page (*.1) and a markdown file for every command
into --dir (default: docs/). Hidden from help, for packagers and
the docs pipeline.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("create %s: %w", dir, err)
			}
			root := cmd.Root()
			if err := doc.GenManTree(root, nil, dir); err != nil {
				return fmt.Errorf("generate man pages: %w", err)
			}
			if err := doc.GenMarkdownTree(root, dir); err != nil {
				return fmt.Errorf("generate markdown: %w", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			var man, md int
			for _, e := range entries {
				switch {
				case strings.HasSuffix(e.Name(), ".1"):
					man++
				case strings.HasSuffix(e.Name(), ".md"):
					md++
				}
			}
			fmt.Fprintf(os.Stderr, "wrote %d man page(s) and %d markdown file(s) to %s\n", man, md, filepath.Clean(dir))
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", "docs", "output directory")
	return c
}
