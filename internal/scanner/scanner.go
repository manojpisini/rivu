package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

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
func (s *Scanner) Scan(root string) ([]registry.Project, error) {
	var out []registry.Project
	root = filepath.Clean(root)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
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
				now := time.Now()
				out = append(out, registry.Project{ID: uuid.NewString(), Name: d.Name(), Slug: sl, Path: path, Channel: channel, FlowStage: registry.FlowForChannel(channel), Language: lang, Stack: stack, HasGit: exists(filepath.Join(path, ".git")), HasBank: exists(filepath.Join(path, ".metadata", "project.toml")), HasMap: exists(filepath.Join(path, ".metadata", "agent", "PROJECT_MAP.md")), CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: false})
				return filepath.SkipDir
			}
		}
		return nil
	})
	return out, err
}
func StackJSON(v []string) string { b, _ := json.Marshal(v); return string(b) }
