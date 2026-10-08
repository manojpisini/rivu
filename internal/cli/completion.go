package cli

import (
	"strings"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

// completeConfluenceNames offers confluence names for the first
// positional of a confluence subcommand; later positions are free text,
// so nothing is offered there (P2.24 precedent, same silent-on-failure
// rule as completeProjects).
func completeConfluenceNames(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	a, err := service.Open()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defer a.Close()
	cs, err := a.Confluences()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, c := range cs {
		if strings.HasPrefix(c.Name, toComplete) {
			out = append(out, c.Name+"\t"+c.Notes)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeConfluenceThenProjects completes a confluence first and
// project slugs after it (add/remove take CONFLUENCE PROJECT...).
func completeConfluenceThenProjects(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completeConfluenceNames(cmd, args, toComplete)
	}
	return completeProjects(cmd, args, toComplete)
}

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
