package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/service/fake"
)

func keyTab() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyTab} }
func keyShiftTab() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyShiftTab}
}
func keyCtrlS() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlS} }
func keyLeft() tea.KeyMsg  { return tea.KeyMsg{Type: tea.KeyLeft} }
func keyRight() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRight} }

// keySpace mirrors what the TTY parser produces: Type=KeySpace with
// the rune kept in Runes (bubbletea key.go:698-700).
func keySpace() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}} }

// openSource runs the `n` dispatch command and feeds the resulting
// screen switch back into the model.
func openSource(t *testing.T, r Root) Root {
	t.Helper()
	r, cmd := upd(t, r, runeKey("n"))
	if cmd != nil {
		r, _ = upd(t, r, cmd())
	}
	return r
}

// TestSourceWizardScaffold (P4.01): n opens the wizard, the indicator
// shows all five steps, tab/shift+tab walk the fields (clamped), enter
// advances steps, ctrl+s teaches what is next, esc and q cancel.
func TestSourceWizardScaffold(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)
	if r.screen != ScreenSource {
		t.Fatalf("screen = %s, want source", screenNames[r.screen])
	}
	v := r.View()
	for _, want := range []string{"Source — Identity", "1 Identity", "2 Classification", "3 Stack", "4 Automation", "5 Dry run", "Name:"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}

	// tab walks fields, clamped at the end of the step
	for range 5 {
		r, _ = upd(t, r, keyTab())
	}
	if r.src.field != 2 { // Identity has 3 fields
		t.Errorf("field = %d, want clamped at 2", r.src.field)
	}
	// shift+tab walks back, clamped at 0
	for range 5 {
		r, _ = upd(t, r, keyShiftTab())
	}
	if r.src.field != 0 {
		t.Errorf("field = %d, want clamped at 0", r.src.field)
	}

	// enter advances steps and resets the field; clamps at the last step
	for range 6 {
		r, _ = upd(t, r, keyEnter())
	}
	if r.src.step != len(sourceSteps)-1 || r.src.field != 0 {
		t.Fatalf("step = %d field = %d, want last step", r.src.step, r.src.field)
	}
	v = r.View()
	if !strings.Contains(v, "Source — Dry run") || !strings.Contains(v, "✓ 1 Identity") || !strings.Contains(v, "▸ 5 Dry run") {
		t.Errorf("step 5 view missing updated indicator: %q", v)
	}
	// arriving at the dry run kicks off the plan preview (P4.06);
	// ctrl+s while it loads does nothing (single in-flight request)
	if !r.src.loading {
		t.Error("step 5 must start the plan preview")
	}
	r, c := upd(t, r, keyCtrlS())
	if c != nil || !r.src.loading {
		t.Error("ctrl+s during the preview must not fire a second request")
	}
	if !strings.Contains(r.View(), "Building the plan") {
		t.Errorf("dry run view missing the loading line: %q", r.View())
	}

	// esc cancels everything; the wizard resets
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard || r.src.step != 0 || r.src.field != 0 || r.src.name != "" {
		t.Errorf("esc: screen=%s src step=%d field=%d name=%q", screenNames[r.screen], r.src.step, r.src.field, r.src.name)
	}

	// ctrl+s on a mid step teaches the way forward
	r = openSource(t, r)
	r, _ = upd(t, r, keyCtrlS())
	if !strings.Contains(r.dashboard.Status, "Step 1 of 5") {
		t.Errorf("ctrl+s status = %q", r.dashboard.Status)
	}

	// q also cancels — the Flow selector row is read-only (blurred), so
	// q leaves instead of typing; on a text field it types instead.
	r, _ = upd(t, r, keyEnter())
	if r.src.input.Focused() {
		t.Fatal("flow selector must not focus the text input")
	}
	r, _ = upd(t, r, keyR('q'))
	if r.screen != ScreenDashboard || r.src.step != 0 {
		t.Errorf("q: screen=%s src=%+v", screenNames[r.screen], r.src)
	}

	// ctrl+c always quits
	r = openSource(t, r)
	var cmd tea.Cmd
	r, cmd = upd(t, r, tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c returned %T, want QuitMsg", cmd())
	}
}

