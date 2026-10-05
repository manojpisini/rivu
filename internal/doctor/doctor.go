package doctor

import (
	"os"
	"path/filepath"

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

// Run scores a project. scaffoldMark is the bridge tool's marker file name
// ("" disables the exclusivity check); when the marker and .git coexist but
// the Bank claims rivu ran init, that is a possible double-init (spec 1.7).
func Run(p registry.Project, scaffoldMark string) Report {
	checks := []Check{{"README", ex(filepath.Join(p.Path, "README.md")) || ex(filepath.Join(p.Path, "README")), 20, "project documentation"}, {"Git", ex(filepath.Join(p.Path, ".git")), 15, "version control"}, {"Bank", ex(filepath.Join(p.Path, ".metadata", "project.toml")), 15, "Rivu metadata"}, {"Map", ex(filepath.Join(p.Path, ".metadata", "agent", "PROJECT_MAP.md")), 15, "agent-readable map"}, {"Tests", ex(filepath.Join(p.Path, "tests")) || ex(filepath.Join(p.Path, "internal")), 10, "test structure"}, {"CI", ex(filepath.Join(p.Path, ".github", "workflows")), 10, "continuous integration"}, {"License", ex(filepath.Join(p.Path, "LICENSE")) || ex(filepath.Join(p.Path, "LICENSE.md")), 10, "license file"}, {"Dependencies", true, 5, "manifest inspection available"}}
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
	return Report{p, checks, score}
}
