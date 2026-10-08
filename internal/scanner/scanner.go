package scanner

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/slug"
)

type Scanner struct {
	Ignore   map[string]bool
	MaxDepth int
}

func New(ignore []string, max int) *Scanner {
	m := map[string]bool{}
	for _, v := range ignore {
		m[v] = true
	}
	return &Scanner{m, max}
}
func exists(p string) bool { _, e := os.Stat(p); return e == nil }

// bankID returns the ID stored in the Bank's project.toml, if present and
// parseable. Adopting it keeps the ID stable across registry loss and lets
// a moved project keep its identity (R-10). Empty means "mint a new one".
func bankID(path string) string {
	b, err := os.ReadFile(filepath.Join(path, ".metadata", "project.toml"))
	if err != nil {
		return ""
	}
	var f struct {
		Rivu struct {
			ID string `toml:"id"`
		} `toml:"rivu"`
	}
	if err := toml.Unmarshal(b, &f); err != nil {
		return ""
	}
	return f.Rivu.ID
}

// threshold is the score needed to treat a directory as a project
// (S-02): every strong marker scores 3, weak markers 1, so one strong
// marker always qualifies while weak ones need three (README + src/ +
// a build file). Weak-only discoveries register but are flagged
// unconfirmed through a scan warning.
const threshold = 3

// marker is one detection rule: a file (or "*.<ext>" glob) in the project
// root that signals an ecosystem. First marker with a language wins for
// lang; stack tokens are deduplicated; strong markers score 3, weak 1.
type marker struct {
	name   string
	lang   string
	stack  string
	strong bool
}

var markers = []marker{
	{"go.mod", "Go", "go", true},
	{"Cargo.toml", "Rust", "rust", true},
	{"pyproject.toml", "Python", "python", true},
	{"requirements.txt", "Python", "python", true},
	{"setup.py", "Python", "python", true},
	{"package.json", "JavaScript/TypeScript", "node", true},
	{"deno.json", "JavaScript/TypeScript", "deno", true},
	{"pom.xml", "Java", "java", true},
	{"build.gradle", "Java", "gradle", true},
	{"build.gradle.kts", "Kotlin", "gradle", true},
	{"*.sln", "C#", "dotnet", true},
	{"*.csproj", "C#", "dotnet", true},
	{"Gemfile", "Ruby", "ruby", true},
	{"composer.json", "PHP", "php", true},
	{"mix.exs", "Elixir", "elixir", true},
	{"CMakeLists.txt", "C/C++", "cmake", true},
	{"Package.swift", "Swift", "swift", true},
	{"Dockerfile", "", "docker", false},
	{"bun.lockb", "", "bun", false},
	{"pnpm-lock.yaml", "", "pnpm", false},
}

func markerPresent(root, name string) bool {
	if strings.HasPrefix(name, "*.") {
		matches, err := filepath.Glob(filepath.Join(root, name))
		return err == nil && len(matches) > 0
	}
	return exists(filepath.Join(root, name))
}

func classify(path string) (string, []string, int, bool) {
	var lang string
	var stack []string
	score := 0
	strong := false
	if exists(filepath.Join(path, ".git")) {
		score += threshold
		strong = true
	}
	if exists(filepath.Join(path, ".metadata", "project.toml")) {
		score += threshold
		strong = true
	}
	for _, m := range markers {
		if !markerPresent(path, m.name) {
			continue
		}
		if lang == "" && m.lang != "" {
			lang = m.lang
		}
		if m.stack != "" && !slices.Contains(stack, m.stack) {
			stack = append(stack, m.stack)
		}
		if m.strong {
			score += threshold
			strong = true
		} else {
			score++
		}
	}
	// Weak markers (S-02): README.md alone and a non-empty src/ alone
	// count 1 but are not sufficient on their own (spec 2.8).
	if exists(filepath.Join(path, "README.md")) {
		score++
	}
	if nonEmptyDir(filepath.Join(path, "src")) {
		score++
	}
	return lang, stack, score, strong
}

func nonEmptyDir(p string) bool {
	ents, err := os.ReadDir(p)
	return err == nil && len(ents) > 0
}

