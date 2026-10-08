package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplyDoctorFixesSafeCreates (P6.03): --fix readme creates a
// missing README, never touches an existing one, and unknown ids are
// refused; --fix map rebuilds the machine-owned Bank/Map files.
func TestApplyDoctorFixesSafeCreates(t *testing.T) {
	a := openTestApp(t)
	sr, err := a.Source("alpha", SourceOpts{Flow: "source"})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	p := sr.Project
	readme := filepath.Join(p.Path, "README.md")

	// The plan lists only fixes that actually apply.
	rs, err := a.Doctor("alpha")
	if err != nil {
		t.Fatal(err)
	}
	plan := DoctorFixPlan(rs, []string{"readme"})
	if len(plan) != 1 || !strings.Contains(plan[0], "alpha: create README.md") {
		t.Fatalf("plan = %v, want one readme line for alpha", plan)
	}

	// Apply: README appears with the project name.
	applied, err := a.ApplyDoctorFixes(rs, []string{"readme"})
	if err != nil {
		t.Fatalf("ApplyDoctorFixes: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	b, err := os.ReadFile(readme)
	if err != nil {
		t.Fatalf("README.md missing after fix: %v", err)
	}
	if !strings.Contains(string(b), "# alpha") {
		t.Errorf("README = %q, want the project name", b)
	}

	// Never overwrite: an existing README is left exactly as it was.
	if err := os.WriteFile(readme, []byte("KEEP"), 0644); err != nil {
		t.Fatal(err)
	}
	rs, _ = a.Doctor("alpha")
	if applied, err = a.ApplyDoctorFixes(rs, []string{"readme"}); err != nil || applied != 0 {
		t.Fatalf("second apply = %d, %v — want no-op on an existing README", applied, err)
	}
	if b, _ := os.ReadFile(readme); string(b) != "KEEP" {
		t.Errorf("existing README rewritten to %q", b)
	}

	// Unknown ids are refused rather than silently ignored.
	if _, err := a.ApplyDoctorFixes(rs, []string{"license"}); err == nil {
		t.Error("unknown fix id must error")
	}
}

// TestApplyDoctorFixesMapRebuild: --fix map regenerates Bank and Map
// (machine-owned files, safety rule 3) when they are missing.
func TestApplyDoctorFixesMapRebuild(t *testing.T) {
	a := openTestApp(t)
	sr, err := a.Source("beta", SourceOpts{Flow: "source", CreateBank: boolPtr(false)})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	p := sr.Project
	mapFile := filepath.Join(p.Path, ".metadata", "agent", "PROJECT_MAP.md")
	if _, err := os.Stat(mapFile); err == nil {
		if err := os.Remove(mapFile); err != nil { // test fixture reset, not a project delete
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(filepath.Join(p.Path, ".metadata")); err != nil { // fixture reset of machine metadata only
		t.Fatal(err)
	}
	rs, err := a.Doctor("beta")
	if err != nil {
		t.Fatal(err)
	}
	plan := DoctorFixPlan(rs, []string{"map"})
	if len(plan) == 0 || !strings.Contains(strings.Join(plan, "\n"), "rebuild Bank and Map") {
		t.Fatalf("plan = %v, want a map rebuild line", plan)
	}
	applied, err := a.ApplyDoctorFixes(rs, []string{"map"})
	if err != nil {
		t.Fatalf("ApplyDoctorFixes map: %v", err)
	}
	if applied != 1 {
		t.Fatalf("applied = %d, want 1", applied)
	}
	if _, err := os.Stat(filepath.Join(p.Path, ".metadata", "project.toml")); err != nil {
		t.Errorf("Bank not rebuilt: %v", err)
	}
	if _, err := os.Stat(mapFile); err != nil {
		t.Errorf("PROJECT_MAP.md not rebuilt: %v", err)
	}
}

func boolPtr(b bool) *bool { return &b }
