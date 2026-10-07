package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/service"
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

// sourceWizard tracks position plus the Identity and Classification
// values; text fields share one textinput, Slug and Flow stage are
// read-only/selector rows (spec 3.5).
type sourceWizard struct {
	step        int
	field       int
	name        string
	description string
	// step 2 Classification (P4.03)
	flow        string // "" means the default first stage
	typ         string
	domain      string
	confluences string
	// step 3 Stack (P4.04)
	language   string
	template   string
	pkgManager string
	// step 4 Automation (P4.05): defaults match the CLI flags
	// (--git true, --bridge false; Bank/Map open-editor on)
	bank, bridge, git, buildMap, openEditor bool
	editor                                  string
	input                                   textinput.Model
	// step 5 Dry run (P4.06)
	plan    *service.SourcePlan
	loading bool
}

// newSourceWizard opens step 1 with the Name field focused; Bank and
// Map rows seed from [automation] so an untouched row preserves the
// config value (CLI: --git true, --bridge false; open editor on).
func newSourceWizard(cfg config.Config) sourceWizard {
	in := textinput.New()
	in.Prompt = ""
	in.Width = 40
	in.Focus()
	return sourceWizard{
		bank:       cfg.Automation.CreateBank,
		git:        true,
		buildMap:   cfg.Automation.BuildMap,
		openEditor: true,
		input:      in,
	}
}

// editable reports whether the focused field takes text input: Slug
// and Flow stage are selector rows; Identity, Classification, Stack
// and the Automation Editor field type; the Automation toggles use
// space.
func (w sourceWizard) editable() bool {
	switch w.step {
	case 0:
		return w.field == 0 || w.field == 2
	case 1:
		return w.field >= 1
	case 2:
		return true
	case 3:
		return w.field == 5
	}
	return false
}

func (w sourceWizard) value() string {
	if w.step == 0 {
		switch w.field {
		case 0:
			return w.name
		case 2:
			return w.description
		}
		return ""
	}
	return w.valueAt(w.field)
}

