package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

// TestEachCheck (D-08): one table, one case per check — the fixture
// decides pass/fail, the finding names exactly what is missing, and
// every failure carries a remedy.
func TestEachCheck(t *testing.T) {
	cases := []struct {
		name    string
		dirs    []string
		files   []string
		check   string
		wantOK  bool
		finding string
	}{
		{"README present", nil, []string{"README.md"}, "README", true, ""},
		{"README missing", nil, nil, "README", false, "README.md missing"},
		{"Git present", []string{".git"}, nil, "Git", true, ""},
		{"Git missing", nil, nil, "Git", false, ".git missing — not a repository"},
		{"Bank present", []string{".metadata"}, []string{".metadata/project.toml"}, "Bank", true, ""},
		{"Bank missing", nil, nil, "Bank", false, ".metadata/project.toml missing"},
		{"Map present", []string{".metadata/agent"}, []string{".metadata/agent/PROJECT_MAP.md"}, "Map", true, ""},
		{"Map missing", nil, nil, "Map", false, ".metadata/agent/PROJECT_MAP.md missing"},
		{"Tests present", []string{"test"}, nil, "Tests", true, ""},
		{"Tests missing", nil, nil, "Tests", false, "no tests detected"},
		{"CI present", []string{".github/workflows"}, nil, "CI", true, ""},
		{"CI missing", nil, nil, "CI", false, "no workflow under .github/workflows"},
		{"License present", nil, []string{"LICENSE"}, "License", true, ""},
		{"License missing", nil, nil, "License", false, "LICENSE missing"},
		{"Deps locked", nil, []string{"go.mod", "go.sum"}, "Dependencies", true, ""},
		{"Deps unlocked", nil, []string{"go.mod"}, "Dependencies", false, "manifest without lockfile: go.mod"},
		{"On disk", nil, nil, "On disk", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			for _, dir := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(d, dir), 0755); err != nil {
					t.Fatal(err)
				}
			}
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(d, f), []byte("x"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			r := Run(registry.Project{Path: d, CreatedAt: time.Now()}, "", 45, nil)
			var got Check
			found := false
			for _, c := range r.Checks {
				if c.Name == tc.check {
					got, found = c, true
					break
				}
			}
			if !found {
				t.Fatalf("no %s check in report", tc.check)
			}
			if got.OK != tc.wantOK {
				t.Errorf("OK = %v (finding %q), want %v", got.OK, got.Finding, tc.wantOK)
			}
			if !tc.wantOK {
				if got.Finding != tc.finding {
					t.Errorf("finding = %q, want %q", got.Finding, tc.finding)
				}
				if got.Remedy == "" {
					t.Error("failing check needs a remedy")
				}
			} else if got.Finding != "" || got.Remedy != "" {
				t.Errorf("passing check carries failure text: %+v", got)
			}
		})
	}
}

func TestCheckWeightsSumTo100(t *testing.T) {
	r := Run(registry.Project{Path: t.TempDir()}, "", 45, nil)
	sum := 0
	for _, c := range r.Checks {
		sum += c.Weight
	}
	if sum != 100 {
		t.Errorf("check weights sum to %d, want 100", sum)
	}
}

