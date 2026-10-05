// Package fake provides a hand-written service.Service for TUI and CLI
// tests: canned results plus a call log, no real I/O.
package fake

import (
	"sync"

	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/service"
)

// Service implements service.Service with canned results. Set the *Res
// and *Err fields, then assert on Calls. Reads (List, Current) are not
// recorded so View-style polling does not flood the log.
type Service struct {
	Projects   []registry.Project
	ScanRes    service.ScanResult
	ScanErr    error
	DoctorRes  []doctor.Report
	DoctorErr  error
	SourceRes  service.SourceResult
	SourceErr  error
	FlowRes    service.FlowResult
	FlowErr    error
	MapErr     error
	OpenErr    error
	CurrentP   registry.Project
	HasCurrent bool

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

func (f *Service) Scan() (service.ScanResult, error) {
	f.record("Scan")
	return f.ScanRes, f.ScanErr
}

func (f *Service) Doctor(q string) ([]doctor.Report, error) {
	f.record("Doctor " + q)
	return f.DoctorRes, f.DoctorErr
}

func (f *Service) Source(name, flow string, gitInit, adopt, dry bool) (service.SourceResult, error) {
	f.record("Source " + name)
	return f.SourceRes, f.SourceErr
}

func (f *Service) Flow(q, to string, flatten, dry bool) (service.FlowResult, error) {
	f.record("Flow " + q + " -> " + to)
	return f.FlowRes, f.FlowErr
}

func (f *Service) Map(q string) error {
	f.record("Map " + q)
	return f.MapErr
}

func (f *Service) OpenProject(q string) error {
	f.record("OpenProject " + q)
	return f.OpenErr
}

func (f *Service) Current() (registry.Project, bool) {
	return f.CurrentP, f.HasCurrent
}
