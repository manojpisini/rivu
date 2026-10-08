package tui

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/style"
)

// The Settings screen (spec 3.8): a section list whose summary line
// mirrors the spec's drawing, plus the config.toml fields the selected
// section owns. Everything is derived from r.cfg on every frame, so an
// edit shows up without any cache to invalidate. Editing is typed per
// field (P5.08): text/ints/lists/pairs go through the input line with
// inline validation, bools toggle, About is read-only. ctrl+s writes
// through config.Save (atomic temp+rename), r restores the section's
// defaults in memory until the next save.

// fieldKind says how a field is edited and validated.
type fieldKind int

const (
	fieldText  fieldKind = iota // non-empty single value
	fieldInt                    // whole number with a minimum bound
	fieldBool                   // true/false, toggled by enter
	fieldList                   // comma-separated strings
	fieldPairs                  // comma-separated k=v pairs
	fieldInfo                   // build info, read-only (About)
)

// settingsField is one config.toml key: how to read it for display and
// how to write it back with validation (spec line 796: one-to-one with
// the file a person could hand-edit).
type settingsField struct {
	key  string
	kind fieldKind
	get  func(config.Config) string
	set  func(*config.Config, string) error
}

// settingsSection is one Settings row: name, one-line summary (spec
// 3.8), its fields, and how r restores the section to defaults.
type settingsSection struct {
	name, label, value string
	fields             []settingsField
	reset              func(*config.Config)
}

func sval(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func slist(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ", ")
}

