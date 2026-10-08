//go:build race

package tui

// raceBuild marks tests running under the race detector. teatest
// enables tea.WithANSICompressor, and bubbletea's SetWindowTitle writes
// the shared compressed output from the event loop without the
// renderer's mutex — a DATA RACE inside bubbletea (v1.3.5 through
// v1.3.10) that no caller can lock away.
const raceBuild = true
