package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/manojpisini/rivu/internal/service"
	"github.com/spf13/cobra"
)

func dashboardCmd() *cobra.Command {
	return &cobra.Command{Use: "dashboard", Short: "Print a Master Dashboard snapshot", RunE: withApp(func(a *service.App, _ []string) error {
		d, err := a.Dashboard()
		if err != nil {
			return err
		}
		printDashboard(d, a.Config.Flow.StaleThresholdDays)
		return nil
	})}
}

func printDashboard(d service.Dashboard, staleDays int) {
	const rule = "────────────────────────────────────────────────────────────"
	fmt.Println("RIVU DASHBOARD")
	fmt.Println(rule)
	fmt.Printf("Root: %s   Roots: %d   Last scan: %s   Health: %d/100\n",
		d.Root, d.Roots, ago(d.LastScan), d.Stats.AvgHealth)

	fmt.Println("\nPortfolio")
	fmt.Printf("  %-20s %d\n", "Total projects", d.Stats.Total)
	for _, c := range d.Stats.ByFlow {
		label := c.Name
		if c.Name == "source" {
			label = "source (untriaged)"
		}
		fmt.Printf("  %-20s %d\n", label, c.Count)
	}

	fmt.Println("\nNeeds attention")
	if len(d.Attention) == 0 {
		fmt.Println("  nothing needs attention")
	}
	for _, at := range d.Attention {
		fmt.Printf("  ! %s\n", attentionLabel(at, staleDays))
	}

	fmt.Println("\nBy language")
	if len(d.Stats.ByLanguage) == 0 {
		fmt.Println("  no projects yet")
	}
	for _, c := range d.Stats.ByLanguage {
		fmt.Printf("  %-14s %s %d\n", c.Name, bar(c.Count, d.Stats.Total), c.Count)
	}

	fmt.Println("\nRecent activity")
	if len(d.Recent) == 0 {
		fmt.Println("  no activity recorded yet")
	}
	for _, ev := range d.Recent {
		fmt.Printf("  %-9s %-8s %s\n", ago(ev.OccurredAt), ev.Event, ev.Slug)
	}
}

func attentionLabel(at service.Attention, staleDays int) string {
	noun := "projects"
	if at.Count == 1 {
		noun = "project"
	}
	switch at.Key {
	case "mismatch_missing":
		return fmt.Sprintf("%d %s registered but missing from disk", at.Count, noun)
	case "unregistered":
		return fmt.Sprintf("%d %s on disk not yet registered", at.Count, noun)
	case "missing_git":
		return fmt.Sprintf("%d %s missing git", at.Count, noun)
	case "missing_bank":
		return fmt.Sprintf("%d %s missing Bank", at.Count, noun)
	case "missing_map":
		return fmt.Sprintf("%d %s missing Map", at.Count, noun)
	case "missing_readme":
		return fmt.Sprintf("%d %s missing README", at.Count, noun)
	case "stale":
		return fmt.Sprintf("%d stale %s (%dd+)", at.Count, noun, staleDays)
	}
	return fmt.Sprintf("%d %s %s", at.Count, noun, at.Key)
}

// ago renders a compact relative time for the snapshot header and the
// activity rows; zero timestamps mean the event never happened.
func ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// bar is a compact in-cell bar chart, capped so a huge outlier cannot
// stretch the row.
func bar(n, total int) string {
	if total <= 0 || n <= 0 {
		return ""
	}
	width := n * 8 / total
	if width < 1 {
		width = 1
	}
	if width > 8 {
		width = 8
	}
	return strings.Repeat("█", width)
}
