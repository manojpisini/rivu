package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/style"
)

// palVisible is how many palette rows fit in the overlay (P5.12).
const palVisible = 10

// paletteAction is one command the `:` palette can run: either a
// synthetic dashboard key (run == nil, the key path shares every guard
// with the real binding) or a custom run for parameterised commands
// like "doctor all" and "flow to active".
type paletteAction struct {
	label string
	key   string
	run   func(r *Root) tea.Cmd
}

// flowTo plans a move of the current selection to stage, mirroring the
// f picker's guards (P5.12).
func flowTo(stage string) func(*Root) tea.Cmd {
	return func(r *Root) tea.Cmd {
		if r.svc == nil {
			r.dashboard.Status = "Flow unavailable: no service in this session"
			return nil
		}
		if _, ok := r.dashboard.selectedProject(); !ok {
			r.dashboard.Status = "Select a project to flow"
			return nil
		}
		nm, cmd := r.dashboard.flowToStage(stage)
		r.dashboard = nm.(Model)
		return cmd
	}
}

// paletteActions is the command list behind the `:` palette — every
// command that changes screen or does work, fuzzy-matched by label.
// Theme switching is deliberately absent: the package-level styles
// would need a re-theme pass (Settings owns persistence instead).
func paletteActions() []paletteAction {
	return []paletteAction{
		{label: "search projects", key: "/"},
		{label: "clear filter", key: "esc"},
		{label: "open project", key: "enter"},
		{label: "project detail", key: "d"},
		{label: "doctor selected", key: "h"},
		{label: "doctor all", run: func(r *Root) tea.Cmd {
			if r.svc == nil {
				r.dashboard.Status = "Health checks unavailable: no service in this session"
				return nil
			}
			r.dashboard.Status = "Running health checks…"
			return doctorCmd(r.svc, "")
		}},
		{label: "map report", key: "a"},
		{label: "flow picker", key: "f"},
		{label: "flow to source", run: flowTo("source")},
		{label: "flow to active", run: flowTo("active")},
		{label: "flow to maintenance", run: flowTo("maintenance")},
		{label: "flow to research", run: flowTo("research")},
		{label: "flow to delta", run: flowTo("delta")},
		{label: "delta project", key: "A"},
		{label: "project actions", key: "x"},
		{label: "untriaged filter", key: "u"},
		{label: "source new project", key: "n"},
		{label: "refresh (scan)", key: "r"},
		{label: "copy path", key: "y"},
		{label: "reveal folder", key: "o"},
		{label: "master dashboard", key: "g"},
		{label: "stats", key: "s"},
		{label: "settings", key: "S"},
		{label: "logs", key: "L"},
		{label: "confluences", key: "c"},
		{label: "help", key: "?"},
		{label: "quit", key: "q"},
	}
}

// paletteLabels extracts the fuzzy targets (kept in sync with the
// actions by construction).
func paletteLabels() []string {
	acts := paletteActions()
	out := make([]string, len(acts))
	for i, a := range acts {
		out[i] = a.label
	}
	return out
}

// fuzzyRanks returns the indices of labels matching query, best score
// first; an empty query keeps the input order. Subsequence matching
// with word-start and consecutive bonuses, gap penalty — enough for a
// 27-item list without a dependency.
func fuzzyRanks(query string, labels []string) []int {
	q := strings.ToLower(strings.TrimSpace(query))
	type hit struct{ idx, score int }
	hits := make([]hit, 0, len(labels))
	for i, l := range labels {
		if q == "" {
			hits = append(hits, hit{i, 0})
			continue
		}
		if s, ok := fuzzyScore(q, strings.ToLower(l)); ok {
			hits = append(hits, hit{i, s})
		}
	}
	for i := 1; i < len(hits); i++ { // insertion sort: stable, tiny n
		for j := i; j > 0 && hits[j].score > hits[j-1].score; j-- {
			hits[j], hits[j-1] = hits[j-1], hits[j]
		}
	}
	out := make([]int, len(hits))
	for i, h := range hits {
		out[i] = h.idx
	}
	return out
}

// fuzzyScore scores s against the lower-case query q; ok is false when
// q is not a subsequence of s.
func fuzzyScore(q, s string) (int, bool) {
	score := 0
	si := 0
	for i := 0; i < len(q); i++ {
		found := -1
		for j := si; j < len(s); j++ {
			if s[j] == q[i] {
				found = j
				break
			}
		}
		if found < 0 {
			return 0, false
		}
		if found == si && si > 0 {
			score += 5 // consecutive run
		}
		if found == 0 || s[found-1] == ' ' || s[found-1] == '-' {
			score += 8 // word start
		}
		score -= found - si // gap penalty
		si = found + 1
	}
	return score, true
}