// junkDirs are OS noise that never contains user projects.
func isJunk(name string) bool {
	return strings.EqualFold(name, "$RECYCLE.BIN") ||
		strings.EqualFold(name, "System Volume Information")
}

// loadIgnorePatterns reads <root>/.rivuignore: a gitignore subset — blank
// lines, # comments, trailing / (dir-only), leading or embedded / (anchored
// to root), path.Match globs otherwise. ponytail: root-level file only, no
// ! negation and no ** support — add per-dir discovery if users ask.
func loadIgnorePatterns(root string) []string {
	b, err := os.ReadFile(filepath.Join(root, ".rivuignore"))
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// ignored reports whether a directory's root-relative path matches any
// .rivuignore pattern.
func ignored(patterns []string, rel string) bool {
	for _, p := range patterns {
		anchored := strings.HasPrefix(p, "/")
		p = strings.Trim(p, "/")
		if p == "" {
			continue
		}
		var ok bool
		if anchored || strings.Contains(p, "/") {
			ok, _ = path.Match(p, filepath.ToSlash(rel))
		} else {
			ok, _ = path.Match(p, filepath.Base(rel))
		}
		if ok {
			return true
		}
	}
	return false
}

// Scan walks root and returns discovered projects. A missing or non-directory
// root is an error; per-entry failures (permission denied, vanished files)
// are collected as warnings so one bad folder never aborts the scan.
func (s *Scanner) Scan(root string) ([]registry.Project, []error, error) {
	return s.ScanContext(context.Background(), root, nil)
}

// ScanContext is Scan with cancellation: the walk stops at the next
// directory once ctx is done (returning ctx.Err()), and progress fires
// with the running count of visited directories.
func (s *Scanner) ScanContext(ctx context.Context, root string, progress func(dirs int)) ([]registry.Project, []error, error) {
	var out []registry.Project
	var warnings []error
	root = filepath.Clean(root)
	fi, err := os.Stat(root)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot scan workspace root: %w", err)
	}
	if !fi.IsDir() {
		return nil, nil, fmt.Errorf("workspace root %s is not a directory", root)
	}
	ignorePatterns := loadIgnorePatterns(root)
	dirs := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if err != nil {
			warnings = append(warnings, fmt.Errorf("skipped %s: %w", path, err))
			return nil
		}
		if d.IsDir() {
			dirs++
			if progress != nil {
				progress(dirs)
			}
		}
		rel, _ := filepath.Rel(root, path)
		depth := 0
		if rel != "." {
			depth = len(strings.Split(rel, string(os.PathSeparator)))
		}
		if d.IsDir() && path != root {
			name := d.Name()
			// Default-skip: dot-dirs (except .metadata), OS junk, config
			// ignore names, workspace .rivuignore patterns, over depth.
			if (strings.HasPrefix(name, ".") && name != ".metadata") ||
				isJunk(name) ||
				s.Ignore[name] ||
				ignored(ignorePatterns, rel) ||
				depth > s.MaxDepth {
				return filepath.SkipDir
			}
			sl, errSlug := slug.Make(name)
			if errSlug != nil {
				// Unsluggable folder names can never be registered safely.
				return filepath.SkipDir
			}
			lang, stack, score, strong := classify(path)
			if score >= threshold {
				channel := "00_Source"
				parts := strings.Split(rel, string(os.PathSeparator))
				if len(parts) > 1 {
					channel = parts[0]
				}
				id := bankID(path)
				if id == "" {
					id = uuid.NewString()
				}
				now := time.Now()
				out = append(out, registry.Project{ID: id, Name: name, Slug: sl, Path: path, Root: root, Channel: channel, FlowStage: registry.FlowForChannel(channel), Language: lang, Stack: stack, HasGit: exists(filepath.Join(path, ".git")), HasBank: exists(filepath.Join(path, ".metadata", "project.toml")), HasMap: exists(filepath.Join(path, ".metadata", "agent", "PROJECT_MAP.md")), CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: false})
				if !strong {
					warnings = append(warnings, fmt.Errorf("unconfirmed: %s reached the score threshold on weak markers only (README, src/, build file)", rel))
				}
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, warnings, err
	}
	return out, warnings, nil
}
