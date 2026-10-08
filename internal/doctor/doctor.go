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

type Check struct {
	Name   string
	OK     bool
	Weight int
	Detail string
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
	if hasTests(root) {
		return Check{"Tests", true, 10, "tests detected"}
	}
	return Check{"Tests", false, 10, "no tests detected"}
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
}{
	{"go.mod", []string{"go.sum"}},
	{"package.json", []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "bun.lockb"}},
	{"Cargo.toml", []string{"Cargo.lock"}},
	{"pyproject.toml", []string{"poetry.lock", "uv.lock"}},
	{"Pipfile", []string{"Pipfile.lock"}},
	{"Gemfile", []string{"Gemfile.lock"}},
	{"composer.json", []string{"composer.lock"}},
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
			return Check{"Dependencies", false, 5, "manifest without lockfile: " + d.manifest}
		}
	}
	if !tracked {
		return Check{"Dependencies", true, 5, "no dependency manifest"}
	}
	return Check{"Dependencies", true, 5, "manifest and lockfile present"}
}

// Run scores a project. scaffoldMark is the bridge tool's marker file name
// ("" disables the exclusivity check); when the marker and .git coexist but
// the Bank claims rivu ran init, that is a possible double-init (spec 1.7).
func Run(p registry.Project, scaffoldMark string) Report {
	checks := []Check{{"README", ex(filepath.Join(p.Path, "README.md")) || ex(filepath.Join(p.Path, "README")), 20, "project documentation"}, {"Git", ex(filepath.Join(p.Path, ".git")), 15, "version control"}, {"Bank", ex(filepath.Join(p.Path, ".metadata", "project.toml")), 15, "Rivu metadata"}, {"Map", ex(filepath.Join(p.Path, ".metadata", "agent", "PROJECT_MAP.md")), 15, "agent-readable map"}, testsCheck(p.Path), {"CI", ex(filepath.Join(p.Path, ".github", "workflows")), 10, "continuous integration"}, {"License", ex(filepath.Join(p.Path, "LICENSE")) || ex(filepath.Join(p.Path, "LICENSE.md")), 10, "license file"}, depCheck(p.Path)}
	if scaffoldMark != "" {
		doubleInit := ex(filepath.Join(p.Path, ".git")) &&
			ex(filepath.Join(p.Path, scaffoldMark)) &&
			gitInitOwner(p.Path) == "rivu"
		checks = append(checks, Check{
			Name:   "Git exclusivity",
			OK:     !doubleInit,
			Weight: 0, // informational: never changes the score
			Detail: "possible double-init, verify history (.git + bridge marker + git_init_owner=rivu)",
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
