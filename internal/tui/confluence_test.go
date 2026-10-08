package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service/fake"
)

// confluenceFixture opens the browser (spec 3.7) with two confluences
// and two members; the root and the dashboard share one fake so Root's
// own commands are recorded too.
func confluenceFixture(t *testing.T) (Root, *fake.Service) {
	t.Helper()
	m, f := scanFixture(t)
	f.ConflRes = []registry.Confluence{
		{ID: "c1", Name: "heap-stack", Members: 2},
		{ID: "c2", Name: "devtools", Members: 0},
	}
	f.ConflShowRes = registry.Confluence{ID: "c1", Name: "heap-stack", Members: 2}
	f.ConflMembers = []registry.Project{
		{ID: "1", Name: "Alpha", Slug: "alpha", FlowStage: "active", Language: "go"},
		{ID: "2", Name: "Beta", Slug: "beta", FlowStage: "source", Language: "rust"},
	}
	m, cmd := updateC(t, m, runeKey("c"))
	if cmd == nil {
		t.Fatal("c must load the confluence browser")
	}
	if !strings.Contains(m.Status, "Loading confluences") {
		t.Fatalf("Status = %q, want progress notice", m.Status)
	}
	r := NewRoot(f, config.Default())
	r.dashboard = m
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenConfluence {
		t.Fatalf("screen = %v, want confluence", r.screen)
	}
	return r, f
}

// TestConfluenceBrowserOpensAndRenders: c loads the list with counts,
// the focused members below and the spec 3.7 footer; esc goes back.
func TestConfluenceBrowserOpensAndRenders(t *testing.T) {
	r, _ := confluenceFixture(t)
	v := r.View()
	for _, want := range []string{
		"CONFLUENCES", "heap-stack", "2 projects", "devtools", "0 projects",
		"MEMBERS - heap-stack", "Alpha", "active", "go", "Beta",
		"[n] new", "[e] membership", "[enter] open",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("browser view missing %q in %q", want, v)
		}
	}
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after esc", r.screen)
	}
}

// TestConfluenceBrowserEmpty: an empty registry still opens the screen
// and explains how to create the first confluence.
func TestConfluenceBrowserEmpty(t *testing.T) {
	m, f := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("c"))
	r := NewRoot(f, config.Default())
	r.dashboard = m
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenConfluence {
		t.Fatalf("screen = %v, want confluence for an empty list", r.screen)
	}
	if v := r.View(); !strings.Contains(v, "No confluences yet") {
		t.Errorf("empty browser = %q, want the create hint", v)
	}
}

// TestConfluenceLoadErrorStaysPut: a failed load lands in the sticky
// banner and never switches away from the dashboard.
func TestConfluenceLoadErrorStaysPut(t *testing.T) {
	m, f := scanFixture(t)
	f.ConflErr = errors.New("db locked")
	m, cmd := updateC(t, m, runeKey("c"))
	r := NewRoot(f, config.Default())
	r.dashboard = m
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after a failed load", r.screen)
	}
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], "could not load confluences: db locked") {
		t.Fatalf("errs = %v, want the sticky failure", r.errs)
	}
}

// TestConfluenceBrowserPanesAndMotion: tab switches between the list
// and member panes; up/down/pgup/pgdn/home/end stay clamped inside the
// focused pane.
func TestConfluenceBrowserPanesAndMotion(t *testing.T) {
	r, _ := confluenceFixture(t)

	r, _ = upd(t, r, runeKey("j"))
	if r.conflCursor != 1 {
		t.Fatalf("cursor = %d, want 1", r.conflCursor)
	}
	r, _ = upd(t, r, runeKey("j"))
	if r.conflCursor != 1 {
		t.Fatalf("cursor past the end = %d, want clamped at 1", r.conflCursor)
	}
	r, _ = upd(t, r, runeKey("k"))
	if r.conflCursor != 0 {
		t.Fatalf("cursor = %d, want 0", r.conflCursor)
	}

	r, _ = upd(t, r, runeKey("tab"))
	if r.conflFocus != 1 {
		t.Fatalf("focus = %d, want the member pane", r.conflFocus)
	}
	r, _ = upd(t, r, runeKey("j"))
	if r.conflMember != 1 {
		t.Fatalf("member = %d, want 1", r.conflMember)
	}
	r, _ = upd(t, r, runeKey("k"))
	r, _ = upd(t, r, runeKey("k"))
	if r.conflMember != 0 {
		t.Fatalf("member above the top = %d, want 0", r.conflMember)
	}
	// end lands on the last member, tab returns and resets the member row
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEnd})
	if r.conflMember != 1 {
		t.Fatalf("end member = %d, want 1", r.conflMember)
	}
	r, _ = upd(t, r, runeKey("tab"))
	if r.conflFocus != 0 || r.conflMember != 0 {
		t.Fatalf("focus/member = %d/%d, want list pane with reset row", r.conflFocus, r.conflMember)
	}
	// enter on the list moves focus to the members (spec: [enter] open)
	r, _ = upd(t, r, keyEnter())
	if r.conflFocus != 1 {
		t.Fatalf("enter focus = %d, want the member pane", r.conflFocus)
	}
}

