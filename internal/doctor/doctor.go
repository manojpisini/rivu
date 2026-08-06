package doctor

import (
	"github.com/manojpisini/rivu/internal/registry"
	"os"
	"path/filepath"
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
}

func ex(p string) bool { _, e := os.Stat(p); return e == nil }
func Run(p registry.Project) Report {
	checks := []Check{{"README", ex(filepath.Join(p.Path, "README.md")) || ex(filepath.Join(p.Path, "README")), 20, "project documentation"}, {"Git", ex(filepath.Join(p.Path, ".git")), 15, "version control"}, {"Bank", ex(filepath.Join(p.Path, ".metadata", "project.toml")), 15, "Rivu metadata"}, {"Map", ex(filepath.Join(p.Path, ".metadata", "agent", "PROJECT_MAP.md")), 15, "agent-readable map"}, {"Tests", ex(filepath.Join(p.Path, "tests")) || ex(filepath.Join(p.Path, "internal")), 10, "test structure"}, {"CI", ex(filepath.Join(p.Path, ".github", "workflows")), 10, "continuous integration"}, {"License", ex(filepath.Join(p.Path, "LICENSE")) || ex(filepath.Join(p.Path, "LICENSE.md")), 10, "license file"}, {"Dependencies", true, 5, "manifest inspection available"}}
	score := 0
	for _, c := range checks {
		if c.OK {
			score += c.Weight
		}
	}
	return Report{p, checks, score}
}
