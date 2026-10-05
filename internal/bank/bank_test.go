package bank

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/registry"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestBuildProtectsExistingFiles(t *testing.T) {
	p := registry.Project{ID: "1", Name: "Demo", Slug: "demo", Path: t.TempDir(), FlowStage: "source", Channel: "00_Source"}
	if err := Build(p, "rivu"); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(p.Path, ".metadata", "overview.md")
	if err := os.WriteFile(f, []byte("custom"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Build(p, "rivu"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(f)
	if string(b) != "custom" {
		t.Fatal("protected file overwritten")
	}
	// Human docs stay create-if-missing on every rebuild.
	for _, name := range []string{"decisions.md", "tasks.md"} {
		g := filepath.Join(p.Path, ".metadata", name)
		if err := os.WriteFile(g, []byte("mine"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := Build(p, "rivu"); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(g); string(got) != "mine" {
			t.Errorf("%s overwritten: %q", name, got)
		}
	}
}

func TestSyncRewritesStageAndPreservesExtras(t *testing.T) {
	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	p := registry.Project{ID: "1", Name: "Demo", Slug: "demo", Path: t.TempDir(), FlowStage: "source", Channel: "00_Source", CreatedAt: created}
	if err := Build(p, "rivu"); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(p.Path, ".metadata")
	// Hand-edits that must survive a Sync.
	body := "[rivu]\nid = \"1\"\nname = \"Demo\"\nslug = \"demo\"\nflow_stage = \"source\"\nchannel = \"00_Source\"\nconfluences = [\"db\", \"queue\"]\ngit_init_owner = \"me\"\ncreated_at = \"2024-05-06T07:08:09Z\"\n\n[user]\nnotes = \"keep me\"\n"
	if err := os.WriteFile(filepath.Join(meta, "project.toml"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"overview.md", "decisions.md", "tasks.md"} {
		if err := os.WriteFile(filepath.Join(meta, f), []byte("human:"+f), 0644); err != nil {
			t.Fatal(err)
		}
	}

	p.FlowStage = "active"
	p.Channel = "01_Active"
	if err := Sync(p, "rivu"); err != nil {
		t.Fatal(err)
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
	if rivu["flow_stage"] != "active" || rivu["channel"] != "01_Active" {
		t.Errorf("stage not refreshed: flow_stage=%v channel=%v", rivu["flow_stage"], rivu["channel"])
	}
	if got := fmt.Sprint(rivu["confluences"]); got != "[db queue]" {
		t.Errorf("confluences lost: %v", got)
	}
	if rivu["git_init_owner"] != "me" {
		t.Errorf("git_init_owner lost: %v", rivu["git_init_owner"])
	}
	if rivu["created_at"] != "2024-05-06T07:08:09Z" {
		t.Errorf("created_at changed: %v", rivu["created_at"])
	}
	if doc["user"]["notes"] != "keep me" {
		t.Errorf("unknown [user] table lost: %v", doc["user"])
	}
	// Human docs stay create-if-missing.
	for _, f := range []string{"overview.md", "decisions.md", "tasks.md"} {
		got, _ := os.ReadFile(filepath.Join(meta, f))
		if string(got) != "human:"+f {
			t.Errorf("%s overwritten by Sync: %q", f, got)
		}
	}
}

func TestSyncGolden(t *testing.T) {
	created := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	p := registry.Project{ID: "id-1", Name: "Demo", Slug: "demo", Path: t.TempDir(), FlowStage: "active", Channel: "01_Active", CreatedAt: created}
	seed := "[rivu]\nid = \"1\"\nname = \"Old\"\nslug = \"old\"\nflow_stage = \"source\"\nchannel = \"00_Source\"\nconfluences = [\"db\", \"queue\"]\ngit_init_owner = \"me\"\ncreated_at = \"2024-05-06T07:08:09Z\"\n\n[user]\nnotes = \"keep me\"\n"
	if err := os.MkdirAll(filepath.Join(p.Path, ".metadata"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Path, ".metadata", "project.toml"), []byte(seed), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Sync(p, "rivu"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(p.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "project_toml.golden")
	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, got, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("project.toml mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestSyncCreatesMissingProjectToml(t *testing.T) {
	p := registry.Project{ID: "2", Name: "Fresh", Slug: "fresh", Path: t.TempDir(), FlowStage: "source", Channel: "00_Source", CreatedAt: time.Now()}
	if err := Sync(p, "rivu"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]any
	if _, err := toml.Decode(string(b), &doc); err != nil {
		t.Fatalf("created file must parse: %v", err)
	}
	if doc["rivu"]["slug"] != "fresh" || doc["rivu"]["git_init_owner"] != "rivu" {
		t.Errorf("unexpected content: %v", doc["rivu"])
	}
}