// TestConfluenceNewPrompt: n opens the name prompt, esc cancels it and
// enter runs the create, reloading the browser afterwards.
func TestConfluenceNewPrompt(t *testing.T) {
	r, f := confluenceFixture(t)

	r, cmd := upd(t, r, runeKey("n"))
	if r.conflTIMode != "new" {
		t.Fatalf("mode = %q, want new", r.conflTIMode)
	}
	if cmd == nil {
		t.Error("n must return the input's focus command")
	}
	if v := r.View(); !strings.Contains(v, "NEW CONFLUENCE") {
		t.Fatalf("prompt = %q, want the input line", v)
	}
	// the prompt owns the keys: tab must not switch panes
	r, _ = upd(t, r, runeKey("tab"))
	if r.conflTIMode != "new" {
		t.Fatal("the prompt must swallow tab")
	}
	r, _ = upd(t, r, keyEsc())
	if r.conflTIMode != "" {
		t.Fatal("esc must cancel the prompt")
	}

	// an empty name warns without a command
	r, _ = upd(t, r, runeKey("n"))
	r, cmd = upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter must answer the empty name")
	}
	if msg := cmd(); !strings.Contains(msg.(toastMsg).t.Text, "no name given") {
		t.Fatalf("toast = %#v, want the empty-name warning", msg)
	}

	// a real name creates and reloads
	f.ConflNewRes = registry.Confluence{ID: "c9", Name: "zen"}
	r, _ = upd(t, r, runeKey("n"))
	r, _ = upd(t, r, runeKey("zen"))
	r, cmd = upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter must run the create")
	}
	mm, ok := cmd().(conflMutatedMsg)
	if !ok || mm.err != nil || !strings.Contains(mm.ok, "created confluence zen") {
		t.Fatalf("msg = %#v, want the create result", mm)
	}
	r, cmd = upd(t, r, mm)
	if cmd == nil {
		t.Fatal("a successful mutation must reload the browser")
	}
	if !contains(f.Calls(), "ConfluenceNew zen") {
		t.Errorf("Calls = %v, want ConfluenceNew zen", f.Calls())
	}
	if len(r.toasts) != 1 || !strings.Contains(r.toasts[0].Text, "created confluence zen") {
		t.Errorf("toasts = %#v, want the confirmation", r.toasts)
	}
}

// TestConfluenceRenamePrompt: r prefills the focused name; enter renames
// and reports the stored name back.
func TestConfluenceRenamePrompt(t *testing.T) {
	r, f := confluenceFixture(t)
	f.ConflRename = "heap-stack-2"

	r, _ = upd(t, r, runeKey("r"))
	if r.conflTIMode != "rename" || r.conflTIQ != "c1" {
		t.Fatalf("mode/q = %q/%q, want rename/c1", r.conflTIMode, r.conflTIQ)
	}
	if v := r.View(); !strings.Contains(v, "RENAME") {
		t.Fatalf("prompt = %q, want the rename input", v)
	}
	r, _ = upd(t, r, runeKey("-2"))
	r, cmd := upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter must run the rename")
	}
	mm := cmd().(conflMutatedMsg)
	if mm.err != nil {
		t.Fatalf("rename failed: %v", mm.err)
	}
	if !contains(f.Calls(), "ConfluenceRename c1 -> heap-stack-2") {
		t.Errorf("Calls = %v, want the rename", f.Calls())
	}
	r, _ = upd(t, r, mm)
	if len(r.toasts) != 1 || !strings.Contains(r.toasts[0].Text, "renamed to heap-stack-2") {
		t.Errorf("toasts = %#v, want the stored name", r.toasts)
	}
}

