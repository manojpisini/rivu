package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/registry"
)

func TestScanDetectsGoProject(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "01_Active", "demo")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FlowStage != "active" || got[0].Language != "Go" {
		t.Fatalf("unexpected: %#v", got)
	}
	if got[0].Root != filepath.Clean(root) {
		t.Errorf("Root = %q, want %q", got[0].Root, filepath.Clean(root))
	}
}

func TestClassifyMarkers(t *testing.T) {
	for _, tc := range []struct {
		files  []string
		lang   string
		stack  string
		score  int
		strong bool
	}{
		{[]string{"go.mod"}, "Go", "go", 3, true},
		{[]string{"Cargo.toml"}, "Rust", "rust", 3, true},
		{[]string{"requirements.txt"}, "Python", "python", 3, true},
		{[]string{"setup.py"}, "Python", "python", 3, true},
		{[]string{"package.json"}, "JavaScript", "node", 3, true},
		{[]string{"deno.json"}, "JavaScript/TypeScript", "deno", 3, true},
		{[]string{"pom.xml"}, "Java", "java", 3, true},
		{[]string{"build.gradle"}, "Java", "gradle", 3, true},
		{[]string{"build.gradle.kts"}, "Kotlin", "gradle", 3, true},
		{[]string{"App.sln"}, "C#", "dotnet", 3, true},
		{[]string{"App.csproj"}, "C#", "dotnet", 3, true},
		{[]string{"Gemfile"}, "Ruby", "ruby", 3, true},
		{[]string{"composer.json"}, "PHP", "php", 3, true},
		{[]string{"mix.exs"}, "Elixir", "elixir", 3, true},
		{[]string{"CMakeLists.txt"}, "C/C++", "cmake", 3, true},
		{[]string{"Package.swift"}, "Swift", "swift", 3, true},
		{[]string{"Dockerfile"}, "", "docker", 1, false},
		{[]string{".git"}, "", "", 3, true},
		{[]string{"notes.txt"}, "", "", 0, false},
		{[]string{"go.mod", "Dockerfile"}, "Go", "go,docker", 4, true},
	} {
		name := tc.files[0]
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			for _, f := range tc.files {
				p := filepath.Join(d, f)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			lang, stack, score, strong := classify(d)
			if lang != tc.lang || strong != tc.strong || score != tc.score || strings.Join(stack, ",") != tc.stack {
				t.Errorf("classify = (%q, %v, %d, %v), want (%q, %v, %d, %v)", lang, stack, score, strong, tc.lang, tc.stack, tc.score, tc.strong)
			}
		})
	}
}

// TestWeakMarkerScoring (S-02): weak markers score 1 and never reach
// the threshold alone or in pairs; three weak markers register but are
// flagged unconfirmed through a scan warning.
func TestWeakMarkerScoring(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Below threshold: README only (1), README + src/ (2).
	write("00_Source/readme-only/README.md", "# hi\n")
	write("00_Source/readme-src/README.md", "# hi\n")
	write("00_Source/readme-src/src/main.go", "package main\n")
	// At threshold on weak markers only: README + src/ + Dockerfile (3).
	write("00_Source/weak-full/README.md", "# hi\n")
	write("00_Source/weak-full/src/main.go", "package main\n")
	write("00_Source/weak-full/Dockerfile", "FROM scratch\n")
	// Strong marker controls.
	write("00_Source/go-proj/go.mod", "module x\n")

	got, warns, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "go-proj,weak-full" {
		t.Errorf("projects = %v, want [go-proj weak-full]", names)
	}
	unconfirmed := false
	for _, w := range warns {
		if strings.Contains(w.Error(), "unconfirmed") && strings.Contains(w.Error(), "weak-full") {
			unconfirmed = true
		}
	}
	if !unconfirmed {
		t.Errorf("weak-only project not flagged unconfirmed, warnings = %v", warns)
	}
	for _, w := range warns {
		if strings.Contains(w.Error(), "go-proj") {
			t.Errorf("strong project must not be flagged: %v", w)
		}
	}
}

