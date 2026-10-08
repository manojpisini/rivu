package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
)

// The Confluence browser (spec 3.7) and the membership overlay it
// shares with the Detail screen (`c`, spec 3.4). Every action here is
// registry-only: creating, renaming and removing a confluence never
// touch a project folder, and removal only unlinks membership
// (safety rule 1).

type (
	// confluenceRow is one confluence with its members loaded for the
	// browser's lower pane.
	confluenceRow struct {
		c       registry.Confluence
		members []registry.Project
	}

	// confluencesMsg carries the whole browser payload; err blocks the
	// screen switch so a failed load stays on the dashboard.
	confluencesMsg struct {
		rows []confluenceRow
		err  error
	}

	// conflMutatedMsg reports a completed create/rename/remove. ok is
	// the success text; err is sticky (a rename can land while its Bank
	// mirror fails, so the reload still runs).
	conflMutatedMsg struct {
		ok  string
		err error
	}

	// conflEditMsg delivers a prepared membership overlay.
	conflEditMsg struct {
		ov  membershipOverlay
		err error
	}

	// membershipAppliedMsg reports the overlay's diff apply.
	membershipAppliedMsg struct{ err error }
)

// confluencesCmd loads every confluence with its members in one
// command; the first open and every post-mutation reload share it.
func confluencesCmd(svc service.Service) tea.Cmd {
	return func() tea.Msg {
		if svc == nil {
			return confluencesMsg{err: errors.New("no service in this session")}
		}
		cs, err := svc.Confluences()
		if err != nil {
			return confluencesMsg{err: err}
		}
		rows := make([]confluenceRow, 0, len(cs))
		for _, c := range cs {
			_, members, e := svc.ConfluenceShow(c.ID)
			if e != nil {
				return confluencesMsg{err: fmt.Errorf("members of %s: %w", c.Name, e)}
			}
			rows = append(rows, confluenceRow{c: c, members: members})
		}
		return confluencesMsg{rows: rows}
	}
}

func conflNewCmd(svc service.Service, name string) tea.Cmd {
	return func() tea.Msg {
		c, err := svc.ConfluenceNew(name, "")
		if err != nil {
			return conflMutatedMsg{err: err}
		}
		return conflMutatedMsg{ok: "created confluence " + c.Name}
	}
}

func conflRenameCmd(svc service.Service, q, newName string) tea.Cmd {
	return func() tea.Msg {
		stored, err := svc.ConfluenceRename(q, newName)
		if err != nil {
			return conflMutatedMsg{err: err}
		}
		return conflMutatedMsg{ok: "renamed to " + stored}
	}
}

func conflDeleteCmd(svc service.Service, q, name string) tea.Cmd {
	return func() tea.Msg {
		if err := svc.ConfluenceDelete(q); err != nil {
			return conflMutatedMsg{err: err}
		}
		return conflMutatedMsg{ok: "removed confluence " + name + " (members unlinked, no project touched)"}
	}
}

// conflEditForConfluenceCmd prepares the browser's membership overlay:
// every project, checked for membership of confluence q (rows are
// project slugs so ConfluenceAdd/Remove can resolve them).
func conflEditForConfluenceCmd(svc service.Service, q string) tea.Cmd {
	return func() tea.Msg {
		c, members, err := svc.ConfluenceShow(q)
		if err != nil {
			return conflEditMsg{err: err}
		}
		ps, err := svc.List(service.Filter{})
		if err != nil {
			return conflEditMsg{err: err}
		}
		ov := membershipOverlay{
			title:      "Membership - " + c.Name,
			confluence: c.ID,
			label:      map[string]string{},
			checked:    map[string]bool{},
			original:   map[string]bool{},
			svc:        svc,
		}
		cur := map[string]bool{}
		for _, m := range members {
			cur[m.Slug] = true
		}
		for _, p := range ps {
			ov.rows = append(ov.rows, p.Slug)
			ov.label[p.Slug] = p.Name
			ov.checked[p.Slug] = cur[p.Slug]
			ov.original[p.Slug] = cur[p.Slug]
		}
		return conflEditMsg{ov: ov}
	}
}