// TestConfluenceRemoveConfirm: x queues a confirm that defaults to No;
// only y deletes, and the delete reloads the browser.
func TestConfluenceRemoveConfirm(t *testing.T) {
	r, f := confluenceFixture(t)

	r, cmd := upd(t, r, runeKey("x"))
	if cmd == nil {
		t.Fatal("x must queue the confirm")
	}
	r, _ = upd(t, r, cmd())
	if len(r.confirms) != 1 {
		t.Fatalf("confirms = %d, want 1", len(r.confirms))
	}
	if v := r.View(); !strings.Contains(v, "Remove confluence heap-stack?") {
		t.Fatalf("modal = %q, want the delete question", v)
	}
	r, _ = upd(t, r, keyEnter())
	if len(r.confirms) != 0 || contains(f.Calls(), "ConfluenceDelete c1") {
		t.Fatalf("enter must decline: confirms=%d calls=%v", len(r.confirms), f.Calls())
	}

	r, cmd = upd(t, r, runeKey("x"))
	r, _ = upd(t, r, cmd())
	r, cmd = upd(t, r, keyR('y'))
	if cmd == nil {
		t.Fatal("y must run the delete")
	}
	mm := cmd().(conflMutatedMsg)
	if mm.err != nil {
		t.Fatalf("delete failed: %v", mm.err)
	}
	if !contains(f.Calls(), "ConfluenceDelete c1") {
		t.Errorf("Calls = %v, want ConfluenceDelete c1", f.Calls())
	}
	r, cmd = upd(t, r, mm)
	if cmd == nil {
		t.Fatal("a successful delete must reload")
	}
	if len(r.toasts) == 0 || !strings.Contains(r.toasts[0].Text, "no project touched") {
		t.Errorf("toasts = %#v, want the safety note", r.toasts)
	}
}

// TestMembershipOverlayFromBrowser: e opens the project checkboxes for
// the focused confluence; space toggles, enter applies only the diff
// and refreshes the member counts, esc discards without a call.
func TestMembershipOverlayFromBrowser(t *testing.T) {
	r, f := confluenceFixture(t)

	r, cmd := upd(t, r, runeKey("e"))
	if cmd == nil {
		t.Fatal("e must load the membership overlay")
	}
	em := cmd().(conflEditMsg)
	if em.err != nil {
		t.Fatalf("load failed: %v", em.err)
	}
	r, _ = upd(t, r, em)
	if r.conflEdit == nil {
		t.Fatal("the overlay must open on the browser")
	}
	v := r.View()
	for _, want := range []string{"Membership - heap-stack", "0 of 2 selected", "Alpha", "Beta"} {
		if !strings.Contains(v, want) {
			t.Errorf("overlay missing %q in %q", want, v)
		}
	}

	// esc discards: no apply command, nothing recorded
	r, cmd = upd(t, r, keyEsc())
	if cmd != nil {
		t.Errorf("esc must queue nothing, got %v", cmd)
	}
	if r.conflEdit != nil {
		t.Fatal("esc must close the overlay")
	}
	if calls := f.Calls(); contains(calls, "ConfluenceAdd a c1") || contains(calls, "ConfluenceRemove a c1") {
		t.Fatalf("esc must not write: %v", calls)
	}

	// fresh load: toggle the first project (slug "a"), enter applies
	r, cmd = upd(t, r, runeKey("e"))
	em = cmd().(conflEditMsg)
	r, _ = upd(t, r, em)
	r, _ = upd(t, r, keySpace())
	if v := r.View(); !strings.Contains(v, "1 of 2 selected") {
		t.Fatalf("toggle = %q, want 1 of 2 selected", v)
	}
	r, cmd = upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter must apply the diff")
	}
	if r.conflEdit != nil {
		t.Fatal("apply must close the overlay")
	}
	am := cmd().(membershipAppliedMsg)
	if am.err != nil {
		t.Fatalf("apply failed: %v", am.err)
	}
	if !contains(f.Calls(), "ConfluenceAdd a c1") {
		t.Errorf("Calls = %v, want ConfluenceAdd a c1", f.Calls())
	}
	r, cmd = upd(t, r, am)
	if cmd == nil {
		t.Fatal("a successful apply must reload the browser")
	}
	if len(r.toasts) != 1 || r.toasts[0].Text != "membership updated" {
		t.Errorf("toasts = %#v, want the confirmation", r.toasts)
	}
}