// TestSourceWizardFreshOnOpen: leaving and reopening starts over.
func TestSourceWizardFreshOnOpen(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)
	r, _ = upd(t, r, keyEnter())
	r, _ = upd(t, r, keyTab())
	r, _ = upd(t, r, keyEsc())
	r = openSource(t, r)
	if r.src.step != 0 || r.src.field != 0 || r.src.name != "" || r.src.description != "" {
		t.Errorf("reopened wizard = %+v, want fresh", r.src)
	}
	if !r.src.input.Focused() {
		t.Error("reopened wizard must focus Name")
	}
}

// TestSourceIdentityStep (P4.02): typed name with live slug preview
// and inline validation, read-only Slug, editable Description, values
// kept when the step advances.
func TestSourceIdentityStep(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)

	// fresh wizard teaches the missing field
	if !strings.Contains(r.View(), "Name is required") {
		t.Fatal("fresh wizard missing the required-name hint")
	}

	// typing captures live (the name contains q: it must type, not cancel)
	for _, c := range "quiet repo" {
		r, _ = upd(t, r, keyR(c))
	}
	if r.src.name != "quiet repo" {
		t.Fatalf("name = %q, want %q", r.src.name, "quiet repo")
	}
	v := r.View()
	if !strings.Contains(v, "quiet-repo") {
		t.Errorf("view missing live slug preview: %q", v)
	}
	if strings.Contains(v, "Name is required") {
		t.Error("hint must clear once a name exists")
	}

	// Slug is a preview: the field blurs and stray keys are inert
	r, _ = upd(t, r, keyTab())
	if r.src.input.Focused() {
		t.Fatal("Slug must be read-only")
	}
	r, _ = upd(t, r, keyR('x'))
	if r.src.name != "quiet repo" {
		t.Errorf("typing on Slug changed name to %q", r.src.name)
	}

	// Description takes the typed value
	r, _ = upd(t, r, keyTab())
	if !r.src.input.Focused() {
		t.Fatal("Description must be focused")
	}
	for _, c := range "does things" {
		r, _ = upd(t, r, keyR(c))
	}
	if r.src.description != "does things" {
		t.Fatalf("description = %q", r.src.description)
	}

	// enter keeps Identity values on step 2
	r, _ = upd(t, r, keyEnter())
	if r.src.step != 1 || r.src.name != "quiet repo" || r.src.description != "does things" {
		t.Fatalf("step 2 wizard = %+v", r.src)
	}

	// a name slug.Make rejects gets the inline explanation
	r, _ = upd(t, r, keyEsc())
	r = openSource(t, r)
	for _, c := range "a/b" {
		r, _ = upd(t, r, keyR(c))
	}
	if !strings.Contains(r.View(), "Name cannot be used as a project slug") {
		t.Errorf("invalid name missing hint: %q", r.View())
	}
}

