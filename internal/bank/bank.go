package bank

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/registry"
)

func writeNew(path, body string) error {
	if _, e := os.Stat(path); e == nil {
		return nil
	}
	return os.WriteFile(path, []byte(body), 0644)
}

// Sync refreshes the machine-owned project.toml so it matches p (which
// just changed stage/channel/path), preserving unknown keys — confluences,
// git_init_owner, user notes. A missing file is created as Build would.
// Human docs (overview, decisions, tasks) are untouched here; they stay
// create-if-missing via writeNew.
func Sync(p registry.Project, owner string) error {
	path := filepath.Join(p.Path, ".metadata", "project.toml")
	doc := map[string]map[string]any{}
	if b, err := os.ReadFile(path); err == nil {
		if _, err := toml.Decode(string(b), &doc); err != nil {
			return fmt.Errorf("parse project.toml: %w", err)
		}
	}
	rivu := doc["rivu"]
	if rivu == nil {
		rivu = map[string]any{}
		doc["rivu"] = rivu
	}
	created := p.CreatedAt
	if s, ok := rivu["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			created = t
		}
	}
	if created.IsZero() {
		created = time.Now()
	}
	if owner == "" {
		owner = "rivu"
	}
	rivu["id"] = p.ID
	rivu["name"] = p.Name
	rivu["slug"] = p.Slug
	rivu["flow_stage"] = p.FlowStage
	rivu["channel"] = p.Channel
	rivu["created_at"] = created.Format(time.RFC3339)
	if _, ok := rivu["confluences"]; !ok {
		rivu["confluences"] = []any{}
	}
	if _, ok := rivu["git_init_owner"]; !ok {
		rivu["git_init_owner"] = owner
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(doc); err != nil {
		return fmt.Errorf("encode project.toml: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
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
