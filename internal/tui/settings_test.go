package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/config"
)

// testSettingsConfig pins machine-independent values so the Settings
// tests and its golden never leak the host home directory or editor.
func testSettingsConfig() config.Config {
	cfg := config.Default()
	cfg.Workspace.Root = "/ws/demo"
	cfg.Editors.Default = "nvim"
	cfg.Data.DBPath = "/home/u/.rivu/rivu.db"
	return cfg
}

// settingsFixture opens the Settings screen (spec 3.8) from the
// dashboard with `S`. RIVU_HOME is pinned so the defaults and any
// save in these tests stay inside the test (safety rule 8).
func settingsFixture(t *testing.T) Root {
	t.Helper()
	t.Setenv("RIVU_HOME", t.TempDir())
	m, f := scanFixture(t)
	m, cmd := updateC(t, m, runeKey("S"))
	if cmd == nil {
		t.Fatal("S must open Settings")
	}
	r := NewRoot(f, testSettingsConfig())
	r.dashboard = m
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	r, _ = upd(t, r, cmd())
	if r.screen != ScreenSettings {
		t.Fatalf("screen = %v, want settings", r.screen)
	}
	return r
}

// TestSettingsScreenOpensAndRenders: S opens the read-only screen with
// the spec 3.8 section list, General's config fields and the footer;
// esc returns to the dashboard.
func TestSettingsScreenOpensAndRenders(t *testing.T) {
	r := settingsFixture(t)
	v := r.View()
	for _, want := range []string{
		"SETTINGS", "General", "Workspace root", "/ws/demo",
		"Editors", "Default editor", "nvim",
		"Flow", "Channel mapping", "00_Source … 90_Delta",
		"Bridge", "Templates", "Health", "Scanner",
		"Appearance", "Keybindings", "Data", "About",
		"FIELDS - GENERAL", "workspace.root", "workspace.secondary_roots",
		"workspace.auto_rescan_on_launch",
		"[tab] section", "[j/k] move", "[esc] back",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("settings view missing %q in %q", want, v)
		}
	}
	r, _ = upd(t, r, keyEsc())
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after esc", r.screen)
	}
}

// TestSettingsSectionMotion: j/k/tab/pgdn/home/end move the section
// cursor, clamp at both ends and the fields pane follows the selection.
func TestSettingsSectionMotion(t *testing.T) {
	r := settingsFixture(t)
	if r.setgCursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", r.setgCursor)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEnd})
	if r.setgCursor != 10 || !strings.Contains(r.View(), "FIELDS - ABOUT") {
		t.Fatalf("end: cursor = %d, view has FIELDS - ABOUT = %v",
			r.setgCursor, strings.Contains(r.View(), "FIELDS - ABOUT"))
	}
	r, _ = upd(t, r, runeKey("j"))
	if r.setgCursor != 10 {
		t.Fatalf("j past the end = %d, want clamp at 10", r.setgCursor)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyHome})
	if r.setgCursor != 0 {
		t.Fatalf("home = %d, want 0", r.setgCursor)
	}
	r, _ = upd(t, r, runeKey("k"))
	if r.setgCursor != 0 {
		t.Fatalf("k past the start = %d, want clamp at 0", r.setgCursor)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyTab})
	if r.setgCursor != 1 || !strings.Contains(r.View(), "FIELDS - EDITORS") {
		t.Fatalf("tab: cursor = %d, want 1 and FIELDS - EDITORS", r.setgCursor)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyPgDown})
	if r.setgCursor != 10 {
		t.Fatalf("pgdn = %d, want clamp at 10", r.setgCursor)
	}
}

// TestSettingsBackKey: q leaves the screen like esc (global key).
func TestSettingsBackKey(t *testing.T) {
	r := settingsFixture(t)
	r, _ = upd(t, r, runeKey("q"))
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard after q", r.screen)
	}
}

