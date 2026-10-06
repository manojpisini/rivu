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
	Projects    []registry.Project
	ScanRes     service.ScanResult
	ScanErr     error
	DoctorRes   []doctor.Report
	DoctorErr   error
	SourceRes   service.SourceResult
	SourceErr   error
	FlowRes     service.FlowResult
	FlowErr     error
	MapErr      error
	MapStatuses []service.MapStatus
	StatsRes    service.Stats
	StatsErr    error
	DashRes     service.Dashboard
	IndexRes    service.IndexResult
	OpenErr     error
	CurrentP    registry.Project
	HasCurrent  bool

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
	f.record("OpenCommand " + q)
	if f.OpenErr != nil {
		return nil, f.OpenErr
	}
	return []string{editor, "path"}, nil
}

func (f *Service) Current() (registry.Project, bool) {
	return f.CurrentP, f.HasCurrent
}