// smap renders a string map as sorted "k=v" pairs so the row is
// deterministic (map iteration order is not).
func smap(m map[string]string) string {
	if len(m) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// sweights renders the health weights the same way, with counts.
func sweights(m map[string]int) string {
	if len(m) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%s=%d", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// setText validates a single-line value and stores it.
func setText(f func(*config.Config, string)) func(*config.Config, string) error {
	return func(c *config.Config, raw string) error {
		v := strings.TrimSpace(raw)
		if v == "" {
			return fmt.Errorf("must not be empty")
		}
		f(c, v)
		return nil
	}
}

// setInt parses a whole number with a minimum and stores it.
func setInt(min int, f func(*config.Config, int)) func(*config.Config, string) error {
	return func(c *config.Config, raw string) error {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < min {
			return fmt.Errorf("must be a whole number >= %d", min)
		}
		f(c, n)
		return nil
	}
}

// setBool stores a "true"/"false" toggle result.
func setBool(f func(*config.Config, bool)) func(*config.Config, string) error {
	return func(c *config.Config, raw string) error {
		v, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("must be true or false")
		}
		f(c, v)
		return nil
	}
}

// splitList turns comma-separated input into a clean slice (empty
// input clears the list).
func splitList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parsePairs parses "k=v, k=v" into a string map with non-empty sides.
func parsePairs(raw string) (map[string]string, error) {
	m := map[string]string{}
	for _, p := range splitList(raw) {
		k, v, ok := strings.Cut(p, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" || v == "" {
			return nil, fmt.Errorf("use k=v pairs like go=code")
		}
		m[k] = v
	}
	return m, nil
}

// parseWeights parses the health map and enforces the sum rule
// (config.Validate would reject the file otherwise).
func parseWeights(raw string) (map[string]int, error) {
	m := map[string]int{}
	sum := 0
	for _, p := range splitList(raw) {
		k, v, ok := strings.Cut(p, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		n, err := strconv.Atoi(v)
		if !ok || k == "" || err != nil {
			return nil, fmt.Errorf("use k=v pairs with numbers, like readme=20")
		}
		m[k] = n
		sum += n
	}
	if len(m) > 0 && sum != 100 {
		return nil, fmt.Errorf("health.weights sums to %d, must be 100", sum)
	}
	return m, nil
}

// setTextList stores a comma-separated list.
func setTextList(f func(*config.Config, []string)) func(*config.Config, string) error {
	return func(c *config.Config, raw string) error {
		f(c, splitList(raw))
		return nil
	}
}

// aboutVersion is the About section's version line. ldflags only reach
// main, so fall back to what the toolchain embedded — the same trick
// cli.buildInfo uses for the CLI.
func aboutVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi.Main.Version == "" || bi.Main.Version == "(devel)" {
		return "dev"
	}
	return bi.Main.Version
}

// settingsSections builds the 11 spec 3.8 sections from config.
func settingsSections(cfg config.Config) []settingsSection {
	weighting := "custom"
	if len(cfg.Health.Weights) == 0 {
		weighting = "default"
	}
	bridge := "disabled"
	if cfg.Bridge.Enabled {
		bridge = "enabled"
	}
	return []settingsSection{
		{
			name: "General", label: "Workspace root", value: sval(cfg.Workspace.Root),
			reset: func(c *config.Config) { c.Workspace = config.Default().Workspace },
			fields: []settingsField{
				{key: "workspace.root", kind: fieldText,
					get: func(c config.Config) string { return c.Workspace.Root },
					set: setText(func(c *config.Config, v string) { c.Workspace.Root = v })},
				{key: "workspace.secondary_roots", kind: fieldList,
					get: func(c config.Config) string { return slist(c.Workspace.SecondaryRoots) },
					set: setTextList(func(c *config.Config, v []string) { c.Workspace.SecondaryRoots = v })},
				{key: "workspace.auto_rescan_on_launch", kind: fieldBool,
					get: func(c config.Config) string { return strconv.FormatBool(c.Workspace.AutoRescan) },
					set: setBool(func(c *config.Config, v bool) { c.Workspace.AutoRescan = v })},
			},
		},
		{
			name: "Editors", label: "Default editor", value: sval(cfg.Editors.Default),
			reset: func(c *config.Config) { c.Editors = config.Default().Editors },
			fields: []settingsField{
				{key: "editors.default", kind: fieldText,
					get: func(c config.Config) string { return c.Editors.Default },
					set: setText(func(c *config.Config, v string) { c.Editors.Default = v })},
				{key: "editors.gui", kind: fieldList,
					get: func(c config.Config) string { return slist(c.Editors.GUI) },
					set: setTextList(func(c *config.Config, v []string) { c.Editors.GUI = v })},
				{key: "editors.per_language", kind: fieldPairs,
					get: func(c config.Config) string { return smap(c.Editors.PerLanguage) },
					set: func(c *config.Config, raw string) error {
						m, err := parsePairs(raw)
						if err != nil {
							return err
						}
						c.Editors.PerLanguage = m
						return nil
					}},
			},
		},
		{
			name: "Flow", label: "Channel mapping", value: "00_Source … 90_Delta",
			reset: func(c *config.Config) { c.Flow = config.Default().Flow },
			fields: []settingsField{
				{key: "flow.stale_threshold_days", kind: fieldInt,
					get: func(c config.Config) string { return strconv.Itoa(c.Flow.StaleThresholdDays) },
					set: setInt(1, func(c *config.Config, v int) { c.Flow.StaleThresholdDays = v })},
				{key: "flow.source_sla_days", kind: fieldInt,
					get: func(c config.Config) string { return strconv.Itoa(c.Flow.SourceSLADays) },
					set: setInt(1, func(c *config.Config, v int) { c.Flow.SourceSLADays = v })},
			},
		},
		{
			name: "Bridge", label: "Bridge integration", value: bridge,
			reset: func(c *config.Config) { c.Bridge = config.Default().Bridge },
			fields: []settingsField{
				{key: "bridge.enabled", kind: fieldBool,
					get: func(c config.Config) string { return strconv.FormatBool(c.Bridge.Enabled) },
					set: setBool(func(c *config.Config, v bool) { c.Bridge.Enabled = v })},
				{key: "bridge.owns_git_init", kind: fieldBool,
					get: func(c config.Config) string { return strconv.FormatBool(c.Bridge.OwnsGitInit) },
					set: setBool(func(c *config.Config, v bool) { c.Bridge.OwnsGitInit = v })},
				{key: "bridge.scaffold_marker", kind: fieldText,
					get: func(c config.Config) string { return c.Bridge.ScaffoldMark },
					set: func(c *config.Config, raw string) error {
						c.Bridge.ScaffoldMark = strings.TrimSpace(raw) // empty disables the marker
						return nil
					}},
			},
		},
		{
			name: "Templates", label: "Default template", value: sval(cfg.Templates.Default),
			reset: func(c *config.Config) { c.Templates = config.Default().Templates },
			fields: []settingsField{
				{key: "templates.default", kind: fieldText,
					get: func(c config.Config) string { return c.Templates.Default },
					set: setText(func(c *config.Config, v string) { c.Templates.Default = v })},
			},
		},
		{
			name: "Health", label: "Score weighting", value: weighting,
			reset: func(c *config.Config) { c.Health = config.Default().Health },
			fields: []settingsField{
				{key: "health.weights", kind: fieldPairs,
					get: func(c config.Config) string { return sweights(c.Health.Weights) },
					set: func(c *config.Config, raw string) error {
						m, err := parseWeights(raw)
						if err != nil {
							return err
						}
						c.Health.Weights = m
						return nil
					}},
			},
		},
		{
			name: "Scanner", label: "Ignore list", value: slist(cfg.Scanner.Ignore),
			reset: func(c *config.Config) { c.Scanner = config.Default().Scanner },
			fields: []settingsField{
				{key: "scanner.ignore", kind: fieldList,
					get: func(c config.Config) string { return slist(c.Scanner.Ignore) },
					set: setTextList(func(c *config.Config, v []string) { c.Scanner.Ignore = v })},
				{key: "scanner.max_depth", kind: fieldInt,
					get: func(c config.Config) string { return strconv.Itoa(c.Scanner.MaxDepth) },
					set: setInt(1, func(c *config.Config, v int) { c.Scanner.MaxDepth = v })},
			},
		},
		{
			name: "Appearance", label: "Theme", value: sval(cfg.Appearance.Theme),
			reset: func(c *config.Config) { c.Appearance = config.Default().Appearance },
			fields: []settingsField{
				{key: "appearance.theme", kind: fieldText,
					get: func(c config.Config) string { return c.Appearance.Theme },
					set: func(c *config.Config, raw string) error {
						v := strings.TrimSpace(raw)
						if v == "" {
							return fmt.Errorf("must not be empty")
						}
						if _, err := style.ByName(v); err != nil {
							return fmt.Errorf("unknown theme")
						}
						c.Appearance.Theme = v
						return nil
					}},
				{key: "appearance.density", kind: fieldText,
					get: func(c config.Config) string { return c.Appearance.Density },
					set: setText(func(c *config.Config, v string) { c.Appearance.Density = v })},
			},
		},
		{
			name: "Keybindings", label: "Profile", value: sval(cfg.Keybindings.Profile),
			reset: func(c *config.Config) { c.Keybindings = config.Default().Keybindings },
			fields: []settingsField{
				{key: "keybindings.profile", kind: fieldText,
					get: func(c config.Config) string { return c.Keybindings.Profile },
					set: setText(func(c *config.Config, v string) { c.Keybindings.Profile = v })},
			},
		},
		{
			name: "Data", label: "DB location", value: sval(cfg.Data.DBPath),
			reset: func(c *config.Config) { c.Data = config.Default().Data },
			fields: []settingsField{
				{key: "data.db_path", kind: fieldText,
					get: func(c config.Config) string { return c.Data.DBPath },
					set: setText(func(c *config.Config, v string) { c.Data.DBPath = v })},
				{key: "data.snapshot_retention_days", kind: fieldInt,
					get: func(c config.Config) string { return strconv.Itoa(c.Data.SnapshotRetentionDays) },
					set: setInt(0, func(c *config.Config, v int) { c.Data.SnapshotRetentionDays = v })},
			},
		},
		{
			name: "About", label: "Version", value: aboutVersion(),
			reset: nil, // build info has no defaults to restore
			fields: []settingsField{
				{key: "version", kind: fieldInfo,
					get: func(config.Config) string { return aboutVersion() }},
				{key: "go", kind: fieldInfo,
					get: func(config.Config) string { return runtime.Version() }},
				{key: "platform", kind: fieldInfo,
					get: func(config.Config) string { return runtime.GOOS + "/" + runtime.GOARCH }},
			},
		},
	}
}

// settingsKeys drives the screen (spec 3.8): section focus, field
// focus, the input line for a field being edited, r for reset and
// ctrl+s for the atomic save. q and esc walk back the same way.
func (r Root) settingsKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	secs := settingsSections(r.cfg)
	ci := min(max(r.setgCursor, 0), len(secs)-1)
	sec := secs[ci]
	fi := min(max(r.setgField, 0), len(sec.fields)-1)
	f := sec.fields[fi]

	if r.setgTIOn {
		switch x.String() {
		case "ctrl+c":
			return r, tea.Quit
		case "esc":
			r.setgTIOn, r.setgErr = false, ""
			return r, nil
		case "enter":
			if err := f.set(&r.cfg, r.setgTI.Value()); err != nil {
				// Inline validation: keep the input open with the
				// bad value so it can be fixed or cancelled.
				r.setgErr = f.key + ": " + err.Error()
				return r, nil
			}
			r.setgTIOn, r.setgErr = false, ""
			return r, nil
		}
		in, cmd := r.setgTI.Update(x)
		r.setgTI = in
		return r, cmd
	}

	secNext := func(d int) {
		r.setgCursor = min(max(len(secs)-1, 0), max(0, ci+d))
		r.setgField, r.setgErr = 0, ""
	}
	switch x.String() {
	case "ctrl+c":
		return r, tea.Quit
	case "esc", "q":
		if r.setgFocus == 1 {
			r.setgFocus, r.setgField, r.setgErr = 0, 0, ""
			return r, nil
		}
		r.screen = ScreenDashboard
		return r, nil
	case "tab":
		secNext(1)
	case "up", "k":
		if r.setgFocus == 0 {
			r.setgCursor = max(0, ci-1)
			r.setgField, r.setgErr = 0, ""
		} else {
			r.setgField = max(0, fi-1)
			r.setgErr = ""
		}
	case "down", "j":
		if r.setgFocus == 0 {
			secNext(1)
		} else {
			r.setgField = min(len(sec.fields)-1, fi+1)
			r.setgErr = ""
		}
	case "pgup":
		if r.setgFocus == 0 {
			r.setgCursor = max(0, ci-10)
			r.setgField, r.setgErr = 0, ""
		} else {
			r.setgField = max(0, fi-10)
			r.setgErr = ""
		}
	case "pgdown":
		if r.setgFocus == 0 {
			secNext(10)
		} else {
			r.setgField = min(len(sec.fields)-1, fi+10)
			r.setgErr = ""
		}
	case "home":
		if r.setgFocus == 0 {
			r.setgCursor, r.setgField, r.setgErr = 0, 0, ""
		} else {
			r.setgField, r.setgErr = 0, ""
		}
	case "end":
		if r.setgFocus == 0 {
			r.setgCursor, r.setgField, r.setgErr = len(secs)-1, 0, ""
		} else {
			r.setgField, r.setgErr = len(sec.fields)-1, ""
		}
	case "enter":
		if r.setgFocus == 0 {
			r.setgFocus, r.setgField, r.setgErr = 1, 0, ""
			return r, nil
		}
		switch f.kind {
		case fieldInfo:
			return r, nil
		case fieldBool:
			v, err := strconv.ParseBool(f.get(r.cfg))
			if err != nil {
				return r, ShowToast(Toast{Level: "bad", Text: "could not read " + f.key + ": " + err.Error()})
			}
			if err := f.set(&r.cfg, strconv.FormatBool(!v)); err != nil {
				return r, ShowToast(Toast{Level: "bad", Text: f.key + ": " + err.Error()})
			}
			return r, nil
		}
		r.setgTI = textinput.New()
		r.setgTI.Prompt = f.key + ": "
		r.setgTI.Width = 48
		r.setgTI.SetValue(f.get(r.cfg))
		r.setgErr = ""
		focus := r.setgTI.Focus()
		r.setgTIOn = true
		return r, focus
	case "r":
		if sec.reset == nil {
			return r, nil
		}
		sec.reset(&r.cfg)
		r.setgErr = ""
		return r, ShowToast(Toast{Text: sec.name + " reset to defaults - ctrl+s to save"})
	case "ctrl+s":
		if err := config.Validate(r.cfg); err != nil {
			first, _, _ := strings.Cut(err.Error(), "\n")
			return r, ShowToast(Toast{Level: "bad", Text: first})
		}
		if err := config.Save(r.cfg); err != nil {
			return r, ShowToast(Toast{Level: "bad", Text: "could not save settings: " + err.Error()})
		}
		r.dashboard.cfg = r.cfg
		p, _ := config.Path()
		return r, ShowToast(Toast{Level: "good", Text: "settings saved to " + p})
	}
	return r, nil
}

