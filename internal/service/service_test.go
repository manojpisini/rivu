package service

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/registry"
)

func TestScanRejectsMissingWorkspaceRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	a.Config.Workspace.Root = filepath.Join(home, "never-created")
	if _, err := a.Scan(); err == nil {
		t.Fatal("Scan accepted a missing workspace root, want error")
	}
}

func TestScanIncludesSecondaryRoots(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	mkGo := func(root, name string) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module x"), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	root1 := filepath.Join(home, "ws1")
	root2 := filepath.Join(home, "ws2")
	p1 := mkGo(root1, "main")
	p2 := mkGo(root2, "extra")

	a.Config.Workspace.Root = root1
	a.Config.Workspace.SecondaryRoots = []string{root2, filepath.Join(home, "gone")}
	res, err := a.Scan()
	if err != nil {
		t.Fatalf("a missing secondary root must warn, not fail: %v", err)
	}
	paths := map[string]bool{}
	for _, p := range res.Projects {
		paths[p.Path] = true
	}
	if !paths[p1] || !paths[p2] {
		t.Errorf("projects from both roots expected, got %v", paths)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "secondary root skipped") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a secondary-root warning, got %v", res.Warnings)
	}
}

func TestSourceGitInitExclusivity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	owner := func(t *testing.T, slug string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(a.Config.Workspace.Root, "00_Source", slug, ".metadata", "project.toml"))
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]map[string]any
		if _, err := toml.Decode(string(b), &doc); err != nil {
			t.Fatal(err)
		}
		s, _ := doc["rivu"]["git_init_owner"].(string)
		return s
	}

	// Bridge owns init: --git is auto-corrected away with a warning.
	a.Config.Bridge.Enabled = true
	sr, err := a.Source("bridged", "source", true, false, false)
	if err != nil {
		t.Fatalf("Source with bridge: %v", err)
	}
	if len(sr.Warnings) != 1 || !strings.Contains(sr.Warnings[0], "bridge owns git init") {
		t.Errorf("expected an auto-correction warning, got %v", sr.Warnings)
	}
	if _, err := os.Stat(filepath.Join(a.Config.Workspace.Root, "00_Source", "bridged", ".git")); !os.IsNotExist(err) {
		t.Error("rivu ran git init despite the bridge owning it")
	}
	if got := owner(t, "bridged"); got != "bridge" {
		t.Errorf("git_init_owner = %q, want bridge", got)
	}

	// Bridge off, git requested: rivu inits and records itself.
	a.Config.Bridge.Enabled = false
	sr, err = a.Source("selfinit", "source", true, false, false)
	if err != nil {
		t.Fatalf("Source with git: %v", err)
	}
	if len(sr.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", sr.Warnings)
	}
	if _, err := os.Stat(filepath.Join(a.Config.Workspace.Root, "00_Source", "selfinit", ".git")); err != nil {
		t.Errorf("git init did not run: %v", err)
	}
	if got := owner(t, "selfinit"); got != "rivu" {
		t.Errorf("git_init_owner = %q, want rivu", got)
	}

	// Nobody inits: recorded as none, not rivu (O-02).
	if _, err := a.Source("noinit", "source", false, false, false); err != nil {
		t.Fatalf("Source without git: %v", err)
	}
	if got := owner(t, "noinit"); got != "none" {
		t.Errorf("git_init_owner = %q, want none", got)
	}
}

func TestSourceGitBranchAndGitignore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	a.Config.Git.DefaultBranch = "trunk"
	a.Config.Templates.Default = "go-cli"

	if _, err := a.Source("branched", "source", true, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	dir := filepath.Join(a.Config.Workspace.Root, "00_Source", "branched")
	head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(head), "refs/heads/trunk") {
		t.Errorf("HEAD = %q, want refs/heads/trunk (git init -b)", head)
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore not written: %v", err)
	}
	if !strings.Contains(string(gi), ".env") {
		t.Errorf(".gitignore missing starter entries: %q", gi)
	}
}

func TestSourceAdoptExistingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	// Pre-existing non-empty directory with user content and its own Bank.
	dir := filepath.Join(a.Config.Workspace.Root, "00_Source", "legacy")
	if err := os.MkdirAll(filepath.Join(dir, ".metadata"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("mine"), 0644); err != nil {
		t.Fatal(err)
	}
	customTOML := "[rivu]\nname = \"legacy\"\ncreated_at = 2020-01-01T00:00:00Z\ngit_init_owner = \"me\"\n"
	if err := os.WriteFile(filepath.Join(dir, ".metadata", "project.toml"), []byte(customTOML), 0644); err != nil {
		t.Fatal(err)
	}

	// Without --adopt it is refused before any write.
	if _, err := a.Source("legacy", "source", true, false, false); err == nil {
		t.Fatal("expected non-empty dir refusal")
	}
	if _, err := os.Stat(filepath.Join(dir, ".metadata", "agent")); !os.IsNotExist(err) {
		t.Error("refused Source must not write files")
	}

	// With --adopt: registered, Bank kept as-is, no git, no Map, README untouched.
	sr, err := a.Source("legacy", "source", true, true, false)
	if err != nil {
		t.Fatalf("adopt Source: %v", err)
	}
	p := sr.Project
	got, err := a.Registry.Resolve("legacy")
	if err != nil || got.Path == "" {
		t.Fatalf("registry row missing: %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil || string(readme) != "mine" {
		t.Errorf("README changed: %q %v", readme, err)
	}
	tomlAfter, err := os.ReadFile(filepath.Join(dir, ".metadata", "project.toml"))
	if err != nil || string(tomlAfter) != customTOML {
		t.Errorf("project.toml rewritten by adopt:\n%s", tomlAfter)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		t.Error("adopt ran git init")
	}
	if _, err := os.Stat(filepath.Join(dir, ".metadata", "agent", "PROJECT_MAP.md")); !os.IsNotExist(err) {
		t.Error("adopt built the Map")
	}
	if p.HasGit || p.HasMap {
		t.Errorf("flags = git:%v map:%v, want both false", p.HasGit, p.HasMap)
	}
	if len(sr.Warnings) != 1 || !strings.Contains(sr.Warnings[0], "skipped git init") {
		t.Errorf("warnings = %v, want skipped-git-init warning", sr.Warnings)
	}
}

func TestGitignoreForTemplates(t *testing.T) {
	for tmpl, want := range map[string]string{
		"go-cli":     "vendor/",
		"node-ts":    "node_modules/",
		"python-web": "__pycache__/",
		"":           "node_modules/",
		"unknown":    ".env",
	} {
		if got := gitignoreFor(tmpl); !strings.Contains(got, want) {
			t.Errorf("gitignoreFor(%q) missing %q", tmpl, want)
		}
	}
}

func TestSourcePreflightRejectsBeforeWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, err := a.Source("demo", "source", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	// Slug/path collision: rejected before any write.
	if _, err := a.Source("demo", "source", false, false, false); err == nil {
		t.Error("duplicate slug accepted")
	}
	// Non-empty existing directory: rejected, contents untouched.
	taken := filepath.Join(a.Config.Workspace.Root, "00_Source", "taken")
	if err := os.MkdirAll(taken, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taken, "keep.txt"), []byte("user data"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Source("taken", "source", false, false, false); err == nil {
		t.Error("non-empty directory accepted without adopt")
	}
	if got, _ := os.ReadFile(filepath.Join(taken, "keep.txt")); string(got) != "user data" {
		t.Errorf("pre-existing content damaged: %q", got)
	}
	// Empty existing directory: allowed.
	empty := filepath.Join(a.Config.Workspace.Root, "00_Source", "emptybox")
	if err := os.MkdirAll(empty, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Source("emptybox", "source", false, false, false); err != nil {
		t.Errorf("empty directory rejected: %v", err)
	}
	// git required but not on PATH: rejected before writing.
	t.Setenv("PATH", t.TempDir())
	if _, err := a.Source("needs-git", "source", true, false, false); err == nil {
		t.Error("missing git accepted")
	}
	if _, err := os.Stat(filepath.Join(a.Config.Workspace.Root, "00_Source", "needs-git")); !os.IsNotExist(err) {
		t.Error("folder created despite failed preflight")
	}
}

func TestSourceRollsBackWhatItCreated(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	// A fake git that always fails, found via PATH by preflight and exec.
	bin := t.TempDir()
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(filepath.Join(bin, "git.cmd"), []byte("@echo off\r\nexit /b 1\r\n"), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.WriteFile(filepath.Join(bin, "git"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, err := a.Source("doomed", "source", true, false, false); err == nil {
		t.Fatal("expected git init failure")
	}
	dir := filepath.Join(a.Config.Workspace.Root, "00_Source", "doomed")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("rollback left the created folder behind")
	}
	if taken, err := a.Registry.SlugOrPathTaken("doomed", dir); err != nil || taken {
		t.Errorf("rollback left a registry row: taken=%v err=%v", taken, err)
	}
}

func TestFlowPreflightRejectsUnsafeMoves(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, err := a.Source("demo", "source", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	// Destination already exists: rejected before renaming.
	src := filepath.Join(a.Config.Workspace.Root, "00_Source", "demo")
	dstDir := filepath.Join(a.Config.Workspace.Root, "01_Active")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dstDir, "demo"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Flow("demo", "active", false, false); err == nil {
		t.Error("existing destination accepted")
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source moved despite failed preflight: %v", err)
	}
	os.RemoveAll(filepath.Join(dstDir, "demo"))

	// Project outside every known root: rejected.
	foreign := t.TempDir()
	if err := os.MkdirAll(filepath.Join(foreign, "foreign"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := a.Registry.Upsert(registry.Project{ID: "foreign-1", Name: "foreign", Slug: "foreign", Path: filepath.Join(foreign, "foreign"), Channel: "00_Source", FlowStage: "source"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Flow("foreign", "active", false, false); err == nil {
		t.Error("foreign-path project accepted")
	}
	// Missing source folder: rejected with a rescan hint.
	if err := os.RemoveAll(src); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Flow("demo", "active", false, false); err == nil {
		t.Error("missing source folder accepted")
	}
}

func TestFlowNoOpWhenAlreadyInStage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, err := a.Source("demo", "active", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	fr, err := a.Flow("demo", "active", false, false)
	if err != nil {
		t.Fatalf("Flow: %v", err)
	}
	p, note := fr.Project, fr.Note
	if !strings.Contains(note, "already") {
		t.Errorf("expected a no-op note, got %q", note)
	}
	if p.FlowStage != "active" {
		t.Errorf("stage changed on no-op: %q", p.FlowStage)
	}
}

func TestFlowPreservesSubfolders(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, err := a.Source("rivu", "source", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	// Simulate a project living under a nested folder inside its channel.
	src := filepath.Join(a.Config.Workspace.Root, "00_Source", "devtools", "rivu")
	if err := os.MkdirAll(filepath.Dir(src), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(a.Config.Workspace.Root, "00_Source", "rivu"), src); err != nil {
		t.Fatal(err)
	}
	// Keep the registered row in sync with the moved folder.
	p0, err := a.Registry.Resolve("rivu")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Registry.UpdatePathFlow(p0.ID, src, "00_Source", "source"); err != nil {
		t.Fatal(err)
	}

	fr, err := a.Flow("rivu", "active", false, false)
	if err != nil {
		t.Fatalf("Flow: %v", err)
	}
	p, note := fr.Project, fr.Note
	wantDst := filepath.Join(a.Config.Workspace.Root, "01_Active", "devtools", "rivu")
	// Flow canonicalises paths; compare in canonical form (Windows 8.3).
	if x, err := filepath.EvalSymlinks(wantDst); err == nil {
		wantDst = x
	}
	if p.Path != wantDst {
		t.Errorf("sub-folders collapsed: path=%q note=%q want %q", p.Path, note, wantDst)
	}
	if _, err := os.Stat(wantDst); err != nil {
		t.Errorf("moved folder missing: %v", err)
	}

	// Flatten drops the intermediate folders.
	fr, err = a.Flow("rivu", "maintenance", true, false)
	if err != nil {
		t.Fatalf("Flow flatten: %v", err)
	}
	p, note = fr.Project, fr.Note
	wantFlat := filepath.Join(a.Config.Workspace.Root, "02_Maintenance", "rivu")
	if x, err := filepath.EvalSymlinks(wantFlat); err == nil {
		wantFlat = x
	}
	if p.Path != wantFlat {
		t.Errorf("flatten ignored: path=%q note=%q want %q", p.Path, note, wantFlat)
	}
	if _, err := os.Stat(wantFlat); err != nil {
		t.Errorf("flattened folder missing: %v", err)
	}
}

func TestMapAndFlowSetAssetFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	a.Config.Automation.CreateBank = false
	a.Config.Automation.BuildMap = false

	flags := func(t *testing.T, q string) (bank, m bool) {
		t.Helper()
		list, err := a.Registry.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range list {
			if p.Slug == q {
				return p.HasBank, p.HasMap
			}
		}
		t.Fatalf("project %q not found", q)
		return
	}

	if _, err := a.Source("demo", "source", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	if b, m := flags(t, "demo"); b || m {
		t.Errorf("auto-creation off: has_bank=%v has_map=%v", b, m)
	}
	if err := a.Map("demo"); err != nil {
		t.Fatalf("Map: %v", err)
	}
	if b, m := flags(t, "demo"); !b || !m {
		t.Errorf("after Map: has_bank=%v has_map=%v, want true/true", b, m)
	}

	if _, err := a.Source("other", "source", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	if _, err := a.Flow("other", "active", false, false); err != nil {
		t.Fatalf("Flow: %v", err)
	}
	if b, m := flags(t, "other"); !b || m {
		t.Errorf("after Flow: has_bank=%v has_map=%v, want true/false", b, m)
	}
}

func TestFlowRefreshesBankStage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	a.Config.Automation.CreateBank = true

	if _, err := a.Source("demo", "source", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	fr, err := a.Flow("demo", "active", false, false)
	if err != nil {
		t.Fatalf("Flow: %v", err)
	}
	p := fr.Project
	b, err := os.ReadFile(filepath.Join(p.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]any
	if _, err := toml.Decode(string(b), &doc); err != nil {
		t.Fatalf("moved project.toml must parse: %v", err)
	}
	if doc["rivu"]["flow_stage"] != "active" || doc["rivu"]["channel"] != "01_Active" {
		t.Errorf("stale project.toml after Flow: %v", doc["rivu"])
	}
}

func TestSourceSlugsNamesAndRejectsUnsafeOnes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, err := a.Source("../../evil", "source", false, false, false); err == nil {
		t.Error("Source accepted a traversal name, want error")
	}
	if _, err := a.Source("CON", "source", false, false, false); err == nil {
		t.Error("Source accepted a reserved device name, want error")
	}
	sr, err := a.Source("My Project", "source", false, false, false)
	p := sr.Project
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if p.Slug != "my-project" {
		t.Errorf("slug = %q, want my-project", p.Slug)
	}
	want := filepath.Join(a.Config.Workspace.Root, "00_Source", "my-project")
	if p.Path != want {
		t.Errorf("path = %q, want %q", p.Path, want)
	}
	if fi, err := os.Stat(want); err != nil || !fi.IsDir() {
		t.Errorf("project dir not created: %v", err)
	}
}

func TestScanRejectsInvalidConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	body := "[scanner]\nmax_depth = 0\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(); err == nil {
		t.Fatal("Open accepted max_depth = 0, want error")
	}
}

func openTestApp(t *testing.T) *App {
	t.Helper()
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	a.Config.Workspace.Root = filepath.Join(home, "ws")
	return a
}

func TestOpenProjectRefusesMissingPath(t *testing.T) {
	a := openTestApp(t)
	sr, err := a.Source("vanish", "source", false, false, false)
	p := sr.Project
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}

	err = a.OpenProject("vanish")
	if err == nil {
		t.Fatal("OpenProject on a vanished path must refuse")
	}
	if !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "rivu scan") {
		t.Errorf("error must offer rivu scan: %v", err)
	}
	got, rerr := a.Registry.Resolve("vanish")
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got.OnDisk {
		t.Error("vanished project must be flagged on_disk=0")
	}
	if !got.LastOpenedAt.IsZero() {
		t.Error("refused open must not record last_opened_at")
	}
}

func TestOpenProjectOpensOnDisk(t *testing.T) {
	a := openTestApp(t)
	if runtime.GOOS == "windows" {
		a.Config.Editors.Default = "cmd /c exit 0"
	} else {
		a.Config.Editors.Default = "true"
	}
	sr, err := a.Source("handy", "source", false, false, false)
	p := sr.Project
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if err := a.OpenProject("handy"); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	got, err := a.Registry.Resolve("handy")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastOpenedAt.IsZero() {
		t.Error("open must record last_opened_at")
	}
	if !got.OnDisk || got.ID != p.ID {
		t.Errorf("project state after open: %+v", got)
	}
}

func TestListAndCurrent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()
	a.Config.Workspace.Root = filepath.Join(home, "ws")

	if _, ok := a.Current(); ok {
		t.Error("Current set before any project exists")
	}
	if _, err := a.Source("solo", "source", false, false, false); err != nil {
		t.Fatalf("Source: %v", err)
	}
	ps, err := a.List()
	if err != nil || len(ps) != 1 || ps[0].Name != "solo" {
		t.Errorf("List = %v, %v; want one project solo", ps, err)
	}
	p, ok := a.Current()
	if !ok || p.Name != "solo" {
		t.Errorf("Current = %+v, %v; want solo", p, ok)
	}
}
