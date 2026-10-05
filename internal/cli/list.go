package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// listJSON is the scripting contract for rivu list --json (documented
// in docs/cli.md).
type listJSON struct {
	Schema   int           `json:"schema"`
	Projects []projectJSON `json:"projects"`
}

type projectJSON struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Path         string   `json:"path"`
	Channel      string   `json:"channel"`
	FlowStage    string   `json:"flow_stage"`
	Language     string   `json:"language,omitempty"`
	Stack        []string `json:"stack,omitempty"`
	HealthScore  int      `json:"health_score"`
	HasGit       bool     `json:"has_git"`
	HasBank      bool     `json:"has_bank"`
	HasMap       bool     `json:"has_map"`
	CreatedAt    string   `json:"created_at"`
	LastOpenedAt string   `json:"last_opened_at,omitempty"`
}

func listCmd() *cobra.Command {
	var flow, lang, confluence, sortKey string
	var unhealthy, stale, asJSON bool
	c := &cobra.Command{
		Use:     "list",
		Aliases: []string{"index"},
		Short:   "List registered projects",
		Long: `List registered projects with optional filters.

Filters combine with AND: --flow active --lang go keeps active Go projects.
--unhealthy means health below 60; --stale means no activity within
[flow].stale_threshold_days of the config. --confluence keeps members of
that confluence.

--sort orders by name, health (worst first), opened (most recent first)
or created (newest first); the default is Flow lifecycle order.
--json prints {"schema":1,"projects":[...]} for scripts.`,
		Args: cobra.MatchAll(cobra.MaximumNArgs(0),
			enumFlag("flow", service.Stages...),
			enumFlag("sort", "name", "health", "opened", "created")),
		RunE: withApp(func(a *service.App, _ []string) error {
			ps, err := a.List(service.Filter{
				Flow:       flow,
				Lang:       lang,
				Unhealthy:  unhealthy,
				Stale:      stale,
				Confluence: confluence,
				Sort:       sortKey,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return printListJSON(os.Stdout, ps)
			}
			printListText(os.Stdout, ps)
			return nil
		}),
	}
	c.Flags().StringVar(&flow, "flow", "", "only this Flow stage (source|active|maintenance|research|delta)")
	c.Flags().StringVar(&lang, "lang", "", "only this language (case-insensitive)")
	c.Flags().BoolVar(&unhealthy, "unhealthy", false, "only projects with health below 60")
	c.Flags().BoolVar(&stale, "stale", false, "only projects past the configured stale threshold")
	c.Flags().StringVar(&confluence, "confluence", "", "only projects in this confluence")
	c.Flags().StringVar(&sortKey, "sort", "", "order by name|health|opened|created (default: Flow order)")
	c.Flags().BoolVar(&asJSON, "json", false, "print JSON with schema 1")
	return c
}

func printListText(w io.Writer, ps []registry.Project) {
	if len(ps) == 0 {
		return
	}
	fmt.Fprintf(w, "%-24s %-12s %-12s %s\n", "SLUG", "FLOW", "LANG", "PATH")
	for _, p := range ps {
		fmt.Fprintf(w, "%-24s %-12s %-12s %s\n", p.Slug, p.FlowStage, p.Language, p.Path)
	}
}

func printListJSON(w io.Writer, ps []registry.Project) error {
	out := listJSON{Schema: 1, Projects: make([]projectJSON, 0, len(ps))}
	for _, p := range ps {
		out.Projects = append(out.Projects, projectToJSON(p))
	}
	return json.NewEncoder(w).Encode(out)
}

// projectToJSON converts a registry project to the shared JSON shape
// used by list --json and scan --json.
func projectToJSON(p registry.Project) projectJSON {
	return projectJSON{
		ID: p.ID, Name: p.Name, Slug: p.Slug, Path: p.Path,
		Channel: p.Channel, FlowStage: p.FlowStage, Language: p.Language,
		Stack: p.Stack, HealthScore: p.HealthScore,
		HasGit: p.HasGit, HasBank: p.HasBank, HasMap: p.HasMap,
		CreatedAt:    rfc3339(p.CreatedAt),
		LastOpenedAt: rfc3339(p.LastOpenedAt),
	}
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
