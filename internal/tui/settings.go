package tui

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/manojpisini/rivu/internal/config"
)

// The Settings screen (spec 3.8): a section list whose summary line
// mirrors the spec's drawing, plus the config.toml fields the selected
// section owns. Everything is derived from r.cfg on every frame, so a
// save (P5.08) shows up without any cache to invalidate. This screen
// reads config only — it needs no service.

// settingsSection is one Settings row: the section name, its one-line
// summary (spec 3.8) and the config.toml keys it owns, each mapped
// one-to-one so Settings can never drift from a hand-edited file.
type settingsSection struct {
	name, label, value string
	fields             [][2]string
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

// sweights renders the health weights map the same way, with counts.
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

// settingsSections builds the11 spec 3.8 sections from config.
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
		{name: "General", label: "Workspace root", value: sval(cfg.Workspace.Root), fields: [][2]string{
			{"workspace.root", sval(cfg.Workspace.Root)},
			{"workspace.secondary_roots", slist(cfg.Workspace.SecondaryRoots)},
			{"workspace.auto_rescan_on_launch", strconv.FormatBool(cfg.Workspace.AutoRescan)},
		}},
		{name: "Editors", label: "Default editor", value: sval(cfg.Editors.Default), fields: [][2]string{
			{"editors.default", sval(cfg.Editors.Default)},
			{"editors.gui", slist(cfg.Editors.GUI)},
			{"editors.per_language", smap(cfg.Editors.PerLanguage)},
		}},
		{name: "Flow", label: "Channel mapping", value: "00_Source … 90_Delta", fields: [][2]string{
			{"flow.stale_threshold_days", strconv.Itoa(cfg.Flow.StaleThresholdDays)},
			{"flow.source_sla_days", strconv.Itoa(cfg.Flow.SourceSLADays)},
		}},
		{name: "Bridge", label: "Bridge integration", value: bridge, fields: [][2]string{
			{"bridge.enabled", strconv.FormatBool(cfg.Bridge.Enabled)},
			{"bridge.owns_git_init", strconv.FormatBool(cfg.Bridge.OwnsGitInit)},
			{"bridge.scaffold_marker", sval(cfg.Bridge.ScaffoldMark)},
		}},
		{name: "Templates", label: "Default template", value: sval(cfg.Templates.Default), fields: [][2]string{
			{"templates.default", sval(cfg.Templates.Default)},
		}},
		{name: "Health", label: "Score weighting", value: weighting, fields: [][2]string{
			{"health.weights", sweights(cfg.Health.Weights)},
		}},
		{name: "Scanner", label: "Ignore list", value: slist(cfg.Scanner.Ignore), fields: [][2]string{
			{"scanner.ignore", slist(cfg.Scanner.Ignore)},
			{"scanner.max_depth", strconv.Itoa(cfg.Scanner.MaxDepth)},
		}},
		{name: "Appearance", label: "Theme", value: sval(cfg.Appearance.Theme), fields: [][2]string{
			{"appearance.theme", sval(cfg.Appearance.Theme)},
			{"appearance.density", sval(cfg.Appearance.Density)},
		}},
		{name: "Keybindings", label: "Profile", value: sval(cfg.Keybindings.Profile), fields: [][2]string{
			{"keybindings.profile", sval(cfg.Keybindings.Profile)},
		}},
		{name: "Data", label: "DB location", value: sval(cfg.Data.DBPath), fields: [][2]string{
			{"data.db_path", sval(cfg.Data.DBPath)},
			{"data.snapshot_retention_days", strconv.Itoa(cfg.Data.SnapshotRetentionDays)},
		}},
		{name: "About", label: "Version", value: aboutVersion(), fields: [][2]string{
			{"version", aboutVersion()},
			{"go", runtime.Version()},
			{"platform", runtime.GOOS + "/" + runtime.GOARCH},
		}},
	}
}

// settingsKeys drives the section list (spec 3.8). It is read-only
// until P5.08 wires enter/ctrl+s/r for field editing.
func (r Root) settingsKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(settingsSections(r.cfg))
	last := max(n-1, 0)
	switch x.String() {
	case "ctrl+c":
		return r, tea.Quit
	case "esc", "q":
		r.screen = ScreenDashboard
		return r, nil
	case "up", "k":
		r.setgCursor = max(0, r.setgCursor-1)
	case "down", "j", "tab":
		r.setgCursor = min(last, r.setgCursor+1)
	case "pgup":
		r.setgCursor = max(0, r.setgCursor-10)
	case "pgdown":
		r.setgCursor = min(last, r.setgCursor+10)
	case "home":
		r.setgCursor = 0
	case "end":
		r.setgCursor = last
	}
	return r, nil
}

// settingsView renders the section list with the spec 3.8 summary
// columns, then the selected section's config.toml fields (spec line
// 796: every field maps one-to-one onto a key).
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
	for _, f := range sel.fields {
		b.WriteString("\n  " + cut(padCell(f[0], 32)+f[1]))
	}
	b.WriteString("\n\n" + r.styleMuted.Render(cut("[tab] section  [j/k] move  [esc] back")))
	return b.String()
}
