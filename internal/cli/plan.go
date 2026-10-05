package cli

import (
	"fmt"
	"io"

	"github.com/manojpisini/rivu/internal/service"
)

// printPlanSection prints a labelled plan section and skips it when
// empty, so a dry-run always shows the full Plan (spec 4.4 rule 2).
func printPlanSection(w io.Writer, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s:\n", label)
	for _, it := range items {
		fmt.Fprintf(w, "    %s\n", it)
	}
}

// printSourcePlan renders a dry-run Source plan: destination plus the
// create/write/run/Bank/registry sections it will perform.
func printSourcePlan(w io.Writer, p service.SourcePlan) {
	fmt.Fprintf(w, "DRY RUN: create %s at %s\n", p.Name, p.Path)
	printPlanSection(w, "create", p.Create)
	printPlanSection(w, "write", p.Write)
	printPlanSection(w, "run", p.Run)
	printPlanSection(w, "bank", p.Bank)
	printPlanSection(w, "registry", p.Registry)
}

// printFlowPlan renders a dry-run Flow plan: the move heading plus the
// move/Bank/registry sections it will perform.
func printFlowPlan(w io.Writer, fr service.FlowResult) {
	p := fr.Plan
	fmt.Fprintf(w, "DRY RUN: move %s from %s to %s\n", fr.Project.Name, p.FromStage, p.ToStage)
	printPlanSection(w, "move", p.Move)
	printPlanSection(w, "bank", p.Bank)
	printPlanSection(w, "registry", p.Registry)
}
