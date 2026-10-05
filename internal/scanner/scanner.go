package scanner

import (
	"fmt"
	"os"
	"path/filepath"
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
func classify(path string) (string, []string, bool) {
	var lang string
	var stack []string
	strong := false
	if exists(filepath.Join(path, "go.mod")) {
		lang = "Go"
		stack = append(stack, "go")
		strong = true
	}
	if exists(filepath.Join(path, "Cargo.toml")) {
		if lang == "" {
			lang = "Rust"
		}
		stack = append(stack, "rust")
		strong = true
	}
	if exists(filepath.Join(path, "pyproject.toml")) {
		if lang == "" {
			lang = "Python"
		}
		stack = append(stack, "python")
		strong = true
	}
	if exists(filepath.Join(path, "package.json")) {
		if lang == "" {
			lang = "JavaScript/TypeScript"
		}
		stack = append(stack, "node")
		strong = true
	}
	if exists(filepath.Join(path, ".git")) {
		strong = true
	}
	if exists(filepath.Join(path, ".metadata", "project.toml")) {
		strong = true
	}
	return lang, stack, strong
}

// Scan walks root and returns discovered projects. A missing or non-directory
// root is an error; per-entry failures (permission denied, vanished files)
// are collected as warnings so one bad folder never aborts the scan.
func (s *Scanner) Scan(root string) ([]registry.Project, []error, error) {
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
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Errorf("skipped %s: %w", path, err))
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		depth := 0
		if rel != "." {
			depth = len(strings.Split(rel, string(os.PathSeparator)))
		}
		if d.IsDir() && path != root {
			if s.Ignore[d.Name()] || depth > s.MaxDepth {
				return filepath.SkipDir
			}
			sl, errSlug := slug.Make(d.Name())
			if errSlug != nil {
				// Unsluggable folder names can never be registered safely.
				return filepath.SkipDir
			}
			lang, stack, strong := classify(path)
			if strong {
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
				out = append(out, registry.Project{ID: id, Name: d.Name(), Slug: sl, Path: path, Channel: channel, FlowStage: registry.FlowForChannel(channel), Language: lang, Stack: stack, HasGit: exists(filepath.Join(path, ".git")), HasBank: exists(filepath.Join(path, ".metadata", "project.toml")), HasMap: exists(filepath.Join(path, ".metadata", "agent", "PROJECT_MAP.md")), CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: false})
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
