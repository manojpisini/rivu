package service

import "github.com/manojpisini/rivu/internal/registry"

// ScanResult is the typed outcome of a workspace Scan: every discovered
// project plus all non-fatal warnings, so callers render both without
// reading App state (P2.02).
type ScanResult struct {
	Projects []registry.Project
	Warnings []string
}

// SourceResult is the typed outcome of Source: the registered (or
// dry-run previewed) project, the Plan behind it, and any
// auto-correction warnings (spec 1.7 bridge exclusivity, O-06 adopt).
type SourceResult struct {
	Project  registry.Project
	Plan     SourcePlan
	Warnings []string
}

// FlowResult is the typed outcome of Flow: the destination project state,
// the Plan that was applied, and a human-readable Note saying what
// happened for the CLI/TUI to display.
type FlowResult struct {
	Project registry.Project
	Plan    FlowPlan
	Note    string
}

// SourcePlan lists every side effect a Source run will perform. Preflight
// (P1.40) builds it before anything is written; execution consumes it;
// dry-run rendering (O-03) prints it.
type SourcePlan struct {
	Name, Slug, Channel, FlowStage string
	ChannelDir, Path               string
	Create                         []string // directories this run will make
	Write                          []string // files this run will write
	Run                            []string // external commands this run will exec
	Registry                       []string // registry changes
	Bank                           []string // bank files
}

// FlowPlan lists every side effect a Flow move will perform. Preflight
// (P1.42) validates it before the rename; execution performs move, bank
// sync and the registry transaction in that order (P1.43).
type FlowPlan struct {
	ID, Query, FromStage, ToStage string
	Src, Dst, Root                string
	Flatten                       bool
	Move                          []string
	Registry                      []string
	Bank                          []string
}

// BulkFlowResult is the typed outcome of FlowBulk: every move that
// happened plus every query that failed, so callers can report partial
// success without re-running anything.
type BulkFlowResult struct {
	Done   []FlowResult
	Failed []Failure
}

// Failure is one query that a bulk operation could not process, with
// the reason. FlowBulk and MapBulk share it.
type Failure struct {
	Query string
	Err   error
}

// MapStatus reports what an agent Map sync would change for one
// project, without writing anything (P2.19 --check/--dry-run).
type MapStatus struct {
	Project       registry.Project
	AgentsMissing bool
	MapStale      bool
}

// BulkMapResult is the typed outcome of MapBulk, mirroring
// BulkFlowResult.
type BulkMapResult struct {
	Done   []registry.Project
	Failed []Failure
}
