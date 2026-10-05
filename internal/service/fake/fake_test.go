package fake_test

import (
	"errors"
	"testing"

	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/manojpisini/rivu/internal/service/fake"
)

func TestFakeServesCannedResultsAndRecordsCalls(t *testing.T) {
	f := &fake.Service{
		Projects:   []registry.Project{{ID: "1", Name: "demo"}},
		ScanRes:    service.ScanResult{Warnings: []string{"w1"}},
		FlowRes:    service.FlowResult{Note: "Flowed demo"},
		FlowErr:    errors.New("boom"),
		HasCurrent: true,
		CurrentP:   registry.Project{ID: "1"},
	}

	if ps, err := f.List(); err != nil || len(ps) != 1 || ps[0].Name != "demo" {
		t.Errorf("List = %v, %v; want demo", ps, err)
	}
	res, err := f.Scan()
	if err != nil || len(res.Warnings) != 1 {
		t.Errorf("Scan = %+v, %v", res, err)
	}
	if _, err := f.Flow("demo", "active", false, false); err == nil {
		t.Error("FlowErr not propagated")
	}
	if p, ok := f.Current(); !ok || p.ID != "1" {
		t.Errorf("Current = %+v, %v", p, ok)
	}
	// Reads are silent; mutations are logged in order.
	if got, want := f.Calls(), []string{"Scan", "Flow demo -> active"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Calls = %v, want %v", got, want)
	}
}
