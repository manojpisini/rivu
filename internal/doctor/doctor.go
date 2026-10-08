package doctor

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/registry"
)

// Check is one health check. Detail describes the passing state (and
// is what --json reports). When a check fails, Finding says exactly
// what is wrong and Remedy says what to do about it (D-03, P6.03).
// Remedy may contain {slug}, filled in by the caller; Fixable names
// the safe `rivu doctor --fix` id when a machine can apply the
// remedy without a product decision.
type Check struct {
	Name    string
	OK      bool
	Weight  int
	Detail  string
	Finding string
	Remedy  string
	Fixable string
}

type Report struct {
	Project registry.Project
	Checks  []Check
	Score   int
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
		Name: "Tests", Weight: 10, Detail: "tests detected",
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
				Name: "Dependencies", Weight: 5,
				Detail:  "manifest without lockfile: " + d.manifest,
				OK:      false,
				Finding: "manifest without lockfile: " + d.manifest,
				Remedy:  d.tidy,
			}
		}
	}
	if !tracked {
		return Check{
			Name: "Dependencies", Weight: 5, OK: true,
			Detail: "no dependency manifest",
		}
	}
	return Check{
		Name: "Dependencies", Weight: 5, OK: true,
		Detail: "manifest and lockfile present",
	}
}

// Run scores a project. scaffoldMark is the bridge tool's marker file name
// ("" disables the exclusivity check); when the marker and .git coexist but
// the Bank claims rivu ran init, that is a possible double-init (spec 1.7).
func Run(p registry.Project, scaffoldMark string) Report {
	root := p.Path
	checks := []Check{
		{
			Name: "README", Weight: 20, Detail: "project documentation",
			OK:      ex(filepath.Join(root, "README.md")) || ex(filepath.Join(root, "README")),
			Finding: "README.md missing",
			Remedy:  "run `rivu doctor {slug} --fix readme --yes`",
			Fixable: "readme",
		},
		{
			Name: "Git", Weight: 15, Detail: "version control",
			OK:      ex(filepath.Join(root, ".git")),
			Finding: ".git missing — not a repository",
			Remedy:  "run `git init` in the project folder",
		},
		{
			Name: "Bank", Weight: 15, Detail: "Rivu metadata",
			OK:      ex(filepath.Join(root, ".metadata", "project.toml")),
			Finding: ".metadata/project.toml missing",
			Remedy:  "run `rivu agent sync {slug}` to rebuild Bank and Map",
			Fixable: "map",
		},
		{
			Name: "Map", Weight: 15, Detail: "agent-readable map",
			OK:      ex(filepath.Join(root, ".metadata", "agent", "PROJECT_MAP.md")),
			Finding: ".metadata/agent/PROJECT_MAP.md missing",
			Remedy:  "run `rivu agent sync {slug}` to rebuild Bank and Map",
			Fixable: "map",
		},
		testsCheck(root),
		{
			Name: "CI", Weight: 10, Detail: "continuous integration",
			OK:      ex(filepath.Join(root, ".github", "workflows")),
			Finding: "no workflow under .github/workflows",
			Remedy:  "add a workflow under .github/workflows",
		},
		{
			Name: "License", Weight: 10, Detail: "license file",
			OK:      ex(filepath.Join(root, "LICENSE")) || ex(filepath.Join(root, "LICENSE.md")),
			Finding: "LICENSE missing",
			Remedy:  "add a LICENSE file",
		},
		depCheck(root),
	}
	if scaffoldMark != "" {
		doubleInit := ex(filepath.Join(root, ".git")) &&
			ex(filepath.Join(root, scaffoldMark)) &&
			gitInitOwner(root) == "rivu"
		checks = append(checks, Check{
			Name:    "Git exclusivity",
			OK:      !doubleInit,
			Weight:  0, // informational: never changes the score
			Detail:  "single git owner",
			Finding: "possible double-init, verify history (.git + bridge marker + git_init_owner=rivu)",
			Remedy:  "verify history (.git + bridge marker + git_init_owner=rivu)",
		})
	}
	score := 0
	for _, c := range checks {
		if c.OK {
			score += c.Weight
		}
	}
	return Report{Project: p, Checks: checks, Score: score}
}
