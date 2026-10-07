package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sourceSteps lists the wizard's five steps and the fields each one
// owns (spec 3.5); field values arrive with P4.02 onward.
var sourceSteps = []struct {
	title  string
	fields []string
}{
	{"Identity", []string{"Name", "Slug", "Description"}},
	{"Classification", []string{"Flow stage", "Type", "Domain", "Confluences"}},
	{"Stack", []string{"Language", "Template", "Package manager"}},
	{"Automation", []string{"Create Bank", "Bridge init (lode)", "Git init", "Build Map", "Open editor after", "Editor"}},
	{"Dry run", nil},
}

// sourceWizard tracks position only: which step is open (0-based) and
// which field is focused within it.
type sourceWizard struct {
	step  int
	field int
}

// updateSource handles the Source screen's keys (spec 3.9): tab and
// shift+tab move between fields, enter advances a step, ctrl+s is the
// save/create action, esc (or q) cancels the whole wizard.
func (r Root) updateSource(k tea.KeyMsg) (Root, tea.Cmd) {
	w := &r.src
	switch k.String() {
	case "ctrl+c":
		return r, tea.Quit
	case "esc", "q":
		r.screen = ScreenDashboard
		r.src = sourceWizard{}
		r.dashboard.Status = ""
		return r, nil
	case "tab":
		if w.field < len(sourceSteps[w.step].fields)-1 {
			w.field++
		}
	case "shift+tab":
		if w.field > 0 {
			w.field--
		}
	case "enter":
		if w.step < len(sourceSteps)-1 {
			w.step++
			w.field = 0
			r.dashboard.Status = ""
		}
	case "ctrl+s":
		if w.step < len(sourceSteps)-1 {
			r.dashboard.Status = fmt.Sprintf("Step %d of %d — enter to continue", w.step+1, len(sourceSteps))
		} else {
			r.dashboard.Status = "Nothing created yet — the dry run is not ready, press esc to leave"
		}
	}
	return r, nil
}

// sourceView renders the wizard: title, five-step indicator, the
// current step's field rows and the wizard key hints.
func (r Root) sourceView() string {
	w := r.src
	step := sourceSteps[w.step]
	var b strings.Builder
	b.WriteString(r.styleTitle.Render("Source — " + step.title))
	b.WriteString("\n" + r.styleMuted.Render(strings.Repeat("─", lipgloss.Width(step.title)+8)))

	b.WriteString("\n\n")
	for i, s := range sourceSteps {
		label := fmt.Sprintf("%d %s", i+1, s.title)
		switch {
		case i == w.step:
			b.WriteString(selectedStyle.Render("▸ " + label))
		case i < w.step:
			b.WriteString(r.styleMuted.Render("✓ " + label))
		default:
			b.WriteString(r.styleMuted.Render(label))
		}
		if i < len(sourceSteps)-1 {
			b.WriteString(r.styleMuted.Render(" · "))
		}
	}

	b.WriteString("\n\n")
	for i, f := range step.fields {
		line := "  " + padCell(f+":", 20) + r.styleMuted.Render("—")
		if i == w.field {
			line = selectedStyle.Render("▸ "+padCell(f+":", 20)) + "—"
		}
		b.WriteString(line + "\n")
	}
	if len(step.fields) == 0 {
		b.WriteString(r.styleMuted.Render("Press ctrl+s to run the dry run.") + "\n")
	}
	if s := r.dashboard.Status; s != "" {
		b.WriteString("\n" + r.styleMuted.Render(s))
	}
	b.WriteString("\n\n" + r.styleMuted.Render("tab next field • shift+tab previous • enter next step • ctrl+s save • esc cancel"))
	return bgStyle.Width(r.width).Render(b.String())
}