// conflEditForProjectCmd prepares the Detail `c` overlay: every
// confluence, checked for the membership of project slug.
func conflEditForProjectCmd(svc service.Service, slug string) tea.Cmd {
	return func() tea.Msg {
		cs, err := svc.Confluences()
		if err != nil {
			return conflEditMsg{err: err}
		}
		names, err := svc.ProjectConfluences(slug)
		if err != nil {
			return conflEditMsg{err: err}
		}
		have := map[string]bool{}
		for _, n := range names {
			have[n] = true
		}
		ov := membershipOverlay{
			title:    "Confluences - " + slug,
			project:  slug,
			checked:  map[string]bool{},
			original: map[string]bool{},
			svc:      svc,
		}
		for _, c := range cs {
			ov.rows = append(ov.rows, c.Name)
			ov.checked[c.Name] = have[c.Name]
			ov.original[c.Name] = have[c.Name]
		}
		return conflEditMsg{ov: ov}
	}
}

// membershipOverlay toggles which rows belong to a target — either the
// projects of one confluence (browser `e`) or the confluences of one
// project (Detail `c`). It owns the keyboard until it closes; esc/q
// discards, enter applies the diff only (unchanged rows are never
// re-sent).
type membershipOverlay struct {
	title      string
	confluence string // target confluence id (rows are project slugs)
	project    string // target project slug (rows are confluence names)
	rows       []string
	label      map[string]string
	checked    map[string]bool
	original   map[string]bool
	cursor     int
	closed     bool
	svc        service.Service
}

func (o *membershipOverlay) dirty() bool {
	for _, row := range o.rows {
		if o.checked[row] != o.original[row] {
			return true
		}
	}
	return false
}

// key gives the overlay the keyboard (every key is swallowed) and
// returns a command when one must run: apply or quit.
func (o *membershipOverlay) key(x tea.KeyMsg) tea.Cmd {
	switch x.String() {
	case "ctrl+c":
		return tea.Quit
	case "esc", "q":
		o.closed = true
	case "up", "k":
		o.cursor = max(0, o.cursor-1)
	case "down", "j":
		o.cursor = min(max(len(o.rows)-1, 0), o.cursor+1)
	case "pgup":
		o.cursor = max(0, o.cursor-10)
	case "pgdown":
		o.cursor = min(max(len(o.rows)-1, 0), o.cursor+10)
	case "home":
		o.cursor = 0
	case "end":
		o.cursor = max(0, len(o.rows)-1)
	case " ":
		if o.cursor < len(o.rows) {
			row := o.rows[o.cursor]
			o.checked[row] = !o.checked[row]
		}
	case "enter":
		o.closed = true
		if o.dirty() && o.svc != nil {
			return o.applyCmd()
		}
	}
	return nil
}

// applyCmd runs only the diff; each failure names its row so a partial
// apply stays explainable.
func (o membershipOverlay) applyCmd() tea.Cmd {
	svc := o.svc
	return func() tea.Msg {
		var errs []error
		for _, row := range o.rows {
			want := o.checked[row]
			if want == o.original[row] {
				continue
			}
			var err error
			if o.confluence != "" {
				if want {
					err = svc.ConfluenceAdd(row, o.confluence)
				} else {
					_, err = svc.ConfluenceRemove(row, o.confluence)
				}
			} else {
				if want {
					err = svc.ConfluenceAdd(o.project, row)
				} else {
					_, err = svc.ConfluenceRemove(o.project, row)
				}
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", row, err))
			}
		}
		return membershipAppliedMsg{err: errors.Join(errs...)}
	}
}

// view centres a checkbox list (spec 3.7's [e] editor): a selection
// count, the visible window of rows and the toggle/save/cancel hint.
func (o membershipOverlay) view(width int) string {
	cut := func(s string) string {
		if width > 0 {
			return ansi.Truncate(s, max(10, width-6), "…")
		}
		return s
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(o.title))
	sel := 0
	for _, row := range o.rows {
		if o.checked[row] {
			sel++
		}
	}
	b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("%d of %d selected", sel, len(o.rows))))
	if len(o.rows) == 0 {
		b.WriteString("\n\n" + mutedStyle.Render("nothing to pick yet"))
	}
	const vis = 12
	start := min(max(0, o.cursor-6), max(0, len(o.rows)-vis))
	end := min(len(o.rows), start+vis)
	for i := start; i < end; i++ {
		row := o.rows[i]
		mark := "[ ]"
		if o.checked[row] {
			mark = "[x]"
		}
		text := row
		if o.label[row] != "" {
			text = o.label[row]
		}
		if i == o.cursor {
			b.WriteString("\n" + selectedStyle.Render(cut("> "+mark+" "+text)))
			continue
		}
		b.WriteString("\n" + mutedStyle.Render(mark) + " " + cut(text))
	}
	if len(o.rows) > vis {
		b.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("row %d of %d", o.cursor+1, len(o.rows))))
	}
	b.WriteString("\n\n" + mutedStyle.Render("space toggle  enter save  esc cancel"))
	return panelStyle.Render(b.String())
}