// TestSettingsSectionsMapConfigKeys: the 11 spec 3.8 sections in spec
// order; every field is a config.toml key (About's build info excepted)
// and the summary values come from config.
func TestSettingsSectionsMapConfigKeys(t *testing.T) {
	secs := settingsSections(testSettingsConfig())
	want := []string{
		"General", "Editors", "Flow", "Bridge", "Templates", "Health",
		"Scanner", "Appearance", "Keybindings", "Data", "About",
	}
	if len(secs) != len(want) {
		t.Fatalf("sections = %d, want %d", len(secs), len(want))
	}
	for i, name := range want {
		if secs[i].name != name {
			t.Errorf("section %d = %q, want %q", i, secs[i].name, name)
		}
	}
	if secs[0].value != "/ws/demo" || secs[1].value != "nvim" {
		t.Errorf("summary values = %q / %q, want config-sourced", secs[0].value, secs[1].value)
	}
	if secs[10].value == "" {
		t.Error("About summary has no version")
	}
	for _, s := range secs {
		for _, f := range s.fields {
			if f.key != "version" && f.key != "go" && f.key != "platform" && !strings.Contains(f.key, ".") {
				t.Errorf("%s: field %q is not a config.toml key", s.name, f.key)
			}
		}
	}
}

// TestSettingsKeyBinding: S is bound and stays out of the 8-item
// ShortHelp (FullHelp only, like every screen key added since P4).
func TestSettingsKeyBinding(t *testing.T) {
	if keys.Settings.Help().Key != "S" || keys.Settings.Help().Desc != "settings" {
		t.Fatalf("binding = %+v, want S/settings", keys.Settings.Help())
	}
	if n := len(keys.ShortHelp()); n != 8 {
		t.Fatalf("ShortHelp = %d bindings, want 8", n)
	}
}

// clearInput backspaces whatever the input line holds.
func clearInput(t *testing.T, r Root) Root {
	t.Helper()
	for i := 0; i < 64; i++ {
		r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	return r
}

// configPath is where config.Save writes under the pinned RIVU_HOME.
func configPath(t *testing.T) string {
	t.Helper()
	p, err := config.Path()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	return p
}

// TestSettingsFieldEditApplies: enter moves into the fields pane,
// enter opens the input line, a valid value lands in r.cfg (memory
// only) and esc walks back out.
func TestSettingsFieldEditApplies(t *testing.T) {
	r := settingsFixture(t)
	r, _ = upd(t, r, keyEnter()) // section -> fields
	if r.setgFocus != 1 || r.setgField != 0 {
		t.Fatalf("focus = %d field = %d, want 1/0", r.setgFocus, r.setgField)
	}
	r, _ = upd(t, r, keyEnter()) // open workspace.root
	if !r.setgTIOn {
		t.Fatal("enter must open the input line")
	}
	r = clearInput(t, r)
	r, _ = upd(t, r, runeKey("/ws/new"))
	r, _ = upd(t, r, keyEnter())
	if r.setgTIOn {
		t.Fatal("valid value must close the input line")
	}
	if r.cfg.Workspace.Root != "/ws/new" {
		t.Fatalf("root = %q, want /ws/new", r.cfg.Workspace.Root)
	}
	if r.dashboard.cfg.Workspace.Root == "/ws/new" {
		t.Fatal("dashboard cfg must only follow on save")
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEsc})
	if r.setgFocus != 0 {
		t.Fatalf("esc = focus %d, want back to sections", r.setgFocus)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEsc})
	if r.screen != ScreenDashboard {
		t.Fatalf("screen = %v, want dashboard", r.screen)
	}
}

