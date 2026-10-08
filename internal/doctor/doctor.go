package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/registry"
)

// Check is one health check (D-04). ID is the stable machine key —
// it matches the `[health].weights` config keys for scoring checks.
// Severity is "error" or "warn" ("" reads as warn) and drives the
// failure marker. Detail describes the passing state (and is what
// --json reports). When a check fails, Finding says exactly what is
// wrong and Remedy says what to do about it (D-03, P6.03). Remedy may
// contain {slug}, filled in by the caller; Fixable names the safe
// `rivu doctor --fix` id when a machine can apply the remedy without
// a product decision. Weight is the score contribution (0 = never
// scores: informational checks).
type Check struct {
	ID       string
	Name     string
	Severity string
	OK       bool
	Weight   int
	Detail   string
	Finding  string
	Remedy   string
	Fixable  string
}

type Report struct {
	Project registry.Project
	Checks  []Check
	Score   int
	// Missing marks a registered project whose folder is gone. Run
	// short-circuits instead of scoring a pile of failures (D-07);
	// callers show MISSING rather than 0/100.
	Missing bool
	// Trend is the recent health history, oldest first, filled by the
	// service from health_snapshots for the sparkline (P4.14).
	Trend []int
}

func ex(p string) bool { _, e := os.Stat(p); return e == nil }

// gitInitOwner reads the recorded git init owner from the Bank, "" if unknown.
func gitInitOwner(path string) string {
	b, err := os.ReadFile(filepath.Join(path, ".metadata", "project.toml"))
	if err != nil {
		return ""
	}
	var f struct {
		Rivu struct {
			GitInitOwner string `toml:"git_init_owner"`
		} `toml:"rivu"`
	}
	if err := toml.Unmarshal(b, &f); err != nil {
		return ""
	}
	return f.Rivu.GitInitOwner
}

// testSkipDirs are never walked looking for tests (D-02).
var testSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "target": true,
	"dist": true, "build": true, "coverage": true, ".metadata": true,
}

// testsCheck is the D-02 real test detection: suites on disk, test
// file patterns, or an ecosystem runner configured — not the old
// `tests/` OR `internal/` proxy.
func testsCheck(root string) Check {
	c := Check{
		ID: "tests", Name: "Tests", Severity: "warn", Weight: 10,
		Detail:  "tests detected",
		Finding: "no tests detected",
		Remedy:  "add tests (a tests/ folder, *_test.go files, or a configured test script)",
	}
	c.OK = hasTests(root)
	return c
}

func hasTests(root string) bool {
	for _, d := range []string{"test", "tests", "__tests__", "spec"} {
		if ex(filepath.Join(root, d)) {
			return true
		}
	}
	if ex(filepath.Join(root, "pytest.ini")) || pyprojectHasPytest(root) || pkgJSONHasTestScript(root) {
		return true
	}
	found := false
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are not evidence either way
		}
		if d.IsDir() {
			if testSkipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		n := d.Name()
		switch {
		case strings.HasSuffix(n, "_test.go"),
			strings.Contains(n, ".spec."),
			strings.Contains(n, ".test."):
			found = true
			return fs.SkipAll
		case strings.HasSuffix(n, ".rs"):
			if b, err := os.ReadFile(path); err == nil && strings.Contains(string(b), "#[cfg(test)]") {
				found = true
				return fs.SkipAll
			}
		}
		return nil
	})
	return found
}

// pyprojectHasPytest reports whether pyproject.toml configures pytest
// ([tool.pytest] or [tool.pytest.ini_options]).
func pyprojectHasPytest(root string) bool {
	b, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return false
	}
	var doc map[string]any
	if err := toml.Unmarshal(b, &doc); err != nil {
		return false
	}
	tool, _ := doc["tool"].(map[string]any)
	_, ok := tool["pytest"]
	return ok
}

// pkgJSONHasTestScript reports whether package.json defines a test
// script (the runner may live outside the repo tree).
func pkgJSONHasTestScript(root string) bool {
	b, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return false
	}
	var pj struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(b, &pj); err != nil {
		return false
	}
	s, ok := pj.Scripts["test"]
	return ok && strings.TrimSpace(s) != ""
}

