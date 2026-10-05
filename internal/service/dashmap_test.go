package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

func TestDashboardSnapshot(t *testing.T) {
	a := openTestApp(t)
	seedProjects(t, a, "alpha", "beta")

	d, err := a.Dashboard()
	if err != nil {
		t.Fatalf("Dashboard: %v", err)
	}
	if d.Root != a.Config.Workspace.Root {
		t.Errorf("Root = %q, want %q", d.Root, a.Config.Workspace.Root)
	}
	if d.Roots != 1 {
		t.Errorf("Roots = %d, want 1", d.Roots)
	}
	if d.Stats.Total != 2 {
		t.Errorf("Stats.Total = %d, want 2", d.Stats.Total)
	}
	// Source logs one event per project.
	if len(d.Recent) != 2 || d.Recent[0].Event != "sourced" {
		t.Errorf("Recent = %+v, want two sourced events", d.Recent)
	}
	// Triage priority: folders exist and are registered, so the first
	// non-zero bucket must be the projects without git.
	if len(d.Attention) == 0 || d.Attention[0].Key != "missing_git" || d.Attention[0].Count != 2 {
		t.Errorf("Attention = %+v, want missing_git=2 first", d.Attention)
	}
	for _, at := range d.Attention {
		if at.Count <= 0 {
			t.Errorf("zero bucket leaked: %+v", at)
		}
	}
}

func TestFlowBulkContinuesPastFailures(t *testing.T) {
	a := openTestApp(t)
	seedProjects(t, a, "alpha", "beta")

	res, err := a.FlowBulk([]string{"alpha", "ghost"}, "delta", false, false)
	if err != nil {
		t.Fatalf("FlowBulk: %v", err)
	}
	if len(res.Done) != 1 {
		t.Errorf("Done = %d, want 1", len(res.Done))
	}
	if len(res.Failed) != 1 || res.Failed[0].Query != "ghost" {
		t.Fatalf("Failed = %+v, want ghost", res.Failed)
	}
	if !errors.Is(res.Failed[0].Err, registry.ErrNotFound) {
		t.Errorf("ghost error = %v, want ErrNotFound", res.Failed[0].Err)
	}

	dry, err := a.FlowBulk([]string{"beta"}, "delta", false, true)
	if err != nil || len(dry.Done) != 1 {
		t.Fatalf("dry FlowBulk = %+v, %v; want one done", dry, err)
	}
	if _, err := os.Stat(filepath.Join(a.Config.Workspace.Root, "00_Source", "beta")); err != nil {
		t.Errorf("dry run moved the folder: %v", err)
	}

	if _, err := a.FlowBulk([]string{"alpha"}, "bogus", false, false); err == nil {
		t.Error("invalid target stage must fail the whole call")
	}
}

func TestDoctorReportsScore(t *testing.T) {
	a := openTestApp(t)
	seedProjects(t, a, "one")

	reps, err := a.Doctor("one")
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if len(reps) != 1 || reps[0].Project.Slug != "one" {
		t.Fatalf("Doctor(one) = %+v, want one report", reps)
	}
	if len(reps[0].Checks) == 0 {
		t.Error("report has no checks")
	}
	p, err := a.Registry.Find("one")
	if err != nil {
		t.Fatal(err)
	}
	if p.HealthScore != reps[0].Score {
		t.Errorf("registry health %d != report score %d", p.HealthScore, reps[0].Score)
	}

	if _, err := a.Doctor("ghost"); !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("Doctor(ghost) = %v, want ErrNotFound", err)
	}
	all, err := a.Doctor("")
	if err != nil || len(all) != 1 {
		t.Errorf("Doctor(all) = %d reports, %v; want 1", len(all), err)
	}
}