// confluenceKeys drives the browser (spec 3.7): two panes — the
// confluence list and its members — plus the new/rename prompt and
// x's delete confirm (default No).
func (r Root) confluenceKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	if r.conflTIMode != "" {
		switch x.String() {
		case "ctrl+c":
			return r, tea.Quit
		case "esc":
			r.conflTI = textinput.Model{}
			r.conflTIMode, r.conflTIQ = "", ""
			return r, nil
		case "enter":
			name := strings.TrimSpace(r.conflTI.Value())
			mode, q := r.conflTIMode, r.conflTIQ
			r.conflTI = textinput.Model{}
			r.conflTIMode, r.conflTIQ = "", ""
			if name == "" {
				return r, ShowToast(Toast{Level: "warn", Text: "no name given - nothing changed"})
			}
			if mode == "rename" {
				return r, conflRenameCmd(r.svc, q, name)
			}
			return r, conflNewCmd(r.svc, name)
		default:
			in, cmd := r.conflTI.Update(x)
			r.conflTI = in
			return r, cmd
		}
	}
	if r.conflEdit != nil {
		cmd := r.conflEdit.key(x)
		if r.conflEdit.closed {
			r.conflEdit = nil
		}
		return r, cmd
	}
	n := len(r.conflRows)
	focused := func() confluenceRow {
		if n == 0 {
			return confluenceRow{}
		}
		return r.conflRows[min(r.conflCursor, n-1)]
	}
	memberCount := func() int {
		if r.conflCursor >= n {
			return 0
		}
		return len(r.conflRows[r.conflCursor].members)
	}
	switch x.String() {
	case "esc", "q":
		r.screen = ScreenDashboard
		return r, nil
	case "ctrl+c":
		return r, tea.Quit
	case "tab", "left", "right":
		r.conflFocus = 1 - r.conflFocus
		r.conflMember = 0
	case "up", "k":
		if r.conflFocus == 0 {
			r.conflCursor = max(0, r.conflCursor-1)
			r.conflMember = 0
		} else {
			r.conflMember = max(0, r.conflMember-1)
		}
	case "down", "j":
		if r.conflFocus == 0 {
			r.conflCursor = min(max(n-1, 0), r.conflCursor+1)
			r.conflMember = 0
		} else {
			r.conflMember = min(max(memberCount()-1, 0), r.conflMember+1)
		}
	case "pgup":
		if r.conflFocus == 0 {
			r.conflCursor = max(0, r.conflCursor-10)
			r.conflMember = 0
		} else {
			r.conflMember = max(0, r.conflMember-10)
		}
	case "pgdown":
		if r.conflFocus == 0 {
			r.conflCursor = min(max(n-1, 0), r.conflCursor+10)
			r.conflMember = 0
		} else {
			r.conflMember = min(max(memberCount()-1, 0), r.conflMember+10)
		}
	case "home":
		if r.conflFocus == 0 {
			r.conflCursor = 0
		}
		r.conflMember = 0
	case "end":
		if r.conflFocus == 0 {
			r.conflCursor = max(0, n-1)
		} else {
			r.conflMember = max(0, memberCount()-1)
		}
		r.conflMember = min(r.conflMember, max(0, memberCount()-1))
	case "enter":
		if r.conflFocus == 0 {
			if n > 0 {
				r.conflFocus = 1
				r.conflMember = 0
			}
			return r, nil
		}
		if r.conflCursor >= n || memberCount() == 0 {
			return r, nil
		}
		p := focused().members[min(r.conflMember, memberCount()-1)]
		if r.svc == nil {
			return r, ShowToast(Toast{Level: "bad", Text: "Open unavailable: no service in this session"})
		}
		cmd, err := openCmd(r.svc, p.Slug, "", r.cfg.Editors.GUI)
		if err != nil {
			return r, ShowToast(Toast{Level: "bad", Text: "could not open " + p.Slug + ": " + err.Error()})
		}
		return r, cmd
	case "n":
		r.conflTI = textinput.New()
		r.conflTI.Prompt = "NEW CONFLUENCE: "
		r.conflTI.Width = 32
		focus := r.conflTI.Focus()
		r.conflTIMode = "new"
		return r, focus
	case "r":
		if n == 0 {
			return r, nil
		}
		row := focused()
		r.conflTI = textinput.New()
		r.conflTI.Prompt = "RENAME: "
		r.conflTI.SetValue(row.c.Name)
		r.conflTI.Width = 32
		focus := r.conflTI.Focus()
		r.conflTIMode, r.conflTIQ = "rename", row.c.ID
		return r, focus
	case "x":
		if n == 0 || r.svc == nil {
			return r, nil
		}
		row := focused()
		svc, id, name := r.svc, row.c.ID, row.c.Name
		return r, Confirm("Remove confluence "+name+"?", func() tea.Cmd {
			return conflDeleteCmd(svc, id, name)
		})
	case "e":
		if n == 0 {
			return r, ShowToast(Toast{Level: "warn", Text: "no confluence yet - press n to create one"})
		}
		if r.svc == nil {
			return r, ShowToast(Toast{Level: "bad", Text: "membership editor unavailable: no service in this session"})
		}
		return r, conflEditForConfluenceCmd(r.svc, focused().c.ID)
	}
	return r, nil
}

