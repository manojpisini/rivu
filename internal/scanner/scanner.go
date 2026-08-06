package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/manojpisini/rivu/internal/registry"
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
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
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
func flowFromChannel(c string) string {
	switch c {
	case "00_Source":
		return "source"
	case "01_Active":
		return "active"
	case "02_Maintenance":
		return "maintenance"
	case "03_Research":
		return "research"
	case "90_Delta":
		return "delta"
	}
	return "source"
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
			lang, stack, strong := classify(path)
			if strong {
				channel := "00_Source"
				parts := strings.Split(rel, string(os.PathSeparator))
				if len(parts) > 1 {
					channel = parts[0]
				}
				now := time.Now()
				out = append(out, registry.Project{ID: uuid.NewString(), Name: d.Name(), Slug: slug(d.Name()), Path: path, Channel: channel, FlowStage: flowFromChannel(channel), Language: lang, Stack: stack, HasGit: exists(filepath.Join(path, ".git")), HasBank: exists(filepath.Join(path, ".metadata", "project.toml")), HasMap: exists(filepath.Join(path, ".metadata", "agent", "PROJECT_MAP.md")), CreatedAt: now, LastScannedAt: now, OnDisk: true, Registered: true})
				return filepath.SkipDir
			}
		}
		return nil
	})
	return out, err
}
func StackJSON(v []string) string { b, _ := json.Marshal(v); return string(b) }
