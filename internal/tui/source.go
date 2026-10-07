package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/slug"
)

// sourceSteps lists the wizard's five steps and the fields each one
// owns (spec 3.5); Classification onward arrive with P4.03+.
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

// sourceWizard tracks position plus the Identity values; Name and
// Description are edited through one shared textinput, Slug is a live
// preview of slug.Make(Name) and never typed (spec 3.5).
type sourceWizard struct {
	step        int
	field       int
	name        string
	description string
	input       textinput.Model
}

// newSourceWizard opens step 1 with the Name field focused.
func newSourceWizard() sourceWizard {
	in := textinput.New()
	in.Prompt = ""
	in.Width = 40
	in.Focus()
	return sourceWizard{input: in}
}

// editable reports whether the focused field is a typed field (Slug
// and every field past step 1 are read-only for now).
func (w sourceWizard) editable() bool {
	return w.step == 0 && (w.field == 0 || w.field == 2)
}

func (w sourceWizard) value() string {
	if w.step != 0 {
		return ""
	}
	switch w.field {
	case 0:
		return w.name
	case 2:
		return w.description
	}
	return ""
}

// focusField loads the focused field into the input (or blurs it for
// read-only rows) and returns the cursor-blink command.
func (w *sourceWizard) focusField() tea.Cmd {
	if !w.editable() {
		w.input.Blur()
		return nil
	}
	w.input.SetValue(w.value())
	w.input.CursorEnd()
	return w.input.Focus()
}

// commitField writes the input back into the wizard's value.
func (w *sourceWizard) commitField() {
	if !w.editable() {
		return
	}
	switch w.field {
	case 0:
		w.name = w.input.Value()
	case 2:
		w.description = w.input.Value()
	}
}

// identityHint is the inline validation for step 1 (P4.02): a missing
// name or one slug.Make rejects gets a teach-the-fix message.
func identityHint(name string) string {
	t := strings.TrimSpace(name)
	if t == "" {
		return "Name is required"
	}
	if s, err := slug.Make(t); err != nil || s == "" {
		return "Name cannot be used as a project slug"
	}
	return ""
}

// updateSource handles the Source screen's keys (spec 3.9): tab and
// shift+tab move between fields, enter advances a step, ctrl+s is the
// save/create action, esc (or q) cancels the whole wizard. Everything
// else types into the focused field.
func (r Root) updateSource(k tea.KeyMsg) (Root, tea.Cmd) {
	w := &r.src
	switch k.String() {
	case "ctrl+c":
		return r, tea.Quit
	case "esc":
		r.screen = ScreenDashboard
		r.src = sourceWizard{}
		r.dashboard.Status = ""
		return r, nil
	case "q":
		// q leaves the wizard, but a focused field types it verbatim
		// (a project name may contain q).
		if !w.input.Focused() {
			r.screen = ScreenDashboard
			r.src = sourceWizard{}
			r.dashboard.Status = ""
			return r, nil
		}
	case "tab":
		w.commitField()
		if w.field < len(sourceSteps[w.step].fields)-1 {
			w.field++
		}
		return r, w.focusField()
	case "shift+tab":
		w.commitField()
		if w.field > 0 {
			w.field--
		}
		return r, w.focusField()
	case "enter":
		w.commitField()
		if w.step < len(sourceSteps)-1 {
			w.step++
			w.field = 0
			r.dashboard.Status = ""
			return r, w.focusField()
		}
		return r, nil
	case "ctrl+s":
		if w.step < len(sourceSteps)-1 {
			r.dashboard.Status = fmt.Sprintf("Step %d of %d — enter to continue", w.step+1, len(sourceSteps))
		} else {
			r.dashboard.Status = "Nothing created yet — the dry run is not ready, press esc to leave"
		}
		return r, nil
	}
	if w.input.Focused() {
		in, cmd := w.input.Update(k)
		w.input = in
		w.commitField() // live values: name drives the slug preview
		return r, cmd
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
		value := r.styleMuted.Render("—")
		if w.step == 0 {
			switch {
			case i == 0:
				if w.field == 0 {
					value = w.input.View()
				} else if w.name != "" {
					value = w.name
				}
			case i == 1:
				if s, err := slug.Make(strings.TrimSpace(w.name)); err == nil && s != "" {
					value = r.styleMuted.Render(s)
				}
			case i == 2:
				if w.field == 2 {
					value = w.input.View()
				} else if w.description != "" {
					value = w.description
				}
			}
		}
		line := "  " + padCell(f+":", 20) + value
		if i == w.field {
			line = selectedStyle.Render("▸ "+padCell(f+":", 20)) + value
		}
		b.WriteString(line + "\n")
		if w.step == 0 && i == 0 {
			if hint := identityHint(w.name); hint != "" {
				b.WriteString(r.styleErr.Render("  ! "+hint) + "\n")
			}
		}
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
