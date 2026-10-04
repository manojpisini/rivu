package mapgen

import (
	"fmt"
	"github.com/manojpisini/rivu/internal/registry"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ignored = map[string]bool{".git": true, "node_modules": true, "vendor": true, "target": true, "dist": true, "build": true, ".venv": true}

func Build(p registry.Project) error {
	d := filepath.Join(p.Path, ".metadata", "agent")
	if err := os.MkdirAll(d, 0755); err != nil {
		return err
	}
	agents := fmt.Sprintf("# Agent Instructions for %s\n\n## Read first\n1. `PROJECT_MAP.md`\n2. `../overview.md`\n3. `../tasks.md`\n4. `../decisions.md`\n\n## Project purpose\nSee `../overview.md`.\n\n## Important rules\n- Preserve the Bank (`.metadata/`).\n- Prefer small, tested changes.\n- Do not modify generated or vendor directories.\n\n## Ignore by default\n`.git`, `node_modules`, `vendor`, `target`, `dist`, `build`, caches.\n\n## Safe commands\nInspect project scripts and configuration before executing commands.\n", p.Name)
	if err := os.WriteFile(filepath.Join(d, "AGENTS.md"), []byte(agents), 0644); err != nil {
		return err
	}
	var files, dirs []string
	entries, _ := os.ReadDir(p.Path)
	for _, e := range entries {
		if ignored[e.Name()] || e.Name() == ".metadata" {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e.Name()+"/")
		} else {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	sort.Strings(dirs)
	stack := strings.Join(p.Stack, ", ")
	if stack == "" {
		stack = "Not detected"
	}
	body := fmt.Sprintf("# Project Map: %s\n\n## Purpose\nSee `.metadata/overview.md`.\n\n## Stack\n%s\n\n## Important files\n%s\n\n## Important directories\n%s\n\n## Generated/vendor directories\n`.git`, `node_modules`, `vendor`, `target`, `dist`, `build`, caches.\n\n## Suggested first-read order\n1. README and manifest files\n2. Source entrypoints\n3. Tests\n4. Build and CI configuration\n", p.Name, stack, list(files), list(dirs))
	return os.WriteFile(filepath.Join(d, "PROJECT_MAP.md"), []byte(body), 0644)
}
func list(v []string) string {
	if len(v) == 0 {
		return "- None detected"
	}
	var b strings.Builder
	for _, x := range v {
		b.WriteString("- `" + x + "`\n")
	}
	return strings.TrimSpace(b.String())
}