func TestDoubleInitCheck(t *testing.T) {
	d := t.TempDir()
	for _, p := range []string{".git", ".lode"} {
		if err := os.MkdirAll(filepath.Join(d, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeOwner := func(owner string) {
		body := "[rivu]\ngit_init_owner = \"" + owner + "\"\n"
		if err := os.MkdirAll(filepath.Join(d, ".metadata"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, ".metadata", "project.toml"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	find := func(r Report, name string) (Check, bool) {
		for _, c := range r.Checks {
			if c.Name == name {
				return c, true
			}
		}
		return Check{}, false
	}

	writeOwner("rivu")
	c, ok := find(Run(registry.Project{Path: d}, ".lode", 45, nil), "Git exclusivity")
	if !ok {
		t.Fatal("exclusivity check missing")
	}
	if c.OK {
		t.Error("double-init not flagged with owner=rivu + marker + .git")
	}

	writeOwner("bridge")
	if c, _ = find(Run(registry.Project{Path: d}, ".lode", 45, nil), "Git exclusivity"); !c.OK {
		t.Error("owner=bridge must not flag double-init")
	}

	if _, ok = find(Run(registry.Project{Path: d}, "", 45, nil), "Git exclusivity"); ok {
		t.Error("check must be absent when no scaffold marker configured")
	}
}

// TestTestDetection (D-02): real signals — suites, test file
// patterns, configured runners — not the old tests/internal proxy.
func TestTestDetection(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string // path -> content; a path ending in / is a dir marker
		want  bool
	}{
		{"empty project", nil, false},
		{"go tests in internal", map[string]string{"internal/foo_test.go": "package x"}, true},
		{"go tests nested", map[string]string{"pkg/db/foo_test.go": "package db"}, true},
		{"tests dir", map[string]string{"tests/README": "# tests"}, true},
		{"jest suite dir", map[string]string{"__tests__/a.spec.ts": ""}, true},
		{"spec file", map[string]string{"src/login.spec.ts": ""}, true},
		{"test file", map[string]string{"src/widget.test.js": ""}, true},
		{"pytest.ini", map[string]string{"pytest.ini": ""}, true},
		{"pyproject with pytest", map[string]string{"pyproject.toml": "[tool.pytest.ini_options]\naddopts = '-q'"}, true},
		{"pyproject without pytest", map[string]string{"pyproject.toml": "[project]\nname = 'x'"}, false},
		{"npm test script", map[string]string{"package.json": `{"scripts":{"test":"vitest run"}}`}, true},
		{"npm without test script", map[string]string{"package.json": `{"scripts":{"build":"tsc"}}`}, false},
		{"rust inline tests", map[string]string{"src/lib.rs": "#[cfg(test)]\nmod t {}"}, true},
		{"rust without tests", map[string]string{"src/lib.rs": "fn a() {}"}, false},
		{"test inside node_modules ignored", map[string]string{"node_modules/dep/x.test.js": ""}, false},
		{"internal without tests is not enough", map[string]string{"internal/models.go": "package models"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			for path, content := range tc.files {
				full := filepath.Join(d, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			c := testsCheck(d)
			if c.OK != tc.want {
				t.Errorf("OK=%v (%s), want %v", c.OK, c.Detail, tc.want)
			}
			if c.Name != "Tests" {
				t.Errorf("check = %+v, want Tests", c)
			}
		})
	}
}

// TestFailingChecksCarryFindingAndRemedy (D-03): every failing check
// says what is wrong and what to do; passing checks keep their label.
func TestFailingChecksCarryFindingAndRemedy(t *testing.T) {
	r := Run(registry.Project{Path: t.TempDir()}, "", 45, nil)
	for _, c := range r.Checks {
		if c.OK {
			if c.Finding != "" {
				t.Errorf("%s passed but has finding %q", c.Name, c.Finding)
			}
			continue
		}
		if c.Finding == "" {
			t.Errorf("%s failed without a finding (detail %q)", c.Name, c.Detail)
		}
		if c.Remedy == "" {
			t.Errorf("%s failed without a remedy", c.Name)
		}
	}
	// A concrete example: README failing names the file and the fix.
	for _, c := range r.Checks {
		if c.Name == "README" && !c.OK {
			if c.Finding != "README.md missing" || c.Fixable != "readme" {
				t.Errorf("README = %+v, want finding README.md missing and fixable readme", c)
			}
		}
	}
}

// TestDependencyCheck (D-01): every known manifest must ship a
// lockfile; projects with no tracked manifest pass vacuously.
func TestDependencyCheck(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"no manifest", nil, true},
		{"go locked", []string{"go.mod", "go.sum"}, true},
		{"go unlocked", []string{"go.mod"}, false},
		{"npm via yarn", []string{"package.json", "yarn.lock"}, true},
		{"npm unlocked", []string{"package.json"}, false},
		{"cargo locked", []string{"Cargo.toml", "Cargo.lock"}, true},
		{"cargo unlocked", []string{"Cargo.toml"}, false},
		{"python unlocked", []string{"pyproject.toml"}, false},
		{"python via uv", []string{"pyproject.toml", "uv.lock"}, true},
		{"maven has no lock convention", []string{"pom.xml"}, true},
		{"one locked one not", []string{"go.mod", "go.sum", "package.json"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(d, f), []byte("x"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			c := depCheck(d)
			if c.OK != tc.want {
				t.Errorf("OK=%v (%s), want %v", c.OK, c.Detail, tc.want)
			}
			if c.Name != "Dependencies" {
				t.Errorf("check = %+v, want Dependencies", c)
			}
		})
	}

	// The score consequence: an unlocked manifest costs the +5.
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	base := Run(registry.Project{Path: d}, "", 45, nil).Score
	if err := os.WriteFile(filepath.Join(d, "go.sum"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Run(registry.Project{Path: d}, "", 45, nil).Score; got != base+5 {
		t.Errorf("lockfile adds %d to score, want +5 (base %d, got %d)", got-base, base, got)
	}
}

// TestRunShortCircuitsMissingFolder (D-07): a registered folder that
// is gone reports MISSING — one failing check, score 0, not a pile-up
// of failures that reads like 5/100.
func TestRunShortCircuitsMissingFolder(t *testing.T) {
	r := Run(registry.Project{Path: filepath.Join(t.TempDir(), "gone")}, "", 45, nil)
	if !r.Missing {
		t.Fatal("want Missing short-circuit")
	}
	if r.Score != 0 {
		t.Errorf("score = %d, want 0", r.Score)
	}
	if len(r.Checks) != 1 {
		t.Fatalf("%d checks, want only On disk", len(r.Checks))
	}
	c := r.Checks[0]
	if c.ID != "on_disk" || c.OK || c.Severity != "error" || c.Weight != 0 {
		t.Errorf("check = %+v, want failing on_disk error weight 0", c)
	}
	if c.Finding == "" || c.Remedy == "" {
		t.Error("missing check needs a finding and a remedy")
	}
}

// TestStaleCheck (D-04): activity older than the threshold fails,
// recent activity passes, a stale project reopened today is fresh
// again, and a threshold below 1 removes the check entirely.
func TestStaleCheck(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		p    registry.Project
		days int
		want bool
		ok   bool
	}{
		{"recent", registry.Project{CreatedAt: now.AddDate(0, 0, -3)}, 45, true, true},
		{"quiet", registry.Project{CreatedAt: now.AddDate(0, 0, -60)}, 45, false, true},
		{"reopened", registry.Project{CreatedAt: now.AddDate(0, 0, -60), LastOpenedAt: now}, 45, true, true},
		{"threshold off", registry.Project{CreatedAt: now.AddDate(0, 0, -60)}, 0, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, ok := staleCheck(tc.p, tc.days)
			if ok != tc.ok {
				t.Fatalf("present = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if c.OK != tc.want {
				t.Errorf("OK = %v (%s), want %v", c.OK, c.Finding, tc.want)
			}
			if c.ID != "stale" || c.Weight != 0 {
				t.Errorf("check = %+v, want stale weight 0", c)
			}
		})
	}
}

// TestBankIncompleteCheck (D-04): project.toml present but the human
// docs missing fails and names them; a complete Bank passes; without
// project.toml the Bank check carries it and this one stays home.
func TestBankIncompleteCheck(t *testing.T) {
	d := t.TempDir()
	if _, ok := bankIncompleteCheck(d); ok {
		t.Fatal("no Bank at all must omit the check (Bank check owns it)")
	}
	meta := filepath.Join(d, ".metadata")
	if err := os.MkdirAll(meta, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "project.toml"), []byte("[rivu]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	c, ok := bankIncompleteCheck(d)
	if !ok || c.OK {
		t.Fatalf("ok=%v check=%+v, want failing bank_incomplete", ok, c)
	}
	for _, want := range []string{"overview.md", "decisions.md", "tasks.md"} {
		if !strings.Contains(c.Finding, want) {
			t.Errorf("finding %q must name %s", c.Finding, want)
		}
	}
	if c.Fixable != "map" {
		t.Errorf("fixable = %q, want map", c.Fixable)
	}
	for _, f := range []string{"overview.md", "decisions.md", "tasks.md"} {
		if err := os.WriteFile(filepath.Join(meta, f), []byte("# x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if c, ok = bankIncompleteCheck(d); !ok || !c.OK {
		t.Errorf("complete Bank: ok=%v check=%+v, want passing", ok, c)
	}
}

// TestGitRepoChecks (D-04): inside a repository .gitignore and
// dirty-tree appear — dirty after an edit, clean after a commit —
// and neither check exists without .git.
func TestGitRepoChecks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	if _, ok := dirtyTreeCheck(t.TempDir()); ok {
		t.Error("no .git → dirty-tree check must stay home")
	}
	if _, ok := gitignoreCheck(t.TempDir()); ok {
		t.Error("no .git → .gitignore check must stay home")
	}
	d := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		full := append([]string{"-C", d}, args...)
		out, err := exec.Command("git", full...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("-c", "user.email=rivu@test", "-c", "user.name=rivu", "add", ".")
	if c, ok := gitignoreCheck(d); !ok || c.OK {
		t.Errorf("fresh repo without .gitignore: ok=%v c=%+v, want failing", ok, c)
	}
	if err := os.WriteFile(filepath.Join(d, ".gitignore"), []byte("*.log\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if c, ok := gitignoreCheck(d); !ok || !c.OK {
		t.Errorf(".gitignore written: ok=%v c=%+v, want passing", ok, c)
	}
	git("add", ".")
	git("-c", "user.email=rivu@test", "-c", "user.name=rivu", "commit", "-q", "-m", "gitignore")
	if c, ok := dirtyTreeCheck(d); !ok || !c.OK {
		t.Errorf("committed: ok=%v c=%+v, want clean", ok, c)
	}
	if err := os.WriteFile(filepath.Join(d, "scratch.txt"), []byte("wip"), 0644); err != nil {
		t.Fatal(err)
	}
	c, ok := dirtyTreeCheck(d)
	if !ok || c.OK {
		t.Fatalf("untracked file: ok=%v c=%+v, want dirty", ok, c)
	}
	if c.Remedy == "" || c.Finding == "" {
		t.Error("dirty tree needs finding and remedy")
	}
	git("-c", "user.email=rivu@test", "-c", "user.name=rivu", "add", ".")
	git("-c", "user.email=rivu@test", "-c", "user.name=rivu", "commit", "-q", "-m", "wip")
	if c, ok := dirtyTreeCheck(d); !ok || !c.OK {
		t.Errorf("committed: ok=%v c=%+v, want clean", ok, c)
	}
}

// TestConfigWeightsDriveScore (D-05): [health].weights decides every
// score contribution — a supplied map scores exactly the keys it
// names, nil falls back to the spec defaults summing to 100.
func TestConfigWeightsDriveScore(t *testing.T) {
	withReadme := t.TempDir()
	if err := os.WriteFile(filepath.Join(withReadme, "README.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	w := map[string]int{"readme": 100}
	r := Run(registry.Project{Path: withReadme}, "", 45, w)
	if r.Score != 100 {
		t.Errorf("score = %d, want 100 from weights {readme:100}", r.Score)
	}
	for _, c := range r.Checks {
		want := 0
		if c.ID == "readme" {
			want = 100
		}
		if c.Weight != want {
			t.Errorf("%s weight = %d, want %d from config", c.Name, c.Weight, want)
		}
	}
	if got := Run(registry.Project{Path: t.TempDir()}, "", 45, w).Score; got != 0 {
		t.Errorf("score without README = %d, want 0", got)
	}
	// nil → spec defaults: README 20 + vacuous deps 5
	if got := Run(registry.Project{Path: withReadme}, "", 45, nil).Score; got != 25 {
		t.Errorf("nil weights score = %d, want spec defaults (20 README + 5 deps)", got)
	}
}

// TestCheckIDsAreUniqueAndTyped (D-04): every check carries a stable
// ID (what [health].weights keys against) and a severity.
func TestCheckIDsAreUniqueAndTyped(t *testing.T) {
	r := Run(registry.Project{Path: t.TempDir(), CreatedAt: time.Now()}, "", 45, nil)
	seen := map[string]bool{}
	for _, c := range r.Checks {
		if c.ID == "" {
			t.Errorf("%s has no ID", c.Name)
		}
		if seen[c.ID] {
			t.Errorf("duplicate ID %q", c.ID)
		}
		seen[c.ID] = true
		if c.Severity != "error" && c.Severity != "warn" {
			t.Errorf("%s severity = %q, want error or warn", c.Name, c.Severity)
		}
	}
	for _, id := range []string{"readme", "git", "bank", "map", "tests", "ci", "license", "deps_fresh", "on_disk", "stale"} {
		if !seen[id] {
			t.Errorf("missing check id %q", id)
		}
	}
}
