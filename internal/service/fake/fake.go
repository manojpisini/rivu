// Package fake provides a hand-written service.Service for TUI and CLI
// tests: canned results plus a call log, no real I/O.
package fake

import (
	"context"
	"sync"

	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
)

// Service implements service.Service with canned results. Set the *Res
// and *Err fields, then assert on Calls. Reads (List, Current) are not
// recorded so View-style polling does not flood the log.
type Service struct {
	Projects      []registry.Project
	ScanRes       service.ScanResult
	ScanErr       error
	DoctorRes     []doctor.Report
	DoctorErr     error
	SourceRes     service.SourceResult
	SourceErr     error
	FlowRes       service.FlowResult
	FlowErr       error
	MapErr        error
	MapStatuses   []service.MapStatus
	MapPreviewRes service.MapPreview
	MapPreviewErr error
	StatsRes      service.Stats
	StatsErr      error
	ActivityRes   []int
	ActivityErr   error
	DashRes       service.Dashboard
	IndexRes      service.IndexResult
	OpenErr       error
	CurrentP      registry.Project
	HasCurrent    bool
	ConflRes      []registry.Confluence
	ConflErr      error
	ConflShowRes  registry.Confluence
	ConflMembers  []registry.Project
	ConflNewRes   registry.Confluence
	ConflRename   string
	ConflRemove   bool
	ConflNames    []string
	LogLines      []string
	LogErr        error
	RecentAct     []registry.Activity
	RecentActErr  error

	mu    sync.Mutex
	calls []string
}

var _ service.Service = (*Service)(nil)

// Calls returns the recorded mutating invocations in order, e.g.
// "Scan", "Flow demo -> active".
func (f *Service) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *Service) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *Service) List(service.Filter) ([]registry.Project, error) {
	return f.Projects, nil
}

func (f *Service) Stats(days int) (service.Stats, error) {
	return f.StatsRes, f.StatsErr
}

// ActivityDaily serves the canned per-day counts for the Stats screen.
func (f *Service) ActivityDaily(days int) ([]int, error) {
	return f.ActivityRes, f.ActivityErr
}

func (f *Service) Dashboard() (service.Dashboard, error) {
	return f.DashRes, f.StatsErr
}

func (f *Service) Index() (service.IndexResult, error) {
	f.record("Index")
	return f.IndexRes, f.ScanErr
}

func (f *Service) Scan() (service.ScanResult, error) {
	f.record("Scan")
	return f.ScanRes, f.ScanErr
}

func (f *Service) ScanContext(ctx context.Context, progress func(dirs int)) (service.ScanResult, error) {
	f.record("ScanContext")
	if err := ctx.Err(); err != nil {
		return service.ScanResult{}, err
	}
	if progress != nil {
		progress(1)
	}
	return f.ScanRes, f.ScanErr
}

func (f *Service) Doctor(q string) ([]doctor.Report, error) {
	f.record("Doctor " + q)
	return f.DoctorRes, f.DoctorErr
}

func (f *Service) Source(name string, o service.SourceOpts) (service.SourceResult, error) {
	f.record("Source " + name)
	return f.SourceRes, f.SourceErr
}

func (f *Service) Flow(q, to string, flatten, dry bool) (service.FlowResult, error) {
	f.record("Flow " + q + " -> " + to)
	return f.FlowRes, f.FlowErr
}

func (f *Service) FlowBulk(queries []string, to string, flatten, dry bool) (service.BulkFlowResult, error) {
	f.record("FlowBulk -> " + to)
	var out service.BulkFlowResult
	for _, q := range queries {
		if f.FlowErr != nil {
			out.Failed = append(out.Failed, service.Failure{Query: q, Err: f.FlowErr})
			continue
		}
		out.Done = append(out.Done, f.FlowRes)
	}
	return out, nil
}

func (f *Service) Map(q string) error {
	f.record("Map " + q)
	return f.MapErr
}

func (f *Service) MapStatus(q string, all bool) ([]service.MapStatus, error) {
	return f.MapStatuses, f.MapErr
}

// MapPreview serves the canned Map report (P4.15) and records the read
// so tests can assert which slug the screen opened for.
func (f *Service) MapPreview(q string) (service.MapPreview, error) {
	f.record("MapPreview " + q)
	return f.MapPreviewRes, f.MapPreviewErr
}

func (f *Service) MapBulk() (service.BulkMapResult, error) {
	f.record("MapBulk")
	var out service.BulkMapResult
	if f.MapErr != nil {
		for _, p := range f.Projects {
			out.Failed = append(out.Failed, service.Failure{Query: p.Slug, Err: f.MapErr})
		}
		return out, nil
	}
	out.Done = f.Projects
	return out, nil
}

func (f *Service) OpenProject(q, editor string) error {
	f.record("OpenProject " + q)
	return f.OpenErr
}

func (f *Service) OpenCommand(q, editor string) ([]string, error) {
	rec := "OpenCommand " + q
	if editor != "" {
		rec += " " + editor
	}
	f.record(rec)
	if f.OpenErr != nil {
		return nil, f.OpenErr
	}
	if editor == "" {
		editor = "code"
	}
	return []string{editor, "path"}, nil
}

func (f *Service) Current() (registry.Project, bool) {
	return f.CurrentP, f.HasCurrent
}

// Confluences serves the canned browser list (P5.04) and records the
// read so tests can assert which screen loaded.
func (f *Service) Confluences() ([]registry.Confluence, error) {
	f.record("Confluences")
	return f.ConflRes, f.ConflErr
}

func (f *Service) ConfluenceShow(q string) (registry.Confluence, []registry.Project, error) {
	f.record("ConfluenceShow " + q)
	return f.ConflShowRes, f.ConflMembers, f.ConflErr
}

func (f *Service) ConfluenceNew(name, notes string) (registry.Confluence, error) {
	f.record("ConfluenceNew " + name)
	if f.ConflErr != nil {
		return registry.Confluence{}, f.ConflErr
	}
	return f.ConflNewRes, nil
}

func (f *Service) ConfluenceRename(q, newName string) (string, error) {
	f.record("ConfluenceRename " + q + " -> " + newName)
	if f.ConflErr != nil {
		return "", f.ConflErr
	}
	return f.ConflRename, nil
}

func (f *Service) ConfluenceDelete(q string) error {
	f.record("ConfluenceDelete " + q)
	return f.ConflErr
}

func (f *Service) ConfluenceAdd(project, confluence string) error {
	f.record("ConfluenceAdd " + project + " " + confluence)
	return f.ConflErr
}

func (f *Service) ConfluenceRemove(project, confluence string) (bool, error) {
	f.record("ConfluenceRemove " + project + " " + confluence)
	return f.ConflRemove, f.ConflErr
}

func (f *Service) ProjectConfluences(q string) ([]string, error) {
	f.record("ProjectConfluences " + q)
	return f.ConflNames, f.ConflErr
}

// LogTail serves the canned rivu.log tail for the Logs screen (P5.10).
func (f *Service) LogTail(n int) ([]string, error) {
	out := f.LogLines
	if n > 0 && len(out) > n {
		out = out[len(out)-n:]
	}
	return out, f.LogErr
}

// RecentActivity serves the canned activity_log rows (P5.10).
func (f *Service) RecentActivity(limit int) ([]registry.Activity, error) {
	out := f.RecentAct
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, f.RecentActErr
}
