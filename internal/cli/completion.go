package cli

import (
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// completeProjects offers registered project slugs for shell tab
// completion on every command that takes a project argument (P2.24).
// It opens the service itself because completion runs outside the
// command's PersistentPreRunE, and stays silent on any failure so tab
// never breaks.
func completeProjects(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	a, err := service.Open()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer a.Close()
	ps, err := a.List(service.Filter{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, p := range ps {
		if strings.HasPrefix(p.Slug, toComplete) {
			out = append(out, p.Slug+"\t"+p.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