// settingsView renders the section list with the spec 3.8 summary
// columns, then the selected section's config.toml fields (spec line
// 796: every field maps one-to-one onto a key). The input line takes
// the footer while a field is being edited, with the inline error
// under it.
func (r Root) settingsView() string {
	cut := func(s string) string {
		if r.width > 0 {
			return ansi.Truncate(s, max(10, r.width-4), "…")
		}
		return s
	}
	secs := settingsSections(r.cfg)
	ci := min(max(r.setgCursor, 0), len(secs)-1)
	sel := secs[ci]
	fi := min(max(r.setgField, 0), len(sel.fields)-1)
	var b strings.Builder
	b.WriteString(r.styleTitle.Render("SETTINGS"))
	for i, s := range secs {
		row := padCell(s.name, 13) + "  " + padCell(s.label, 20) + "  " + s.value
		if i == ci {
			b.WriteString("\n" + r.styleTitle.Render("> ") + cut(row))
		} else {
			b.WriteString("\n  " + cut(row))
		}
	}
	b.WriteString("\n\n" + r.styleMuted.Render(cut("FIELDS - "+strings.ToUpper(sel.name))))
	for i, f := range sel.fields {
		row := cut(padCell(f.key, 32) + f.get(r.cfg))
		if i == fi && r.setgFocus == 1 {
			b.WriteString("\n" + r.styleTitle.Render("> ") + row)
		} else {
			b.WriteString("\n  " + row)
		}
	}
	b.WriteString("\n\n")
	if r.setgTIOn {
		b.WriteString(r.setgTI.View())
		b.WriteString("\n" + r.styleMuted.Render("enter apply  esc cancel"))
		if r.setgErr != "" {
			b.WriteString("\n" + r.styleErr.Render(cut(r.setgErr)))
		}
		return b.String()
	}
	if r.setgErr != "" {
		b.WriteString(r.styleErr.Render(cut(r.setgErr)) + "\n\n")
	}
	if r.setgFocus == 1 {
		b.WriteString(r.styleMuted.Render(cut("[enter] edit  [j/k] field  [tab] section  [ctrl+s] save  [r] reset  [esc] sections")))
	} else {
		b.WriteString(r.styleMuted.Render(cut("[tab] section  [j/k] move  [enter] fields  [ctrl+s] save  [r] reset  [esc] back")))
	}
	return b.String()
}
