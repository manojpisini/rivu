package bank

import (
	"fmt"
	"github.com/manojpisini/rivu/internal/registry"
	"os"
	"path/filepath"
	"time"
)

func writeNew(path, body string) error {
	if _, e := os.Stat(path); e == nil {
		return nil
	}
	return os.WriteFile(path, []byte(body), 0644)
}
func Build(p registry.Project, owner string) error {
	d := filepath.Join(p.Path, ".metadata")
	if err := os.MkdirAll(d, 0755); err != nil {
		return err
	}
	if owner == "" {
		owner = "rivu"
	}
	created := p.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	toml := fmt.Sprintf("[rivu]\nid = %q\nname = %q\nslug = %q\nflow_stage = %q\nchannel = %q\nconfluences = []\ngit_init_owner = %q\ncreated_at = %q\n", p.ID, p.Name, p.Slug, p.FlowStage, p.Channel, owner, created.Format(time.RFC3339))
	files := map[string]string{"project.toml": toml, "overview.md": fmt.Sprintf("# Project Overview: %s\n\n## Purpose\n\n## Current status\n\n## Important context\n\n## Next steps\n", p.Name), "decisions.md": "# Decisions\n\n## " + time.Now().Format("2006-01-02") + " — Initial project setup\n", "tasks.md": "# Tasks\n\n## Now\n\n## Next\n\n## Later\n"}
	for n, b := range files {
		if err := writeNew(filepath.Join(d, n), b); err != nil {
			return err
		}
	}
	return nil
}
