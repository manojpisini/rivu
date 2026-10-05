package app

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
