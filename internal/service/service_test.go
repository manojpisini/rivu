package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/logx"
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
	sr, err := a.Source("bridged", SourceOpts{Flow: "source", Git: true})
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
	sr, err = a.Source("selfinit", SourceOpts{Flow: "source", Git: true})
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
	if _, err := a.Source("noinit", SourceOpts{Flow: "source"}); err != nil {
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

	if _, err := a.Source("branched", SourceOpts{Flow: "source", Git: true}); err != nil {
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
	if _, err := a.Source("legacy", SourceOpts{Flow: "source", Git: true}); err == nil {
		t.Fatal("expected non-empty dir refusal")
	}
	if _, err := os.Stat(filepath.Join(dir, ".metadata", "agent")); !os.IsNotExist(err) {
		t.Error("refused Source must not write files")
	}

	// With --adopt: registered, Bank kept as-is, no git, no Map, README untouched.
	sr, err := a.Source("legacy", SourceOpts{Flow: "source", Git: true, Adopt: true})
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

	if _, err := a.Source("demo", SourceOpts{Flow: "source"}); err != nil {
		t.Fatalf("Source: %v", err)
	}
	// Slug/path collision: rejected before any write.
	if _, err := a.Source("demo", SourceOpts{Flow: "source"}); err == nil {
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
	if _, err := a.Source("taken", SourceOpts{Flow: "source"}); err == nil {
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
	if _, err := a.Source("emptybox", SourceOpts{Flow: "source"}); err != nil {
		t.Errorf("empty directory rejected: %v", err)
	}
	// git required but not on PATH: rejected before writing.
	t.Setenv("PATH", t.TempDir())
	if _, err := a.Source("needs-git", SourceOpts{Flow: "source", Git: true}); err == nil {
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

	if _, err := a.Source("doomed", SourceOpts{Flow: "source", Git: true}); err == nil {
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

	if _, err := a.Source("demo", SourceOpts{Flow: "source"}); err != nil {
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

	if _, err := a.Source("demo", SourceOpts{Flow: "active"}); err != nil {
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

	if _, err := a.Source("rivu", SourceOpts{Flow: "source"}); err != nil {
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

	if _, err := a.Source("demo", SourceOpts{Flow: "source"}); err != nil {
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

	if _, err := a.Source("other", SourceOpts{Flow: "source"}); err != nil {
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

	if _, err := a.Source("demo", SourceOpts{Flow: "source"}); err != nil {
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

	if _, err := a.Source("../../evil", SourceOpts{Flow: "source"}); err == nil {
		t.Error("Source accepted a traversal name, want error")
	}
	if _, err := a.Source("CON", SourceOpts{Flow: "source"}); err == nil {
		t.Error("Source accepted a reserved device name, want error")
	}
	sr, err := a.Source("My Project", SourceOpts{Flow: "source"})
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
	sr, err := a.Source("vanish", SourceOpts{Flow: "source"})
	p := sr.Project
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}

	err = a.OpenProject("vanish", "")
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
	sr, err := a.Source("handy", SourceOpts{Flow: "source"})
	p := sr.Project
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if err := a.OpenProject("handy", ""); err != nil {
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
	if _, err := a.Source("solo", SourceOpts{Flow: "source"}); err != nil {
		t.Fatalf("Source: %v", err)
	}
	ps, err := a.List(Filter{})
	if err != nil || len(ps) != 1 || ps[0].Name != "solo" {
		t.Errorf("List = %v, %v; want one project solo", ps, err)
	}
	p, ok := a.Current()
	if !ok || p.Name != "solo" {
		t.Errorf("Current = %+v, %v; want solo", p, ok)
	}
}

func TestScanContextCancelledBeforeCommit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	root := filepath.Join(home, "ws")
	p := filepath.Join(root, "demo")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	a.Config.Workspace.Root = root

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.ScanContext(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	ps, err := a.List(Filter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ps) != 0 {
		t.Errorf("cancelled scan committed %d projects, want none", len(ps))
	}
}

// TestDoctorAttachesSnapshotTrend: health_snapshots rows ride along on
// the Doctor report for the sparkline (P4.14), oldest first — including
// the fresh point the same run just wrote (P5.05).
func TestDoctorAttachesSnapshotTrend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	p := registry.Project{ID: "p1", Name: "Alpha", Slug: "alpha", Path: filepath.Join(home, "alpha"), FlowStage: "source"}
	if err := a.Registry.Upsert(p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for i, score := range []int{55, 60} {
		if _, err := a.Registry.DB.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES(?,?,?,?)`,
			"s"+string(rune('0'+i)), "p1", score, base.Add(time.Duration(i)*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := a.Doctor("alpha")
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("reports = %d, want 1", len(rs))
	}
	if len(rs[0].Trend) != 3 || rs[0].Trend[0] != 55 || rs[0].Trend[1] != 60 || rs[0].Trend[2] != rs[0].Score {
		t.Errorf("Trend = %v, want [55 60 %d] oldest first", rs[0].Trend, rs[0].Score)
	}
}

// TestDoctorWritesSnapshotAndDedupes: each run persists today's score
// into health_snapshots; a second run the same day with an unchanged
// score adds no row (P5.05).
func TestDoctorWritesSnapshotAndDedupes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	p := registry.Project{ID: "p1", Name: "Alpha", Slug: "alpha", Path: filepath.Join(home, "alpha"), FlowStage: "source"}
	if err := a.Registry.Upsert(p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	rs, err := a.Doctor("alpha")
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("reports = %d, want 1", len(rs))
	}
	count := func() int {
		t.Helper()
		var n int
		if err := a.Registry.DB.QueryRow(`SELECT count(*) FROM health_snapshots WHERE project_id='p1'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(); n != 1 {
		t.Fatalf("snapshots after first run = %d, want 1", n)
	}
	if len(rs[0].Trend) != 1 || rs[0].Trend[0] != rs[0].Score {
		t.Errorf("Trend = %v, want the fresh score %d", rs[0].Trend, rs[0].Score)
	}
	if _, err := a.Doctor("alpha"); err != nil {
		t.Fatalf("second Doctor: %v", err)
	}
	if n := count(); n != 1 {
		t.Fatalf("snapshots after same-day rerun = %d, want 1 (deduped)", n)
	}
}

// TestDoctorPrunesSnapshotsBeyondRetention: a doctor run drops history
// older than snapshot_retention_days and keeps the rest; a non-positive
// window keeps everything (P5.06).
func TestDoctorPrunesSnapshotsBeyondRetention(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")

	a, err := Open()
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	p := registry.Project{ID: "p1", Name: "Alpha", Slug: "alpha", Path: filepath.Join(home, "alpha"), FlowStage: "source"}
	if err := a.Registry.Upsert(p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	now := time.Now()
	seed := []struct {
		id string
		at time.Time
	}{
		{"ancient", now.AddDate(0, 0, -120)},
		{"fresh", now.AddDate(0, 0, -10)},
	}
	for _, s := range seed {
		if _, err := a.Registry.DB.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES(?,?,?,?)`,
			s.id, "p1", 60, s.at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Doctor("alpha"); err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	got, err := a.Registry.HealthSnapshots("p1", 0)
	if err != nil {
		t.Fatal(err)
	}
	// fresh (day -10) + today's run (retention 90 days); ancient is gone.
	if len(got) != 2 {
		t.Fatalf("snapshots after prune = %+v, want 2 (fresh + today)", got)
	}
	// A zero window disables pruning entirely.
	a.Config.Data.SnapshotRetentionDays = 0
	if _, err := a.Registry.DB.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES('ancient2','p1',40,?)`,
		now.AddDate(0, 0, -400)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Doctor("alpha"); err != nil {
		t.Fatalf("Doctor (retention 0): %v", err)
	}
	if got, _ := a.Registry.HealthSnapshots("p1", 0); len(got) != 3 {
		t.Fatalf("snapshots with retention 0 = %+v, want 3 (nothing pruned)", got)
	}
}

// TestLogTailAndRecentActivity backs the Logs screen (P5.10): the
// tail reads <home>/logs/rivu.log (missing file is empty, not an
// error) and activity_log rows come through the service.
func TestLogTailAndRecentActivity(t *testing.T) {
	a := openTestApp(t)

	if tail, err := a.LogTail(10); err != nil || len(tail) != 0 {
		t.Fatalf("LogTail before any file = %q (err %v), want empty", tail, err)
	}

	dir := filepath.Join(os.Getenv("RIVU_HOME"), "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logx.Path(filepath.Join(os.Getenv("RIVU_HOME"))),
		[]byte("one\ntwo\nthree\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tail, err := a.LogTail(2)
	if err != nil {
		t.Fatalf("LogTail: %v", err)
	}
	if len(tail) != 2 || tail[0] != "two" || tail[1] != "three" {
		t.Errorf("LogTail(2) = %q, want [two three]", tail)
	}

	if _, err := a.Source("log-act", SourceOpts{Flow: "source"}); err != nil {
		t.Fatalf("Source: %v", err)
	}
	act, err := a.RecentActivity(5)
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	if len(act) == 0 || act[0].Slug != "log-act" || act[0].Event != "sourced" {
		t.Errorf("RecentActivity = %+v, want newest sourced log-act", act)
	}
}
