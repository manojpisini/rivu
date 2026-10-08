package service

import (
	"github.com/manojpisini/rivu/internal/registry"
)

// Confluence use-cases are registry-only: they never touch the
// filesystem, and deleting a confluence only unlinks it (spec 1.2.6).
// The CLI and the TUI (spec 3.7) share these.

// Confluences returns every confluence with its member count.
func (a *App) Confluences() ([]registry.Confluence, error) {
	return a.Registry.ListConfluences()
}

// ConfluenceNew creates a confluence; the name is slug-normalised.
func (a *App) ConfluenceNew(name, notes string) (registry.Confluence, error) {
	return a.Registry.CreateConfluence(name, notes)
}

// ConfluenceShow returns one confluence (by name or id) and its member
// projects in lifecycle order.
func (a *App) ConfluenceShow(q string) (registry.Confluence, []registry.Project, error) {
	c, err := a.Registry.Confluence(q)
	if err != nil {
		return registry.Confluence{}, nil, err
	}
	members, err := a.Registry.ConfluenceMembers(q)
	if err != nil {
		return registry.Confluence{}, nil, err
	}
	return c, members, nil
}

// ConfluenceRename renames a confluence and returns the stored
// (slug-normalised) name; members stay linked.
func (a *App) ConfluenceRename(q, newName string) (string, error) {
	return a.Registry.RenameConfluence(q, newName)
}

// ConfluenceDelete removes the confluence and its membership links
// only — member projects are never touched (safety rule 1).
func (a *App) ConfluenceDelete(q string) error {
	return a.Registry.DeleteConfluence(q)
}

// ConfluenceAdd links a project to an existing confluence. Unlike
// Source's --confluence flag, the confluence must already exist here so
// a typo cannot silently create a new grouping.
func (a *App) ConfluenceAdd(project, confluence string) error {
	if _, err := a.Registry.Confluence(confluence); err != nil {
		return err
	}
	p, err := a.Registry.Resolve(project)
	if err != nil {
		return err
	}
	return a.Registry.AttachConfluence(p.ID, confluence)
}

// ConfluenceRemove unlinks a project from a confluence and reports
// whether a membership was actually removed.
func (a *App) ConfluenceRemove(project, confluence string) (bool, error) {
	p, err := a.Registry.Resolve(project)
	if err != nil {
		return false, err
	}
	return a.Registry.DetachConfluence(p.ID, confluence)
}
