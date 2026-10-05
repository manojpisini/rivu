package bank

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/registry"
)

func TestBuildErrorPaths(t *testing.T) {
	base := t.TempDir()
	// MkdirAll fails when the project path itself is an existing file.
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	p := registry.Project{ID: "1", Name: "Blocked", Slug: "blocked", Path: filepath.Join(blocker, "child"), FlowStage: "source", Channel: "00_Source"}
	if err := Build(p, "rivu", Meta{}); err == nil {
		t.Fatal("Build under an existing file must fail")
	}
	// Sync hits the same MkdirAll wall on its way to WriteFile.
	if err := Sync(p, "rivu"); err == nil {
		t.Fatal("Sync under an existing file must fail")
	}
}

func TestBuildFullMeta(t *testing.T) {
	p := registry.Project{ID: "id-9", Name: "Full", Slug: "full", Path: t.TempDir(), FlowStage: "source", Channel: "00_Source"}
	m := Meta{Domain: "chan/app", Type: "service", Template: "go-cli", Description: "desc", Confluences: []string{"db", "queue"}}
	if err := Build(p, "owner", m); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`domain = "chan/app"`,
		`type = "service"`,
		`template = "go-cli"`,
		`description = "desc"`,
		`confluences = ["db", "queue"]`,
		`git_init_owner = "owner"`,
		`created_at = "`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in:\n%s", want, s)
		}
	}
}

func TestSyncFallsBackOnBadCreatedAt(t *testing.T) {
	p := registry.Project{ID: "3", Name: "Bad", Slug: "bad", Path: t.TempDir(), FlowStage: "source", Channel: "00_Source"}
	meta := filepath.Join(p.Path, ".metadata")
	if err := os.MkdirAll(meta, 0755); err != nil {
		t.Fatal(err)
	}
	seed := "[rivu]\nid = \"3\"\nname = \"Bad\"\nslug = \"bad\"\nflow_stage = \"source\"\nchannel = \"00_Source\"\nconfluences = []\ngit_init_owner = \"me\"\ncreated_at = \"not-a-time\"\n"
	if err := os.WriteFile(filepath.Join(meta, "project.toml"), []byte(seed), 0644); err != nil {
		t.Fatal(err)
	}

	if err := Sync(p, ""); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(meta, "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]any
	if _, err := toml.Decode(string(b), &doc); err != nil {
		t.Fatalf("synced file must parse: %v", err)
	}
	rivu := doc["rivu"]
	got, _ := rivu["created_at"].(string)
	if _, err := time.Parse(time.RFC3339, got); err != nil {
		t.Errorf("unparseable created_at %q must fall back to a valid time", got)
	}
	if rivu["git_init_owner"] != "me" {
		t.Errorf("git_init_owner lost: %v", rivu["git_init_owner"])
	}
}