func TestMapStatusAndMapBulk(t *testing.T) {
	a := openTestApp(t)
	seeded := seedProjects(t, a, "m1", "m2")
	p1 := seeded["m1"]

	// Source already builds the Map, so the first read is clean.
	st, err := a.MapStatus("m1", false)
	if err != nil {
		t.Fatalf("MapStatus: %v", err)
	}
	if len(st) != 1 || st[0].AgentsMissing || st[0].MapStale {
		t.Fatalf("initial status = %+v, want one clean project", st)
	}

	// Lose AGENTS.md and dirty the map to exercise both flags.
	agentDir := filepath.Join(p1.Path, ".metadata", "agent")
	if err := os.Remove(filepath.Join(agentDir, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "PROJECT_MAP.md"), []byte("junk"), 0644); err != nil {
		t.Fatal(err)
	}
	st, err = a.MapStatus("m1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) != 1 || !st[0].AgentsMissing || !st[0].MapStale {
		t.Fatalf("dirty status = %+v, want agents missing and map stale", st)
	}
	if _, err := a.MapStatus("ghost", false); !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("MapStatus(ghost) = %v, want ErrNotFound", err)
	}

	res, err := a.MapBulk()
	if err != nil {
		t.Fatalf("MapBulk: %v", err)
	}
	if len(res.Done) != 2 || len(res.Failed) != 0 {
		t.Fatalf("MapBulk = done %d failed %d, want 2/0", len(res.Done), len(res.Failed))
	}

	all, err := a.MapStatus("", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("MapStatus(all) = %d, want 2", len(all))
	}
	for _, s := range all {
		if s.AgentsMissing || s.MapStale {
			t.Errorf("%s still dirty after MapBulk: %+v", s.Project.Slug, s)
		}
	}

	// One broken folder never blocks the rest.
	blocker := filepath.Join(a.Config.Workspace.Root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := a.Registry.Upsert(registry.Project{ID: "bad1", Name: "Bad", Slug: "bad", Path: filepath.Join(blocker, "child"), FlowStage: "source", Channel: "00_Source"}); err != nil {
		t.Fatal(err)
	}
	res2, err := a.MapBulk()
	if err != nil {
		t.Fatalf("MapBulk with blocker: %v", err)
	}
	if len(res2.Done) != 2 || len(res2.Failed) != 1 {
		t.Errorf("MapBulk with blocker = done %d failed %d, want 2/1", len(res2.Done), len(res2.Failed))
	}
	if len(res2.Failed) == 1 && res2.Failed[0].Query != "bad" {
		t.Errorf("failed query = %q, want bad", res2.Failed[0].Query)
	}
}

func TestIndexFailures(t *testing.T) {
	a := openTestApp(t)
	seedProjects(t, a, "ok")

	// Primary root that is not a directory fails the embedded scan.
	goodRoot := a.Config.Workspace.Root
	fileRoot := filepath.Join(a.Config.Workspace.Root, "blocker")
	if err := os.WriteFile(fileRoot, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	a.Config.Workspace.Root = fileRoot
	if _, err := a.Index(); err == nil {
		t.Error("Index over a file root must fail")
	}
	a.Config.Workspace.Root = goodRoot

	// A stage mismatch whose project.toml cannot be written fails only
	// that row: the folder stays detectable on disk (strong marker) but
	// project.toml is a directory, so bank.Sync's WriteFile fails.
	sr, err := a.Source("reloc2", SourceOpts{Flow: "active"})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	if err := a.Registry.UpdatePathFlow(sr.Project.ID, sr.Project.Path, "01_Active", "source"); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(sr.Project.Path, ".metadata")
	if err := os.RemoveAll(meta); err != nil {
		t.Fatal(err)
	}
	mkdir(t, filepath.Join(meta, "project.toml"))

	res, err := a.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if len(res.Failures) != 1 || res.Failures[0].Query != "reloc2" {
		t.Errorf("Failures = %+v, want reloc2 bank sync failure", res.Failures)
	}
}

func TestScanWarnsOnBrokenSecondaryRoot(t *testing.T) {
	a := openTestApp(t)
	seedProjects(t, a, "one")
	blocker := filepath.Join(a.Config.Workspace.Root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	a.Config.Workspace.SecondaryRoots = []string{filepath.Join(blocker, "child")}

	res, err := a.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "secondary root skipped") {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings = %v, want a secondary root skip", res.Warnings)
	}
}

func TestStatsPrefersLastOpenedForActivity(t *testing.T) {
	a := openTestApp(t)
	seeded := seedProjects(t, a, "touchy")
	tweak(t, a, seeded["touchy"].ID, "created_at", time.Now().AddDate(0, 0, -30).Format(time.RFC3339))
	tweak(t, a, seeded["touchy"].ID, "last_opened_at", time.Now().Format(time.RFC3339))

	if _, err := a.Stats(0); err != nil {
		t.Fatalf("Stats: %v", err)
	}
}

func TestOpenCommandArgv(t *testing.T) {
	a := openTestApp(t)
	seeded := seedProjects(t, a, "ed")
	p := seeded["ed"]

	argv, err := a.OpenCommand("ed", "code --wait")
	if err != nil {
		t.Fatalf("OpenCommand: %v", err)
	}
	want := strings.Join([]string{"code", "--wait", p.Path}, " ")
	if got := strings.Join(argv, " "); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}

	// Empty editor resolves a default but still launches nothing.
	argv, err = a.OpenCommand("ed", "")
	if err != nil {
		t.Fatalf("OpenCommand default editor: %v", err)
	}
	if len(argv) == 0 || argv[len(argv)-1] != p.Path {
		t.Errorf("default argv = %v, want trailing project path", argv)
	}

	if _, err := a.OpenCommand("ed", `code "unclosed`); err == nil {
		t.Error("unbalanced quotes must fail")
	}

	// A vanished folder is reported, never opened (E-03).
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := a.OpenCommand("ed", "code"); !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("vanished folder = %v, want ErrNotFound", err)
	}
	if _, err := a.OpenCommand("ghost", "code"); !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("OpenCommand(ghost) = %v, want ErrNotFound", err)
	}
}