// TestSourceClassificationStep (P4.03): the Flow stage selector
// cycles with arrows and jumps with digits, Type/Domain/Confluences
// type text, and the values survive the step change.
func TestSourceClassificationStep(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)
	r, _ = upd(t, r, keyEnter()) // -> Classification, Flow stage focused
	if r.src.step != 1 || r.src.field != 0 {
		t.Fatalf("step=%d field=%d, want 1/0", r.src.step, r.src.field)
	}

	v := r.View()
	for _, want := range []string{"Source — Classification", "Flow stage:", "Type:", "Domain:", "Confluences:", "source"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	if !strings.Contains(v, "1-5 to change the stage") {
		t.Error("selector hint missing")
	}

	// arrows cycle through the lifecycle, wrapping at both ends
	r, _ = upd(t, r, keyRight())
	if r.src.flow != "active" {
		t.Errorf("right -> flow = %q", r.src.flow)
	}
	r, _ = upd(t, r, keyLeft())
	r, _ = upd(t, r, keyLeft())
	if r.src.flow != "delta" {
		t.Errorf("wrap left -> flow = %q", r.src.flow)
	}
	// digits jump straight to a stage (flow picker vocabulary)
	r, _ = upd(t, r, keyR('3'))
	if r.src.flow != "maintenance" {
		t.Errorf("digit 3 -> flow = %q", r.src.flow)
	}
	// stray keys on the selector row are inert (input is blurred)
	r, _ = upd(t, r, keyR('z'))
	if r.src.flow != "maintenance" {
		t.Errorf("stray z changed flow to %q", r.src.flow)
	}

	// Type, Domain and Confluences take typed values
	r, _ = upd(t, r, keyTab())
	if !r.src.input.Focused() {
		t.Fatal("Type must be focused")
	}
	for _, c := range "cli" {
		r, _ = upd(t, r, keyR(c))
	}
	r, _ = upd(t, r, keyTab())
	for _, c := range "devtools" {
		r, _ = upd(t, r, keyR(c))
	}
	r, _ = upd(t, r, keyTab())
	for _, c := range "ship,brand" {
		r, _ = upd(t, r, keyR(c))
	}
	if r.src.typ != "cli" || r.src.domain != "devtools" || r.src.confluences != "ship,brand" {
		t.Fatalf("classification = %q/%q/%q", r.src.typ, r.src.domain, r.src.confluences)
	}

	// values are visible and survive into step 3
	v = r.View()
	for _, want := range []string{"cli", "devtools", "ship,brand"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	r, _ = upd(t, r, keyEnter())
	if r.src.step != 2 || r.src.flow != "maintenance" || r.src.typ != "cli" || r.src.domain != "devtools" || r.src.confluences != "ship,brand" {
		t.Fatalf("step 3 wizard = %+v", r.src)
	}
}

// TestSourceStackStep (P4.04): Language, Template and Package manager
// take typed values, are shown on the step, and survive into step 4.
func TestSourceStackStep(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)
	r, _ = upd(t, r, keyEnter())
	r, _ = upd(t, r, keyEnter()) // -> Stack, Language focused
	if r.src.step != 2 || r.src.field != 0 || !r.src.input.Focused() {
		t.Fatalf("stack open: step=%d field=%d focused=%v", r.src.step, r.src.field, r.src.input.Focused())
	}

	v := r.View()
	for _, want := range []string{"Source — Stack", "Language:", "Template:", "Package manager:"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}

	// all three rows type through the shared input
	for _, c := range "go" {
		r, _ = upd(t, r, keyR(c))
	}
	r, _ = upd(t, r, keyTab())
	for _, c := range "go-cli" {
		r, _ = upd(t, r, keyR(c))
	}
	r, _ = upd(t, r, keyTab())
	for _, c := range "go-mod" {
		r, _ = upd(t, r, keyR(c))
	}
	if r.src.language != "go" || r.src.template != "go-cli" || r.src.pkgManager != "go-mod" {
		t.Fatalf("stack = %q/%q/%q", r.src.language, r.src.template, r.src.pkgManager)
	}
	v = r.View()
	for _, want := range []string{"go-cli", "go-mod"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}

	// shift+tab walks back across the rows
	r, _ = upd(t, r, keyShiftTab())
	if r.src.field != 1 || r.src.template != "go-cli" {
		t.Errorf("shift+tab -> field=%d template=%q", r.src.field, r.src.template)
	}

	// values survive into Automation
	r, _ = upd(t, r, keyTab())
	r, _ = upd(t, r, keyEnter())
	if r.src.step != 3 || r.src.language != "go" || r.src.template != "go-cli" || r.src.pkgManager != "go-mod" {
		t.Fatalf("step 4 wizard = %+v", r.src)
	}
}