// paletteCanOpen reports whether `:` starts the palette: never while a
// text input of any screen owns typing (search, Settings field, Source
// wizard, Confluence prompt, Stats export picker).
func (r Root) paletteCanOpen() bool {
	if r.screen == ScreenSource {
		return false
	}
	return !r.dashboard.search.Focused() && !r.setgTIOn && !r.statsExport &&
		r.conflEdit == nil && r.conflTIMode == ""
}

// openPalette shows the overlay with an empty query.
func (r Root) openPalette() (tea.Model, tea.Cmd) {
	r.palOn = true
	r.palTI.SetValue("")
	r.palTI.Focus()
	r.palCursor = 0
	return r, nil
}

// paletteKeys owns the keyboard while the overlay is open: enter runs
// the selection, esc cancels, up/down move, everything else types.
func (r Root) paletteKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch x.String() {
	case "esc":
		r.palOn = false
		return r, nil
	case "ctrl+c":
		return r, tea.Quit
	case "enter":
		return r.runPalette()
	case "up", "ctrl+k":
		if r.palCursor > 0 {
			r.palCursor--
		}
		return r, nil
	case "down", "ctrl+j":
		if r.palCursor < len(r.palRows())-1 {
			r.palCursor++
		}
		return r, nil
	}
	in, cmd := r.palTI.Update(x)
	r.palTI = in
	r.palCursor = 0
	return r, cmd
}

// palRows are the labels matching the current query, ranked.
func (r Root) palRows() []int {
	return fuzzyRanks(r.palTI.Value(), paletteLabels())
}

// runPalette executes the highlighted action: overlays the dashboard
// was showing are dismissed (the command supersedes them) and the
// dashboard key path runs with every guard intact.
func (r Root) runPalette() (tea.Model, tea.Cmd) {
	rows := r.palRows()
	if len(rows) == 0 || r.palCursor >= len(rows) {
		return r, nil
	}
	a := paletteActions()[rows[r.palCursor]]
	r.palOn = false
	r.palTI.SetValue("")
	r.dashboard.DetailFull = false
	r.dashboard.FixKey = ""
	r.dashboard.helpOpen = false
	r.dashboard.flowPick = false
	r.dashboard.actionMenu = false
	r.screen = ScreenDashboard
	if a.run != nil {
		return r, a.run(&r)
	}
	return r.forward(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(a.key)})
}

// paletteView renders the overlay: query line, ranked matches with
// their keys, and a hint row.
func (r Root) paletteView() string {
	rows := r.palRows()
	var b strings.Builder
	b.WriteString(r.styleTitle.Render("COMMAND"))
	b.WriteString("\n" + r.palTI.View())
	acts := paletteActions()
	switch {
	case len(rows) == 0:
		b.WriteString("\n\n" + r.styleMuted.Render("no match"))
	default:
		start := max(0, min(r.palCursor-palVisible/2, len(rows)-palVisible))
		end := min(len(rows), start+palVisible)
		for i := start; i < end; i++ {
			a := acts[rows[i]]
			pad := strings.Repeat(" ", max(1, 24-lipgloss.Width(a.label)))
			if i == r.palCursor {
				b.WriteString("\n" + r.styleTitle.Render("> "+a.label) + pad + r.styleMuted.Render(a.key))
			} else {
				b.WriteString("\n  " + a.label + pad + r.styleMuted.Render(a.key))
			}
		}
		if len(rows) > palVisible {
			b.WriteString("\n" + r.styleMuted.Render(fmt.Sprintf("%d matches", len(rows))))
		}
	}
	b.WriteString("\n" + r.styleMuted.Render("enter run · ↑↓ move · esc cancel"))
	return r.styleConfirm.Render(b.String())
}

// newPaletteInput builds the overlay's text input once, from the theme.
func newPaletteInput(theme style.Theme) textinput.Model {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Placeholder = "type a command…"
	ti.CharLimit = 64
	ti.PromptStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	return ti
}

// paletteBinds reports whether the palette key matched — shared by the
// root router so help and handling cannot drift.
func paletteBinds(x tea.KeyMsg) bool { return key.Matches(x, keys.Palette) }