// boolRow renders a yes/no toggle the way spec 3.5 shows it.
func boolRow(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// gitRow is the Git init row: the bridge lock (spec 1.7) wins and the
// row reads "via bridge" instead of its own value.
func (w sourceWizard) gitRow() string {
	if w.bridge {
		return "via bridge"
	}
	return boolRow(w.git)
}

// toggle flips the focused Automation yes/no field; Git init is
// locked while the bridge owns it.
func (w *sourceWizard) toggle() {
	switch w.field {
	case 0:
		w.bank = !w.bank
	case 1:
		w.bridge = !w.bridge
	case 2:
		if !w.bridge {
			w.git = !w.git
		}
	case 3:
		w.buildMap = !w.buildMap
	case 4:
		w.openEditor = !w.openEditor
	}
}

// flowValue is the Flow stage the selector shows; "" means the
// lifecycle's first stage (source).
func (w sourceWizard) flowValue() string {
	if w.flow == "" {
		return flowOrder[1]
	}
	return w.flow
}

// valueAt reads a typed row's raw value by field index for steps 2+
// (the view shows it when the row is not focused).
func (w sourceWizard) valueAt(i int) string {
	switch w.step {
	case 1:
		switch i {
		case 1:
			return w.typ
		case 2:
			return w.domain
		case 3:
			return w.confluences
		}
	case 2:
		switch i {
		case 0:
			return w.language
		case 1:
			return w.template
		case 2:
			return w.pkgManager
		}
	case 3:
		if i == 5 {
			return w.editor
		}
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
	switch w.step {
	case 0:
		switch w.field {
		case 0:
			w.name = w.input.Value()
		case 2:
			w.description = w.input.Value()
		}
	case 1:
		switch w.field {
		case 1:
			w.typ = w.input.Value()
		case 2:
			w.domain = w.input.Value()
		case 3:
			w.confluences = w.input.Value()
		}
	case 2:
		switch w.field {
		case 0:
			w.language = w.input.Value()
		case 1:
			w.template = w.input.Value()
		case 2:
			w.pkgManager = w.input.Value()
		}
	case 3:
		if w.field == 5 {
			w.editor = w.input.Value()
		}
	}
}

// cycleFlow moves the stage selector (flow picker keys, dashboard
// vocabulary): arrows step, digits jump.
func (w *sourceWizard) cycleFlow(dir int) {
	stages := flowOrder[1:]
	i := slices.Index(stages, w.flowValue())
	w.flow = stages[((i+dir)%len(stages)+len(stages))%len(stages)]
}

func (w *sourceWizard) setFlow(i int) {
	if i >= 0 && i < len(flowOrder)-1 {
		w.flow = flowOrder[i+1]
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
			if w.step == len(sourceSteps)-1 {
				// step 5 opens by previewing the plan (spec 3.5)
				w.loading = true
				return r, sourceCmd(r.svc, w.name, w.opts(), true)
			}
			return r, w.focusField()
		}
		return r, nil
	case "ctrl+s":
		if w.step < len(sourceSteps)-1 {
			r.dashboard.Status = fmt.Sprintf("Step %d of %d — enter to continue", w.step+1, len(sourceSteps))
			return r, nil
		}
		return r.sourceConfirm()
	case "y", "Y":
		if w.step == len(sourceSteps)-1 {
			return r.sourceConfirm()
		}
	case "n", "N":
		// decline: nothing has been written, leave (spec 3.5 y/N)
		if w.step == len(sourceSteps)-1 && !w.input.Focused() {
			r.screen = ScreenDashboard
			r.src = sourceWizard{}
			r.dashboard.Status = ""
			return r, nil
		}
	case "left", "right", "up", "down":
		// the Flow stage selector (P4.03); on a text field the arrows
		// move the input's cursor below
		if w.step == 1 && w.field == 0 {
			w.cycleFlow(arrowDir(k.String()))
			return r, nil
		}
	case "1", "2", "3", "4", "5":
		if w.step == 1 && w.field == 0 {
			w.setFlow(int(k.String()[0] - '1'))
			return r, nil
		}
	case " ":
		// Automation toggles (P4.05); on the Editor row a space types
		if w.step == 3 && w.field < 5 {
			w.toggle()
			return r, nil
		}
	}
	if w.input.Focused() {
		in, cmd := w.input.Update(k)
		w.input = in
		w.commitField() // live values: name drives the slug preview
		return r, cmd
	}
	return r, nil
}

// opts maps the wizard onto the service's Source inputs; the dry run
// and the apply share it (spec 3.5).
func (w sourceWizard) opts() service.SourceOpts {
	var conv []string
	for _, c := range strings.Split(w.confluences, ",") {
		if c = strings.TrimSpace(c); c != "" {
			conv = append(conv, c)
		}
	}
	return service.SourceOpts{
		Flow:        w.flow,
		Domain:      w.domain,
		Type:        w.typ,
		Language:    w.language,
		Template:    w.template,
		Description: w.description,
		Confluence:  conv,
		Git:         w.git,
		Bridge:      w.bridge,
		CreateBank:  &w.bank,
		BuildMap:    &w.buildMap,
	}
}

// sourceDryMsg is the previewed plan; sourceApplyMsg is the real run.
type sourceDryMsg struct {
	res service.SourceResult
	err error
}

type sourceApplyMsg struct {
	res service.SourceResult
	err error
}

func sourceCmd(svc service.Service, name string, o service.SourceOpts, dry bool) tea.Cmd {
	o.Dry = dry
	return func() tea.Msg {
		res, err := svc.Source(name, o)
		if dry {
			return sourceDryMsg{res: res, err: err}
		}
		return sourceApplyMsg{res: res, err: err}
	}
}

// sourceConfirm is the dry step's confirm action (y / ctrl+s): with a
// ready plan it applies for real, otherwise it (re)builds the preview.
func (r Root) sourceConfirm() (Root, tea.Cmd) {
	w := &r.src
	if w.loading {
		return r, nil
	}
	if w.plan == nil {
		w.loading = true
		return r, sourceCmd(r.svc, w.name, w.opts(), true)
	}
	w.loading = true
	return r, sourceCmd(r.svc, w.name, w.opts(), false)
}

// arrowDir maps the selector's arrow keys to -1/+1.
func arrowDir(key string) int {
	if key == "left" || key == "up" {
		return -1
	}
	return 1
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
		switch w.step {
		case 0:
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
		case 1, 2:
			if w.step == 1 && i == 0 { // Flow stage selector
				value = w.flowValue()
				if w.field == 0 {
					value = selectedStyle.Render(value)
				}
			} else if w.field == i {
				value = w.input.View()
			} else if v := w.valueAt(i); v != "" {
				value = v
			}
		case 3:
			if i == 5 { // Editor
				if w.field == 5 {
					value = w.input.View()
				} else if w.editor != "" {
					value = w.editor
				}
			} else {
				switch i {
				case 0:
					value = boolRow(w.bank)
				case 1:
					value = boolRow(w.bridge)
				case 2:
					value = r.styleMuted.Render(w.gitRow())
				case 3:
					value = boolRow(w.buildMap)
				case 4:
					value = boolRow(w.openEditor)
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
		if w.step == 1 && i == 0 && w.field == 0 {
			b.WriteString(r.styleMuted.Render("  ←/→ or 1-5 to change the stage") + "\n")
		}
		if w.step == 3 && i == w.field && w.field < 5 {
			if w.field == 2 && w.bridge {
				b.WriteString(r.styleMuted.Render("  locked: the bridge owns git init (spec 1.7)") + "\n")
			} else {
				b.WriteString(r.styleMuted.Render("  space toggles") + "\n")
			}
		}
	}
	if len(step.fields) == 0 {
		// step 5: the SourcePlan preview + confirm (spec 3.5)
		switch {
		case w.loading:
			b.WriteString(r.styleMuted.Render("Building the plan…"))
		case w.plan == nil:
			b.WriteString(r.styleErr.Render("! plan unavailable — ctrl+s retries"))
		default:
			b.WriteString(r.styleMuted.Render("Dry Run") + "\n")
			for _, line := range sourcePlanLines(*w.plan) {
				b.WriteString(line + "\n")
			}
			b.WriteString("\n" + r.styleTitle.Render("Confirm? y/N"))
		}
	}
	if s := r.dashboard.Status; s != "" {
		b.WriteString("\n" + r.styleMuted.Render(s))
	}
	b.WriteString("\n\n" + r.styleMuted.Render("tab next field • shift+tab previous • enter next step • ctrl+s save • esc cancel"))
	return bgStyle.Width(r.width).Render(b.String())
}