// TestSourceAutomationStep (P4.05): space toggles the yes/no rows,
// the bridge locks Git init to "via bridge" (spec 1.7), the Editor
// row takes text, and the values survive into the dry run.
func TestSourceAutomationStep(t *testing.T) {
	r, _ := rootOf(t)
	r = openSource(t, r)
	for range 3 {
		r, _ = upd(t, r, keyEnter())
	}
	if r.src.step != 3 || r.src.field != 0 {
		t.Fatalf("automation open: step=%d field=%d", r.src.step, r.src.field)
	}

	v := r.View()
	for _, want := range []string{"Source — Automation", "Create Bank:", "Bridge init (lode):", "Git init:", "Build Map:", "Open editor after:", "Editor:", "space toggles"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	// CLI defaults: bank/git/map/open yes, bridge no
	for _, want := range []string{"yes", "no"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}

	// Create Bank toggles off and back on
	r, _ = upd(t, r, keySpace())
	if r.src.bank {
		t.Fatal("space did not toggle Create Bank off")
	}
	r, _ = upd(t, r, keySpace())
	if !r.src.bank {
		t.Fatal("space did not toggle Create Bank back on")
	}

	// Git init toggles while the bridge is off
	r, _ = upd(t, r, keyTab())
	r, _ = upd(t, r, keyTab()) // field 2: Git init
	r, _ = upd(t, r, keySpace())
	if r.src.git {
		t.Fatal("space did not toggle Git init off")
	}
	r, _ = upd(t, r, keySpace())
	if !r.src.git {
		t.Fatal("space did not toggle Git init back on")
	}

	// Bridge on locks the Git row to "via bridge"
	r, _ = upd(t, r, keyShiftTab()) // field 1: Bridge
	r, _ = upd(t, r, keySpace())
	if !r.src.bridge {
		t.Fatal("space did not toggle the bridge on")
	}
	r, _ = upd(t, r, keyTab()) // back to Git: locked
	r, _ = upd(t, r, keySpace())
	if !r.src.git {
		t.Fatal("locked Git init must not toggle")
	}
	if r.src.gitRow() != "via bridge" {
		t.Fatalf("gitRow = %q, want via bridge", r.src.gitRow())
	}
	if !strings.Contains(r.View(), "via bridge") {
		t.Error("view missing the via bridge lock")
	}
	if !strings.Contains(r.View(), "the bridge owns git init") {
		t.Error("view missing the lock explanation")
	}

	// Editor row takes text (space types there too)
	for range 3 {
		r, _ = upd(t, r, keyTab()) // fields 3, 4, 5
	}
	if r.src.field != 5 || !r.src.input.Focused() {
		t.Fatalf("editor field=%d focused=%v", r.src.field, r.src.input.Focused())
	}
	for _, c := range "nvim" {
		r, _ = upd(t, r, keyR(c))
	}
	r, _ = upd(t, r, keySpace())
	if r.src.editor != "nvim " {
		t.Fatalf("editor = %q, want %q (space must type on the Editor row)", r.src.editor, "nvim ")
	}

	// everything survives into the dry run step
	r, _ = upd(t, r, keyEnter())
	if r.src.step != 4 || !r.src.bank || !r.src.bridge || !r.src.git || !r.src.buildMap || !r.src.openEditor || r.src.editor != "nvim " {
		t.Fatalf("step 5 wizard = %+v", r.src)
	}
}

// walkToDry types a name, steps through to the Dry run and feeds the
// plan preview command so r.src.plan is ready.
func walkToDry(t *testing.T, r Root, f *fake.Service, name string) Root {
	t.Helper()
	for _, c := range name {
		r, _ = upd(t, r, keyR(c))
	}
	for range 4 {
		var c tea.Cmd
		r, c = upd(t, r, keyEnter())
		if c != nil {
			r, _ = upd(t, r, c())
		}
	}
	return r
}

// TestSourceDryRunApply (P4.06): step 5 previews the SourcePlan, y
// applies, and the wizard jumps to the new project; n cancels after
// the dry run with nothing but the preview call made.
func TestSourceDryRunApply(t *testing.T) {
	r, f := rootOf(t)
	f.SourceRes = service.SourceResult{
		Project: registry.Project{ID: "9", Slug: "quiet-repo", Name: "quiet repo", Path: "/w/00_Source/quiet-repo", FlowStage: "source"},
		Plan: service.SourcePlan{
			Name: "quiet repo", Channel: "00_Source",
			Create: []string{"/w/00_Source/quiet-repo"},
			Run:    []string{"git init"},
		},
	}
	r = openSource(t, r)
	r = walkToDry(t, r, f, "quiet repo")

	if r.src.plan == nil || r.src.loading {
		t.Fatalf("dry step: plan=%v loading=%v, want a ready plan", r.src.plan, r.src.loading)
	}
	v := r.View()
	for _, want := range []string{"Source — Dry run", "Dry Run", "Will create /w/00_Source/quiet-repo", "Will run git init", "Confirm? y/N"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q", want)
		}
	}
	if n := strings.Count(strings.Join(f.Calls(), "\n"), "Source quiet repo"); n != 1 {
		t.Fatalf("calls = %v, want exactly the dry-run Source", f.Calls())
	}

	// y applies for real
	r, cmd := upd(t, r, keyR('y'))
	if cmd == nil {
		t.Fatal("y with a ready plan must apply")
	}
	if !r.src.loading {
		t.Error("apply must mark the wizard busy")
	}
	r, cmd = upd(t, r, cmd())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %s, want dashboard after apply", screenNames[r.screen])
	}
	if r.pendingJump != "quiet-repo" {
		t.Errorf("pendingJump = %q", r.pendingJump)
	}
	if n := strings.Count(strings.Join(f.Calls(), "\n"), "Source quiet repo"); n != 2 {
		t.Errorf("calls = %v, want dry-run + apply", f.Calls())
	}

	// the refreshed list (with the new project) lands via the batch:
	// the toast queues and the detail opens on the new project
	f.Projects = append(f.Projects, registry.Project{ID: "9", Slug: "quiet-repo", Name: "quiet repo", FlowStage: "source"})
	bm, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("apply returned %T, want a batch of refresh+toast", cmd())
	}
	for _, c := range bm {
		r, _ = upd(t, r, c())
	}
	if len(r.toasts) == 0 || !strings.Contains(r.toasts[0].Text, "Sourced quiet repo") {
		t.Errorf("toasts = %+v, want the sourced line", r.toasts)
	}
	if r.pendingJump != "" {
		t.Errorf("pendingJump = %q after refresh, want consumed", r.pendingJump)
	}
	if !r.dashboard.DetailFull || r.dashboard.Cursor >= len(r.dashboard.Visible) ||
		r.dashboard.Visible[r.dashboard.Cursor].Slug != "quiet-repo" {
		t.Errorf("jump: full=%v cursor=%d visible=%+v", r.dashboard.DetailFull, r.dashboard.Cursor, r.dashboard.Visible)
	}
}

