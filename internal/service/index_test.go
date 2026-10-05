package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