// TestSettingsInlineValidation: a bad int keeps the input open, shows
// the error under it and leaves the config untouched.
func TestSettingsInlineValidation(t *testing.T) {
	r := settingsFixture(t)
	for i := 0; i < 2; i++ {
		r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyTab}) // General -> Flow
	}
	r, _ = upd(t, r, keyEnter())
	if !strings.Contains(r.View(), "flow.stale_threshold_days") {
		t.Fatalf("Flow fields missing stale_threshold_days in %q", r.View())
	}
	r, _ = upd(t, r, keyEnter())
	if !r.setgTIOn {
		t.Fatal("enter must open the input line")
	}
	r = clearInput(t, r)
	r, _ = upd(t, r, runeKey("abc"))
	r, _ = upd(t, r, keyEnter())
	if !r.setgTIOn {
		t.Fatal("invalid value must keep the input open")
	}
	if !strings.Contains(r.setgErr, "flow.stale_threshold_days") || !strings.Contains(r.setgErr, "whole number") {
		t.Fatalf("setgErr = %q, want inline parse complaint", r.setgErr)
	}
	if v := r.View(); !strings.Contains(v, "flow.stale_threshold_days: must be a whole number") {
		t.Fatalf("view missing inline error in %q", v)
	}
	if r.cfg.Flow.StaleThresholdDays != 45 {
		t.Fatalf("stale threshold = %d, want the original 45", r.cfg.Flow.StaleThresholdDays)
	}
	r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyEsc})
	if r.setgTIOn || r.setgErr != "" {
		t.Fatalf("esc must cancel the edit (on=%v err=%q)", r.setgTIOn, r.setgErr)
	}
}

// TestSettingsBoolToggle: enter on a bool field flips it without an
// input line.
func TestSettingsBoolToggle(t *testing.T) {
	r := settingsFixture(t)
	for i := 0; i < 3; i++ {
		r, _ = upd(t, r, tea.KeyMsg{Type: tea.KeyTab}) // -> Bridge
	}
	r, _ = upd(t, r, keyEnter())
	if r.setgField != 0 {
		t.Fatalf("field = %d, want 0 (bridge.enabled)", r.setgField)
	}
	r, _ = upd(t, r, keyEnter())
	if r.setgTIOn {
		t.Fatal("bool fields must not open the input line")
	}
	if !r.cfg.Bridge.Enabled {
		t.Fatal("enter must toggle bridge.enabled to true")
	}
	v := r.View()
	if !strings.Contains(v, "Bridge integration    enabled") {
		t.Fatalf("summary did not flip to enabled in %q", v)
	}
	r, _ = upd(t, r, keyEnter())
	if r.cfg.Bridge.Enabled {
		t.Fatal("second enter must toggle back to false")
	}
}

// TestSettingsCtrlSSaves: ctrl+s validates, writes config.toml through
// the atomic Save and toasts the path; the dashboard cfg follows.
func TestSettingsCtrlSSaves(t *testing.T) {
	r := settingsFixture(t)
	r, _ = upd(t, r, keyEnter())
	r, _ = upd(t, r, keyEnter()) // edit workspace.root
	r = clearInput(t, r)
	r, _ = upd(t, r, runeKey("/ws/saved"))
	r, _ = upd(t, r, keyEnter())
	if len(r.toasts) != 0 {
		t.Fatalf("apply must not toast yet, got %v", r.toasts)
	}
	r, toast := upd(t, r, keyCtrlS())
	r, _ = upd(t, r, toast())
	if len(r.toasts) != 1 || r.toasts[0].Level != "good" || !strings.Contains(r.toasts[0].Text, "settings saved") {
		t.Fatalf("toasts = %+v, want one good save notice", r.toasts)
	}
	if r.dashboard.cfg.Workspace.Root != "/ws/saved" {
		t.Fatal("dashboard cfg must follow a successful save")
	}
	body, err := os.ReadFile(configPath(t))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(body), "/ws/saved") {
		t.Fatalf("config.toml = %q, want the saved root", body)
	}
}

// TestSettingsResetSection: r restores the section's defaults in
// memory only — nothing hits disk until ctrl+s.
func TestSettingsResetSection(t *testing.T) {
	r := settingsFixture(t)
	defRoot := config.Default().Workspace.Root
	r, toast := upd(t, r, runeKey("r"))
	r, _ = upd(t, r, toast())
	if r.cfg.Workspace.Root != defRoot {
		t.Fatalf("root after reset = %q, want default %q", r.cfg.Workspace.Root, defRoot)
	}
	if len(r.toasts) != 1 || !strings.Contains(r.toasts[0].Text, "reset to defaults") {
		t.Fatalf("toasts = %+v, want a reset notice", r.toasts)
	}
	if _, err := os.Stat(configPath(t)); err == nil {
		t.Fatal("reset must not write config.toml on its own")
	}
}
