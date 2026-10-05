package cli

import (
	"fmt"
	"os"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func sourceCmd() *cobra.Command {
	var flow, domain, typ, language, template, description, editor string
	var git, dry, adopt, noGit, bridge, open bool
	var confluence []string
	c := &cobra.Command{
		Use:     "source <name>",
		Aliases: []string{"new"},
		Args:    cobra.MatchAll(cobra.ExactArgs(1), enumFlag("template", "empty", "go-cli", "node-ts")),
		Short:   "Source a structured project",
		Long: `Source a structured project in the workspace.

Classification flags are stored in .metadata/project.toml: --type,
--description, --domain (Channel/Domain/slug folders), --template
(empty|go-cli|node-ts) and --confluence (repeatable). --language is
kept on the registry row. --bridge records that the bridge tool owns
git init, so rivu never runs it (spec 1.7). --open launches the
editor afterwards, honouring --editor.`,
		RunE: withApp(func(a *service.App, args []string) error {
			sr, e := a.Source(args[0], service.SourceOpts{
				Flow:        flow,
				Git:         git && !noGit,
				Adopt:       adopt,
				Dry:         dry,
				Domain:      domain,
				Type:        typ,
				Language:    language,
				Template:    template,
				Description: description,
				Confluence:  confluence,
				Bridge:      bridge,
			})
			if e != nil {
				return e
			}
			if dry {
				fmt.Printf("DRY RUN: create %s at %s\n", sr.Project.Name, sr.Project.Path)
			} else {
				fmt.Printf("Sourced %s [%s] at %s\n", sr.Project.Name, sr.Project.FlowStage, sr.Project.Path)
			}
			for _, w := range sr.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
			if open && !dry {
				return a.OpenProject(sr.Project.Slug, editor)
			}
			return nil
		}),
	}
	c.Flags().StringVar(&flow, "flow", "source", "Initial flow stage")
	c.Flags().BoolVar(&git, "git", true, "Initialize git")
	c.Flags().BoolVar(&noGit, "no-git", false, "Skip git init (same as --git=false)")
	c.Flags().BoolVar(&adopt, "adopt", false, "Register an existing directory as-is (Bank only, files untouched)")
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview without writing")
	c.Flags().StringVar(&domain, "domain", "", "Folder level under the channel: Channel/Domain/slug")
	c.Flags().StringVar(&typ, "type", "", "Project classification (app, library, service, ...)")
	c.Flags().StringVar(&language, "language", "", "Language to record on the registry row")
	c.Flags().StringVar(&template, "template", "", "Starter template: empty, go-cli or node-ts")
	c.Flags().StringVar(&description, "description", "", "One-line purpose stored in project.toml")
	c.Flags().StringSliceVar(&confluence, "confluence", nil, "Attach to this confluence (repeatable)")
	c.Flags().BoolVar(&bridge, "bridge", false, "Bridge tool owns git init — rivu skips it")
	c.Flags().BoolVar(&open, "open", false, "Open the project when done")
	c.Flags().StringVar(&editor, "editor", "", "Editor for --open (default: configured editor)")
	return c
}