func TestScanSkipsDotAndJunkDirs(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{
		filepath.Join(root, ".Trash", "old-proj"),
		filepath.Join(root, "$RECYCLE.BIN", "recycled"),
		filepath.Join(root, "System Volume Information", "svi"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("junk/dot dirs must be skipped, found %#v", got)
	}
}

func TestScanKeepsMetadataDir(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, ".metadata")
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module m"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf(".metadata is the documented dot-dir exception, found %d", len(got))
	}
}

func TestScanHonorsRivuignore(t *testing.T) {
	root := t.TempDir()
	ignore := "# skip noise\nbuild/\ntemp*\n/deep-skip\nmiddle/\n"
	if err := os.WriteFile(filepath.Join(root, ".rivuignore"), []byte(ignore), 0644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"build", "temporal", "a/middle", "deep-skip", "src/deep-skip"} {
		p := filepath.Join(root, filepath.FromSlash(d))
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	// "build/", "temp*", "middle/" match any depth; "/deep-skip" is
	// anchored to root, so src/deep-skip must still be discovered.
	if len(got) != 1 || got[0].Name != "deep-skip" ||
		!strings.HasSuffix(got[0].Path, filepath.Join("src", "deep-skip")) {
		var names []string
		for _, p := range got {
			names = append(names, p.Path)
		}
		t.Errorf(".rivuignore not honored, discovered: %v", names)
	}
}

func TestScanMaxDepth(t *testing.T) {
	root := t.TempDir()
	shallow := filepath.Join(root, "01_Active", "one")
	deep := filepath.Join(root, "01_Active", "two", "three")
	for _, d := range []string{shallow, deep} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// depth of one = channel folders only, two = projects inside them.
	got, _, err := New(nil, 1).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("depth 1 must not reach projects, found %d", len(got))
	}
	got, _, err = New(nil, 2).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "one" {
		t.Errorf("depth 2 should find only the shallow project, got %#v", got)
	}
}

func TestScanHonorsConfigIgnore(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "scratch")
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module x"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _, err := New([]string{"scratch"}, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("config ignore names must be skipped, found %#v", got)
	}
}

func TestScanDupSlugDisambiguated(t *testing.T) {
	root := t.TempDir()
	for _, ch := range []string{"00_Source", "01_Active"} {
		d := filepath.Join(root, ch, "demo")
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "go.mod"), []byte("module demo"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}
	r, err := registry.Open(filepath.Join(t.TempDir(), "rivu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if warns, err := r.ApplyDiscovery(got); err != nil || len(warns) != 0 {
		t.Fatalf("ApplyDiscovery: warns=%v err=%v", warns, err)
	}
	list, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	slugs := map[string]bool{}
	for _, p := range list {
		slugs[p.Slug] = true
	}
	if !slugs["demo"] || !slugs["demo-2"] {
		t.Errorf("duplicate folder names must get unique slugs, got %v", slugs)
	}
}

func TestScanAdoptsBankID(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "demo")
	meta := filepath.Join(p, ".metadata")
	if err := os.MkdirAll(meta, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "project.toml"),
		[]byte("[rivu]\nid = \"11111111-2222-3333-4444-555555555555\"\nname = \"demo\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 project, got %d", len(got))
	}
	if got[0].ID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("Bank ID not adopted, got %q", got[0].ID)
	}
}

func TestScanMalformedBankIDMintsFresh(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "demo")
	meta := filepath.Join(p, ".metadata")
	if err := os.MkdirAll(meta, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meta, "project.toml"), []byte("not = [valid"), 0644); err != nil {
		t.Fatal(err)
	}
	got, _, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID == "" {
		t.Fatalf("malformed Bank file must not block scan: %#v", got)
	}
}

func TestScanMissingRootErrors(t *testing.T) {
	if _, _, err := New(nil, 6).Scan(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("missing root must be an error, not zero projects")
	}
}

func TestScanFileRootErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New(nil, 6).Scan(p); err == nil {
		t.Error("file root must be an error")
	}
}

