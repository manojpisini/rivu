package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// confluenceJSON is the scripting contract for
// `rivu confluence list --json` (documented in docs/cli.md).
type confluenceJSON struct {
	Schema      int               `json:"schema"`
	Confluences []confluenceEntry `json:"confluences"`
}

// confluenceShowJSON is the contract for `rivu confluence show --json`.
type confluenceShowJSON struct {
	Schema     int             `json:"schema"`
	Confluence confluenceEntry `json:"confluence"`
	Projects   []projectJSON   `json:"projects"`
}

type confluenceEntry struct {
	Name    string `json:"name"`
	Notes   string `json:"notes"`
	Members int    `json:"members"`
}

func confluenceCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "confluence",
		Short: "Manage and browse Confluences",
		Long: `Manage Confluences: named, cross-cutting groupings of projects.

A Confluence is a tag, not a folder (spec 1.2.6): member projects keep
their Channels and Flow stages, and a project can sit in several
Confluences. rm only unlinks the grouping — no project is ever deleted.

Subcommands: list, new, rename, rm, add, remove, show. With no
subcommand this prints the list.`,
		RunE: withApp(func(a *service.App, _ []string) error {
			return confluenceList(a, asJSON)
		}),
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON with schema 1")
	c.AddCommand(
		confluenceListCmd(), confluenceNewCmd(), confluenceRenameCmd(),
		confluenceRmCmd(), confluenceAddCmd(), confluenceRemoveCmd(),
		confluenceShowCmd(),
	)
	return c
}

func confluenceListCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Short:   "List Confluences with member counts",
		Long: `List every Confluence with its member count and notes.
--json prints {"schema":1,"confluences":[{name,notes,members}]} with an
empty array when there are none.`,
		RunE: withApp(func(a *service.App, _ []string) error {
			return confluenceList(a, asJSON)
		}),
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON with schema 1")
	return c
}

func confluenceList(a *service.App, asJSON bool) error {
	cs, err := a.Confluences()
	if err != nil {
		return err
	}
	if asJSON {
		out := confluenceJSON{Schema: 1, Confluences: make([]confluenceEntry, 0, len(cs))}
		for _, c := range cs {
			out.Confluences = append(out.Confluences, confluenceEntry{Name: c.Name, Notes: c.Notes, Members: c.Members})
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	printConfluenceList(os.Stdout, cs)
	return nil
}

func printConfluenceList(w io.Writer, cs []registry.Confluence) {
	if len(cs) == 0 {
		return
	}
	fmt.Fprintf(w, "%-24s %-8s %s\n", "NAME", "MEMBERS", "NOTES")
	for _, c := range cs {
		fmt.Fprintf(w, "%-24s %-8d %s\n", c.Name, c.Members, c.Notes)
	}
}

func confluenceNewCmd() *cobra.Command {
	var notes string
	c := &cobra.Command{
		Use:   "new NAME",
		Args:  cobra.ExactArgs(1),
		Short: "Create a Confluence",
		Long: `Create a Confluence. The name is normalised to a slug ("Heap & Stack"
becomes heap-stack); use that form in every other subcommand. --notes
stores a short description shown by list and show.`,
		RunE: withApp(func(a *service.App, args []string) error {
			nc, err := a.ConfluenceNew(args[0], notes)
			if err != nil {
				return err
			}
			fmt.Printf("created confluence %s\n", nc.Name)
			return nil
		}),
	}
	c.Flags().StringVar(&notes, "notes", "", "short description shown by list/show")
	return c
}

func confluenceRenameCmd() *cobra.Command {
	var dry, yes bool
	c := &cobra.Command{
		Use:   "rename CONFLUENCE NEW-NAME",
		Args:  cobra.ExactArgs(2),
		Short: "Rename a Confluence (members stay linked)",
		Long: `Rename a Confluence. The new name is slug-normalised and must not be
taken; members stay linked. Requires --yes; --dry-run previews the
rename and writes nothing (spec 1.4.3).`,
		RunE: withApp(func(a *service.App, args []string) error {
			if err := requireYes(dry, yes); err != nil {
				return err
			}
			c, members, err := a.ConfluenceShow(args[0])
			if err != nil {
				return confluenceErr(err)
			}
			if dry {
				fmt.Printf("DRY RUN rename %s -> %s (%d member(s) stay linked)\n", c.Name, args[1], len(members))
				return nil
			}
			newName, err := a.ConfluenceRename(c.Name, args[1])
			if err != nil {
				return confluenceErr(err)
			}
			fmt.Printf("renamed %s -> %s (%d member(s) stay linked)\n", c.Name, newName, len(members))
			return nil
		}),
	}
	c.ValidArgsFunction = completeConfluenceNames
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview the rename without writing")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the rename")
	return c
}

func confluenceRmCmd() *cobra.Command {
	var dry, yes bool
	c := &cobra.Command{
		Use:     "rm CONFLUENCE",
		Aliases: []string{"delete"},
		Args:    cobra.ExactArgs(1),
		Short:   "Remove a Confluence (unlink only)",
		Long: `Remove a Confluence: the grouping and its membership links go away,
member projects are never touched or deleted (safety rule 1). Requires
--yes; --dry-run previews the unlink and writes nothing.`,
		RunE: withApp(func(a *service.App, args []string) error {
			if err := requireYes(dry, yes); err != nil {
				return err
			}
			c, members, err := a.ConfluenceShow(args[0])
			if err != nil {
				return confluenceErr(err)
			}
			if dry {
				fmt.Printf("DRY RUN rm %s: unlink %d membership(s); projects untouched\n", c.Name, len(members))
				return nil
			}
			if err := confluenceErr(a.ConfluenceDelete(c.Name)); err != nil {
				return err
			}
			fmt.Printf("removed confluence %s (%d membership(s) unlinked; projects untouched)\n", c.Name, len(members))
			return nil
		}),
	}
	c.ValidArgsFunction = completeConfluenceNames
	c.Flags().BoolVar(&dry, "dry-run", false, "Preview the unlink without writing")
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the removal")
	return c
}

func confluenceAddCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "add CONFLUENCE PROJECT...",
		Args:  cobra.MinimumNArgs(2),
		Short: "Add projects to a Confluence",
		Long: `Link one or more projects to an existing Confluence (create the
Confluence first with confluence new). Projects are linked in order and
the command stops at the first project that does not exist.`,
		RunE: withApp(func(a *service.App, args []string) error {
			for _, q := range args[1:] {
				if err := confluenceErr(a.ConfluenceAdd(q, args[0])); err != nil {
					return err
				}
				fmt.Printf("added %s to %s\n", q, args[0])
			}
			return nil
		}),
	}
	c.ValidArgsFunction = completeConfluenceThenProjects
	return c
}

func confluenceRemoveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:     "remove CONFLUENCE PROJECT...",
		Aliases: []string{"unlink"},
		Args:    cobra.MinimumNArgs(2),
		Short:   "Remove projects from a Confluence",
		Long: `Unlink one or more projects from a Confluence. Only the membership
goes away — the Confluence and the projects both stay. A project that
was not a member is reported and not an error.`,
		RunE: withApp(func(a *service.App, args []string) error {
			for _, q := range args[1:] {
				linked, err := a.ConfluenceRemove(q, args[0])
				if err != nil {
					return confluenceErr(err)
				}
				if linked {
					fmt.Printf("removed %s from %s\n", q, args[0])
				} else {
					fmt.Printf("%s was not in %s\n", q, args[0])
				}
			}
			return nil
		}),
	}
	c.ValidArgsFunction = completeConfluenceThenProjects
	return c
}

func confluenceShowCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "show CONFLUENCE",
		Args:  cobra.ExactArgs(1),
		Short: "Show a Confluence and its member projects",
		Long: `Show one Confluence with its notes and member projects in Flow
lifecycle order.
--json prints {"schema":1,"confluence":{name,notes,members},
"projects":[…]} with "projects":[] when there are no members.`,
		RunE: withApp(func(a *service.App, args []string) error {
			c, members, err := a.ConfluenceShow(args[0])
			if err != nil {
				return confluenceErr(err)
			}
			if asJSON {
				out := confluenceShowJSON{Schema: 1, Confluence: confluenceEntry{Name: c.Name, Notes: c.Notes, Members: len(members)},
					Projects: make([]projectJSON, 0, len(members))}
				for _, p := range members {
					out.Projects = append(out.Projects, projectToJSON(p))
				}
				return json.NewEncoder(os.Stdout).Encode(out)
			}
			printConfluenceShow(os.Stdout, c, members)
			return nil
		}),
	}
	c.ValidArgsFunction = completeConfluenceNames
	c.Flags().BoolVar(&asJSON, "json", false, "Print JSON with schema 1")
	return c
}

func printConfluenceShow(w io.Writer, c registry.Confluence, members []registry.Project) {
	fmt.Fprintf(w, "%-24s %-8d %s\n", c.Name, len(members), c.Notes)
	if len(members) == 0 {
		fmt.Fprintf(w, "\nno members yet — add one with: rivu confluence add %s PROJECT\n", c.Name)
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%-24s %-12s %s\n", "SLUG", "FLOW", "LANG")
	for _, p := range members {
		fmt.Fprintf(w, "%-24s %-12s %s\n", p.Slug, p.FlowStage, p.Language)
	}
}

// confluenceErr adds the "what next" hint to lookup failures without
// breaking errors.Is, so exit 3 still applies (AGENTS 5).
func confluenceErr(err error) error {
	if errors.Is(err, registry.ErrConfluenceNotFound) {
		return fmt.Errorf("%w (rivu confluence list shows what exists)", err)
	}
	return err
}
