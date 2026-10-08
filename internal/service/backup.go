package service

import (
	"fmt"
	"time"

	"github.com/manojpisini/rivu/internal/registry"
)

// ImportResult is the typed outcome of Import: where the pre-import
// snapshot of the database landed, so the caller can say so (P5.13).
type ImportResult struct {
	Backup string
}

// Export returns every registry row as a portable dump — the service
// behind `rivu db export`.
func (a *App) Export() (registry.Dump, error) {
	return a.Registry.Export()
}

// Import replaces the registry contents with d, snapshotting the
// current database file first. The caller must have confirmed
// (spec 4.4 rule 2: Plan → confirm → Apply); project folders on disk
// are never touched (safety rule 1).
func (a *App) Import(d registry.Dump) (ImportResult, error) {
	tag := "preimport-" + time.Now().Format("20060102-150405")
	path, err := a.Registry.BackupTo(tag)
	if err != nil {
		return ImportResult{}, err
	}
	if err := a.Registry.Import(d); err != nil {
		return ImportResult{Backup: path}, fmt.Errorf("restore failed (previous registry kept at %s): %w", path, err)
	}
	return ImportResult{Backup: path}, nil
}