// depManifests pairs a dependency manifest with the lockfiles that
// make its versions reproducible. Ecosystems without a lockfile
// convention (Maven, Gradle, plain scripts) are deliberately absent —
// there is nothing to verify for them.
var depManifests = []struct {
	manifest string
	locks    []string
	tidy     string // the safe command that produces the lockfile
}{
	{"go.mod", []string{"go.sum"}, "run `go mod tidy`"},
	{"package.json", []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb"}, "run `npm install` (or yarn/pnpm install)"},
	{"Cargo.toml", []string{"Cargo.lock"}, "run `cargo generate-lockfile`"},
	{"pyproject.toml", []string{"poetry.lock", "uv.lock"}, "run `poetry lock` or `uv lock`"},
	{"Pipfile", []string{"Pipfile.lock"}, "run `pipenv lock`"},
	{"Gemfile", []string{"Gemfile.lock"}, "run `bundle install`"},
	{"composer.json", []string{"composer.lock"}, "run `composer update --lock`"},
}

// depCheck is the D-01 real dependency check: every known manifest
// must ship a lockfile. A project with no tracked manifest passes
// vacuously (there are no dependencies to pin).
func depCheck(root string) Check {
	tracked := false
	for _, d := range depManifests {
		if !ex(filepath.Join(root, d.manifest)) {
			continue
		}
		tracked = true
		locked := false
		for _, l := range d.locks {
			if ex(filepath.Join(root, l)) {
				locked = true
				break
			}
		}
		if !locked {
			return Check{
				ID: "deps_fresh", Name: "Dependencies", Severity: "warn", Weight: 5,
				Detail:  "manifest without lockfile: " + d.manifest,
				OK:      false,
				Finding: "manifest without lockfile: " + d.manifest,
				Remedy:  d.tidy,
			}
		}
	}
	if !tracked {
		return Check{
			ID: "deps_fresh", Name: "Dependencies", Severity: "warn", Weight: 5, OK: true,
			Detail: "no dependency manifest",
		}
	}
	return Check{
		ID: "deps_fresh", Name: "Dependencies", Severity: "warn", Weight: 5, OK: true,
		Detail: "manifest and lockfile present",
	}
}

// onDiskCheck (D-04) verifies the project folder is still where the
// registry says it is. It never scores; when the folder is gone Run
// short-circuits with only this check failing (D-07).
func onDiskCheck(root string) Check {
	return Check{
		ID: "on_disk", Name: "On disk", Severity: "error", Weight: 0,
		Detail:  "project folder present",
		OK:      ex(root),
		Finding: "project folder missing on disk",
		Remedy:  "restore the folder, or re-add the project after moving it back",
	}
}

// staleCheck (D-04) flags a project quieter than the configured
// threshold. Activity is max(created, opened) — the same rule the
// dashboard uses, so the two counts can never disagree.
func staleCheck(p registry.Project, staleDays int) (Check, bool) {
	if staleDays < 1 {
		return Check{}, false
	}
	act := p.CreatedAt
	if p.LastOpenedAt.After(act) {
		act = p.LastOpenedAt
	}
	stale := time.Now().AddDate(0, 0, -staleDays).After(act)
	c := Check{
		ID: "stale", Name: "Stale", Severity: "warn", Weight: 0,
		Detail:  fmt.Sprintf("activity within %dd", staleDays),
		OK:      !stale,
		Finding: fmt.Sprintf("no activity in %dd", staleDays),
		Remedy:  "open the project, or move it to Delta if it is retired",
	}
	return c, true
}

// bankIncompleteCheck (D-04): project.toml exists but the human Bank
// files do not. Only evaluated once the Bank is initialised — the
// Bank check already carries the "no project.toml" case. `--fix map`
// heals it: bank.Build fills what is missing and never overwrites.
func bankIncompleteCheck(root string) (Check, bool) {
	meta := filepath.Join(root, ".metadata")
	if !ex(filepath.Join(meta, "project.toml")) {
		return Check{}, false
	}
	var missing []string
	for _, f := range []string{"overview.md", "decisions.md", "tasks.md"} {
		if !ex(filepath.Join(meta, f)) {
			missing = append(missing, f)
		}
	}
	if len(missing) == 0 {
		return Check{
			ID: "bank_incomplete", Name: "Bank files", Severity: "warn", Weight: 0,
			OK: true, Detail: "human docs present",
		}, true
	}
	return Check{
		ID: "bank_incomplete", Name: "Bank files", Severity: "warn", Weight: 0,
		OK:      false,
		Detail:  "human docs present",
		Finding: "Bank incomplete: missing " + strings.Join(missing, ", "),
		Remedy:  "run `rivu agent sync {slug}` to create the missing Bank files",
		Fixable: "map",
	}, true
}

// gitignoreCheck (D-04) only speaks up inside a repository — without
// .git there is nothing to ignore yet (the Git check says so).
func gitignoreCheck(root string) (Check, bool) {
	if !ex(filepath.Join(root, ".git")) {
		return Check{}, false
	}
	return Check{
		ID: "gitignore", Name: ".gitignore", Severity: "warn", Weight: 0,
		Detail:  ".gitignore present",
		OK:      ex(filepath.Join(root, ".gitignore")),
		Finding: ".gitignore missing — secrets and build output may be committed",
		Remedy:  "add a .gitignore listing secrets and build output",
	}, true
}

// dirtyTreeCheck (D-04) runs `git status --porcelain` — arguments
// only, never a shell (safety rule 7), with a bounded timeout so a
// stuck index cannot hang the report. Omitted when there is no
// repository or git itself fails, rather than guessing.
func dirtyTreeCheck(root string) (Check, bool) {
	if !ex(filepath.Join(root, ".git")) {
		return Check{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain").CombinedOutput()
	if err != nil {
		return Check{}, false
	}
	c := Check{
		ID: "dirty_tree", Name: "Dirty tree", Severity: "warn", Weight: 0,
		Detail: "working tree clean", OK: true,
	}
	if len(bytes.TrimSpace(out)) > 0 {
		c.OK = false
		c.Finding = "uncommitted changes"
		c.Remedy = "commit or stash your work so the Map matches git"
	}
	return c, true
}

// Run scores a project. scaffoldMark is the bridge tool's marker file name
// ("" disables the exclusivity check); when the marker and .git coexist but
// the Bank claims rivu ran init, that is a possible double-init (spec 1.7).
// staleDays is the flow.stale_threshold_days config (<1 disables the check).
// A missing folder short-circuits: MISSING instead of a pile of failures (D-07).
func Run(p registry.Project, scaffoldMark string, staleDays int) Report {
	root := p.Path
	if !ex(root) {
		return Report{
			Project: p, Missing: true, Score: 0,
			Checks: []Check{onDiskCheck(root)},
		}
	}
	checks := []Check{
		{
			ID: "readme", Name: "README", Severity: "warn", Weight: 20,
			Detail:  "project documentation",
			OK:      ex(filepath.Join(root, "README.md")) || ex(filepath.Join(root, "README")),
			Finding: "README.md missing",
			Remedy:  "run `rivu doctor {slug} --fix readme --yes`",
			Fixable: "readme",
		},
		{
			ID: "git", Name: "Git", Severity: "warn", Weight: 15,
			Detail:  "version control",
			OK:      ex(filepath.Join(root, ".git")),
			Finding: ".git missing — not a repository",
			Remedy:  "run `git init` in the project folder",
		},
		{
			ID: "bank", Name: "Bank", Severity: "warn", Weight: 15,
			Detail:  "Rivu metadata",
			OK:      ex(filepath.Join(root, ".metadata", "project.toml")),
			Finding: ".metadata/project.toml missing",
			Remedy:  "run `rivu agent sync {slug}` to rebuild Bank and Map",
			Fixable: "map",
		},
		{
			ID: "map", Name: "Map", Severity: "warn", Weight: 15,
			Detail:  "agent-readable map",
			OK:      ex(filepath.Join(root, ".metadata", "agent", "PROJECT_MAP.md")),
			Finding: ".metadata/agent/PROJECT_MAP.md missing",
			Remedy:  "run `rivu agent sync {slug}` to rebuild Bank and Map",
			Fixable: "map",
		},
		testsCheck(root),
		{
			ID: "ci", Name: "CI", Severity: "warn", Weight: 10,
			Detail:  "continuous integration",
			OK:      ex(filepath.Join(root, ".github", "workflows")),
			Finding: "no workflow under .github/workflows",
			Remedy:  "add a workflow under .github/workflows",
		},
		{
			ID: "license", Name: "License", Severity: "warn", Weight: 10,
			Detail:  "license file",
			OK:      ex(filepath.Join(root, "LICENSE")) || ex(filepath.Join(root, "LICENSE.md")),
			Finding: "LICENSE missing",
			Remedy:  "add a LICENSE file",
		},
		depCheck(root),
		onDiskCheck(root),
	}
	if c, ok := staleCheck(p, staleDays); ok {
		checks = append(checks, c)
	}
	if c, ok := bankIncompleteCheck(root); ok {
		checks = append(checks, c)
	}
	if scaffoldMark != "" {
		doubleInit := ex(filepath.Join(root, ".git")) &&
			ex(filepath.Join(root, scaffoldMark)) &&
			gitInitOwner(root) == "rivu"
		checks = append(checks, Check{
			ID: "git_exclusivity", Name: "Git exclusivity", Severity: "warn",
			OK:      !doubleInit,
			Weight:  0, // informational: never changes the score
			Detail:  "single git owner",
			Finding: "possible double-init, verify history (.git + bridge marker + git_init_owner=rivu)",
			Remedy:  "verify history (.git + bridge marker + git_init_owner=rivu)",
		})
	}
	if c, ok := gitignoreCheck(root); ok {
		checks = append(checks, c)
	}
	if c, ok := dirtyTreeCheck(root); ok {
		checks = append(checks, c)
	}
	score := 0
	for i := range checks {
		c := &checks[i]
		if c.OK {
			// a passing check reports its Detail, never failure text
			// left over from the template (D-03)
			c.Finding = ""
			c.Remedy = ""
			score += c.Weight
		}
	}
	return Report{Project: p, Checks: checks, Score: score}
}
