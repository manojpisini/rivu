package tui

import (
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
// dashboard with `S`.
func settingsFixture(t *testing.T) Root {
	t.Helper()
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
			if f[0] != "version" && f[0] != "go" && f[0] != "platform" && !strings.Contains(f[0], ".") {
				t.Errorf("%s: field %q is not a config.toml key", s.name, f[0])
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