// confluenceView renders the browser (spec 3.7): the confluence list
// with member counts and a member-name preview on top, the focused
// confluence's members below.
func (r Root) confluenceView() string {
	cut := func(s string) string {
		if r.width > 0 {
			return ansi.Truncate(s, max(10, r.width-4), "…")
		}
		return s
	}
	var b strings.Builder
	b.WriteString(r.styleTitle.Render("CONFLUENCES"))
	if len(r.conflRows) == 0 {
		b.WriteString("\n\n" + r.styleMuted.Render("No confluences yet - press n to create one."))
		b.WriteString("\n" + r.styleMuted.Render("esc back"))
		return b.String()
	}
	ci := min(r.conflCursor, len(r.conflRows)-1)
	row := r.conflRows[ci]
	b.WriteString("\n" + r.styleMuted.Render(fmt.Sprintf("%d/%d", ci+1, len(r.conflRows))))
	for _, cr := range r.conflRows {
		noun := "projects"
		if cr.c.Members == 1 {
			noun = "project"
		}
		preview := ""
		if len(cr.members) > 0 {
			var slugs []string
			for _, mem := range cr.members[:min(3, len(cr.members))] {
				slugs = append(slugs, mem.Slug)
			}
			preview = "  " + strings.Join(slugs, ", ")
		}
		content := cut(fmt.Sprintf("%s  %d %s%s", cr.c.Name, cr.c.Members, noun, preview))
		if cr.c.Name == row.c.Name && r.conflFocus == 0 {
			b.WriteString("\n" + r.styleTitle.Render("> ") + content)
		} else {
			b.WriteString("\n  " + content)
		}
	}

	b.WriteString("\n\n" + r.styleMuted.Render(cut("MEMBERS - "+row.c.Name)))
	if len(row.members) == 0 {
		b.WriteString("\n" + r.styleMuted.Render("no projects in this confluence yet - press e to add"))
	}
	for j, mem := range row.members {
		content := cut(fmt.Sprintf("%s  %s  %s", mem.Name, mem.FlowStage, fallback(mem.Language, "—")))
		if j == r.conflMember && r.conflFocus == 1 {
			b.WriteString("\n" + r.styleTitle.Render("> ") + content)
		} else {
			b.WriteString("\n  " + content)
		}
	}

	b.WriteString("\n\n")
	if r.conflTIMode != "" {
		b.WriteString(r.conflTI.View())
		b.WriteString("\n" + r.styleMuted.Render("enter save  esc cancel"))
	} else {
		b.WriteString(r.styleMuted.Render(cut("[tab] switch  [n] new  [r] rename  [x] remove  [e] membership  [enter] open  [esc] back")))
	}
	return b.String()
}