// TestSourceDryRunCancel: n (and esc) leave without the apply ever
// running, and a preview that returns after cancel is dropped.
func TestSourceDryRunCancel(t *testing.T) {
	r, f := rootOf(t)
	f.SourceRes = service.SourceResult{
		Project: registry.Project{ID: "9", Slug: "quiet-repo", Name: "quiet repo", Path: "/w/00_Source/quiet-repo"},
		Plan:    service.SourcePlan{Name: "quiet repo"},
	}

	// n declines with the plan on screen
	r = openSource(t, r)
	for _, c := range "quiet repo" {
		r, _ = upd(t, r, keyR(c))
	}
	for range 3 {
		r, _ = upd(t, r, keyEnter())
	}
	var preview tea.Cmd
	r, preview = upd(t, r, keyEnter()) // -> dry run, preview in flight
	if preview == nil {
		t.Fatal("entering the dry run must request the plan")
	}
	r, _ = upd(t, r, preview())
	if r.src.plan == nil {
		t.Fatal("preview did not load")
	}
	r, _ = upd(t, r, keyR('n'))
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %s after n, want dashboard", screenNames[r.screen])
	}
	if n := strings.Count(strings.Join(f.Calls(), "\n"), "Source"); n != 1 {
		t.Errorf("calls = %v, want only the dry run", f.Calls())
	}

	// esc while the preview is still in flight drops the stale result
	r = openSource(t, r)
	for _, c := range "late" {
		r, _ = upd(t, r, keyR(c))
	}
	var late tea.Cmd
	for range 3 {
		r, _ = upd(t, r, keyEnter())
	}
	r, late = upd(t, r, keyEnter())
	if late == nil {
		t.Fatal("preview cmd missing")
	}
	r, _ = upd(t, r, keyEsc())
	r, _ = upd(t, r, late())
	if r.src.plan != nil || r.src.loading {
		t.Errorf("stale preview survived cancel: plan=%v loading=%v", r.src.plan, r.src.loading)
	}
	if r.screen != ScreenDashboard {
		t.Errorf("screen = %s, want dashboard", screenNames[r.screen])
	}
}