func TestScanCollectsWalkWarnings(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("directory permissions are not enforced on Windows")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0755) })
	good := filepath.Join(root, "demo")
	if err := os.Mkdir(good, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	got, warns, err := New(nil, 6).Scan(root)
	if err != nil {
		t.Fatalf("permission warning must not abort the scan: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("good project still discovered, got %d", len(got))
	}
	if len(warns) == 0 {
		t.Error("expected a warning for the unreadable directory")
	}
}

func TestScanContextCancelled(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "01_Active", "demo")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := New(nil, 6).ScanContext(ctx, root, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestScanContextProgressCountsDirs(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "01_Active", "demo")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "go.mod"), []byte("module demo"), 0644); err != nil {
		t.Fatal(err)
	}
	var counts []int
	got, _, err := New(nil, 6).ScanContext(context.Background(), root, func(dirs int) {
		counts = append(counts, dirs)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("projects = %d, want 1", len(got))
	}
	// root, 01_Active, demo at minimum
	if len(counts) < 3 || counts[len(counts)-1] < 3 {
		t.Fatalf("progress counts = %v, want at least 3 rising entries", counts)
	}
	for i := 1; i < len(counts); i++ {
		if counts[i] <= counts[i-1] {
			t.Fatalf("progress must strictly increase: %v", counts)
		}
	}
}

// TestNodeEcosystemDetection (S-04): TypeScript vs JavaScript by
// tsconfig.json or a typescript dependency, framework by package.json
// deps (framework leads the stack), package manager by lockfile.
func TestNodeEcosystemDetection(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		lang  string
		stack []string // must all appear, in this relative order
		front string   // optional: exact stack[0]
	}{
		{"tsconfig", map[string]string{"package.json": `{"name":"x"}`, "tsconfig.json": "{}"}, "TypeScript", []string{"node"}, ""},
		{"plain js", map[string]string{"package.json": `{"name":"x"}`}, "JavaScript", []string{"node"}, ""},
		{"ts dependency", map[string]string{"package.json": `{"devDependencies":{"typescript":"^5"}}`}, "TypeScript", []string{"node"}, ""},
		{"npm lockfile", map[string]string{"package.json": "{}", "package-lock.json": "{}"}, "JavaScript", []string{"node", "npm"}, ""},
		{"yarn lockfile", map[string]string{"package.json": "{}", "yarn.lock": "x"}, "JavaScript", []string{"node", "yarn"}, ""},
		{"pnpm lockfile", map[string]string{"package.json": "{}", "pnpm-lock.yaml": "x"}, "JavaScript", []string{"node", "pnpm"}, ""},
		{"bun lockfile", map[string]string{"package.json": "{}", "bun.lockb": "x"}, "JavaScript", []string{"node", "bun"}, ""},
		{"next", map[string]string{"package.json": `{"dependencies":{"next":"14","react":"18"}}`}, "JavaScript", []string{"node"}, "next"},
		{"astro leads vite", map[string]string{"package.json": `{"dependencies":{"astro":"3","vite":"5"}}`, "bun.lockb": "x"}, "JavaScript", []string{"node", "bun"}, "astro"},
		{"vite only", map[string]string{"package.json": `{"devDependencies":{"vite":"5"}}`}, "JavaScript", []string{"node"}, "vite"},
		{"deno untouched", map[string]string{"deno.json": "{}"}, "JavaScript/TypeScript", []string{"deno"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := t.TempDir()
			for f, body := range tc.files {
				p := filepath.Join(d, f)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			lang, stack, _, _ := classify(d)
			if lang != tc.lang {
				t.Errorf("lang = %q, want %q", lang, tc.lang)
			}
			last := -1
			for _, want := range tc.stack {
				i := slices.Index(stack, want)
				if i < 0 {
					t.Fatalf("stack %v missing %q", stack, want)
				}
				if i < last {
					t.Errorf("stack %v: %q out of order", stack, want)
				}
				last = i
			}
			if tc.front != "" && (len(stack) == 0 || stack[0] != tc.front) {
				t.Errorf("stack = %v, want %q leading", stack, tc.front)
			}
		})
	}
}