// TestMembershipApplyFailureIsSticky: a failing write lands in the
// sticky banner naming the row, and the overlay still closes.
func TestMembershipApplyFailureIsSticky(t *testing.T) {
	r, f := confluenceFixture(t)
	r, cmd := upd(t, r, runeKey("e"))
	r, _ = upd(t, r, cmd().(conflEditMsg))
	r, _ = upd(t, r, keySpace())

	f.ConflErr = errors.New("disk full")
	r, cmd = upd(t, r, keyEnter())
	am := cmd().(membershipAppliedMsg)
	if am.err == nil || !strings.Contains(am.err.Error(), "disk full") {
		t.Fatalf("apply err = %v, want disk full", am.err)
	}
	r, _ = upd(t, r, am)
	if r.conflEdit != nil {
		t.Error("a failed apply must still close the overlay")
	}
	if len(r.errs) != 1 || !strings.Contains(r.errs[0], "membership changed") {
		t.Fatalf("errs = %v, want the sticky failure", r.errs)
	}
}

// TestMembershipOverlayFromDetail: `c` on the full-screen Detail opens
// the confluence checkboxes for the selected project (spec 3.4); only
// the toggled row is written.
func TestMembershipOverlayFromDetail(t *testing.T) {
	m, f := scanFixture(t)
	f.ConflRes = []registry.Confluence{
		{ID: "c1", Name: "heap-stack"},
		{ID: "c2", Name: "devtools"},
	}
	f.ConflNames = []string{"heap-stack"}
	m = update(t, m, runeKey("d"))
	if !m.DetailFull {
		t.Fatal("d must open the full-screen detail")
	}
	m, cmd := updateC(t, m, runeKey("c"))
	if cmd == nil {
		t.Fatal("c on Detail must load the membership overlay")
	}
	r := NewRoot(f, config.Default())
	r.dashboard = m
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r, cmd = upd(t, r, cmd())
	if cmd != nil {
		t.Fatalf("open must not queue work, got %v", cmd)
	}
	if r.dashboard.conflEdit == nil {
		t.Fatal("the overlay must open on the Detail screen")
	}
	v := r.View()
	for _, want := range []string{"Confluences - a", "[x] heap-stack", "[ ] devtools", "1 of 2 selected"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail overlay missing %q in %q", want, v)
		}
	}
	// toggle devtools on; heap-stack is already correct and skipped
	r, _ = upd(t, r, runeKey("j"))
	r, _ = upd(t, r, keySpace())
	r, cmd = upd(t, r, keyEnter())
	if cmd == nil {
		t.Fatal("enter must apply the diff")
	}
	am := cmd().(membershipAppliedMsg)
	if am.err != nil {
		t.Fatalf("apply failed: %v", am.err)
	}
	r, _ = upd(t, r, am)
	if r.dashboard.conflEdit != nil {
		t.Fatal("apply must close the Detail overlay")
	}
	if !contains(f.Calls(), "ConfluenceAdd a devtools") {
		t.Errorf("Calls = %v, want only the toggled row", f.Calls())
	}
	if contains(f.Calls(), "ConfluenceAdd a heap-stack") {
		t.Errorf("unchanged rows must not be written: %v", f.Calls())
	}
}

// TestConfluenceKeyBinding: `c` is in the table and the short help
// stays at its fixed eight entries.
func TestConfluenceKeyBinding(t *testing.T) {
	if keys.Confluence.Help().Key != "c" || keys.Confluence.Help().Desc != "confluences" {
		t.Fatalf("binding = %+v, want c/confluences", keys.Confluence.Help())
	}
	if n := len(keys.ShortHelp()); n != 8 {
		t.Errorf("ShortHelp = %d entries, want 8", n)
	}
	found := false
	for _, b := range keys.FullHelp()[1] {
		if b.Help().Key == "c" {
			found = true
		}
	}
	if !found {
		t.Error("Confluence must appear in the FullHelp actions column")
	}
}
