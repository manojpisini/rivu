package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

func TestIndexReconcilesStageMismatch(t *testing.T) {
	a := openTestApp(t)
	sr, err := a.Source("reloc", SourceOpts{Flow: "active"})
	if err != nil {
		t.Fatalf("Source: %v", err)
	}
	p := sr.Project

	// Simulate a hand-edited or half-applied state: the folder sits in
	// 01_Active but the registry stage still says source (mismatch).
	if err := a.Registry.UpdatePathFlow(p.ID, p.Path, "01_Active", "source"); err != nil {
		t.Fatal(err)
	}

	res, err := a.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if len(res.Reconciled) != 1 || res.Reconciled[0].Slug != "reloc" ||
		res.Reconciled[0].From != "source" || res.Reconciled[0].To != "active" {
		t.Fatalf("Reconciled = %+v, want reloc source->active", res.Reconciled)
	}

	// Registry agrees with the folder again and the mismatch is gone.
	got, err := a.Registry.Find("reloc")
	if err != nil {
		t.Fatal(err)
	}
	if got.FlowStage != "active" || got.Channel != "01_Active" {
		t.Errorf("after index: channel=%q stage=%q, want 01_Active/active", got.Channel, got.FlowStage)
	}
	st, err := a.Registry.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.StageMismatch) != 0 {
		t.Errorf("stage mismatch remains: %+v", st.StageMismatch)
	}

	// project.toml followed the stage (bank.Sync inside reconcile).
	body, err := os.ReadFile(filepath.Join(p.Path, ".metadata", "project.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `flow_stage = "active"`) {
		t.Errorf("project.toml not updated:\n%s", body)
	}

	// Idempotent: a second rebuild finds nothing to reconcile.
	res, err = a.Index()
	if err != nil {
		t.Fatalf("second Index: %v", err)
	}
	if len(res.Reconciled) != 0 {
		t.Errorf("second Reconciled = %+v, want none", res.Reconciled)
	}
}

// TestIndexLeavesRootPlacementAlone (S-05): a root project stays
// flagged as a stage mismatch — index must not fake a channel for it
// or report a no-op reconcile forever.
func TestIndexLeavesRootPlacementAlone(t *testing.T) {
	a := openTestApp(t)
	solo := filepath.Join(a.Config.Workspace.Root, "rootsolo")
	if err := os.MkdirAll(solo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(solo, "go.mod"), []byte("module x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := a.Registry.Upsert(registry.Project{
		ID: "root-1", Name: "rootsolo", Slug: "rootsolo", Path: solo,
		Channel: registry.RootChannel, FlowStage: "source",
		CreatedAt: time.Now(), OnDisk: true, Registered: true,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := a.Index()
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	if len(res.Reconciled) != 0 {
		t.Errorf("Reconciled = %+v, want none for root placement", res.Reconciled)
	}
	st, err := a.Registry.States()
	if err != nil {
		t.Fatal(err)
	}
	flagged := false
	for _, p := range st.StageMismatch {
		if p.Slug == "rootsolo" {
			flagged = true
		}
	}
	if !flagged {
		t.Errorf("root project must stay flagged: %+v", st.StageMismatch)
	}
	got, err := a.Registry.Find("rootsolo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Channel != registry.RootChannel {
		t.Errorf("channel was rewritten to %q", got.Channel)
	}
}
