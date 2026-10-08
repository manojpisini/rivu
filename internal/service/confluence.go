package service

import (
	"errors"
	"fmt"

	"github.com/manojpisini/rivu/internal/bank"
	"github.com/manojpisini/rivu/internal/registry"
)

// Confluence use-cases are registry-only for membership: they never
// move or delete project folders, and deleting a confluence only
// unlinks it (spec 1.2.6). Each mutation mirrors the registry's
// membership into the affected projects' Banks (P5.03).

// mirrorConfluences rewrites [rivu].confluences in the project's Bank
// from the registry. Projects without a Bank or missing on disk are
// skipped — creating folders here would resurrect projects (safety
// rule 1) and doctor reports the gap instead.
// ponytail: only confluence mutations and Source refresh the mirror;
// Sync/Flow preserve the file as-is — derive inside bank.Sync if
// hand-edited drift ever needs repairing.
func (a *App) mirrorConfluences(p registry.Project) error {
	if !p.OnDisk || !p.HasBank {
		return nil
	}
	names, err := a.Registry.ProjectConfluences(p.ID)
	if err != nil {
		return err
	}
	return bank.SetConfluences(p, names)
}

// mirrorMembers refreshes every member of a confluence that just
// changed; each failure carries the project slug. The registry change
// already happened, so callers say so in their error text.
func (a *App) mirrorMembers(members []registry.Project) error {
	var errs []error
	for _, p := range members {
		if err := a.mirrorConfluences(p); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p.Slug, err))
		}
	}
	return errors.Join(errs...)
}

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
// (slug-normalised) name; members stay linked and their Banks follow.
func (a *App) ConfluenceRename(q, newName string) (string, error) {
	members, err := a.Registry.ConfluenceMembers(q)
	if err != nil {
		return "", err
	}
	stored, err := a.Registry.RenameConfluence(q, newName)
	if err != nil {
		return "", err
	}
	if err := a.mirrorMembers(members); err != nil {
		return stored, fmt.Errorf("confluence renamed, but project.toml mirror failed: %w", err)
	}
	return stored, nil
}

// ConfluenceDelete removes the confluence and its membership links
// only — member projects are never touched (safety rule 1); their
// Banks drop the name.
func (a *App) ConfluenceDelete(q string) error {
	members, err := a.Registry.ConfluenceMembers(q)
	if err != nil {
		return err
	}
	if err := a.Registry.DeleteConfluence(q); err != nil {
		return err
	}
	if err := a.mirrorMembers(members); err != nil {
		return fmt.Errorf("confluence removed, but project.toml mirror failed: %w", err)
	}
	return nil
}

// ConfluenceAdd links a project to an existing confluence and mirrors
// the membership into the project's Bank. Unlike Source's --confluence
// flag, the confluence must already exist here so a typo cannot
// silently create a new grouping.
func (a *App) ConfluenceAdd(project, confluence string) error {
	if _, err := a.Registry.Confluence(confluence); err != nil {
		return err
	}
	p, err := a.Registry.Resolve(project)
	if err != nil {
		return err
	}
	if err := a.Registry.AttachConfluence(p.ID, confluence); err != nil {
		return err
	}
	if err := a.mirrorConfluences(p); err != nil {
		return fmt.Errorf("confluence linked, but project.toml mirror failed: %w", err)
	}
	return nil
}

// ConfluenceRemove unlinks a project from a confluence and reports
// whether a membership was actually removed; a real unlink also clears
// the name from the project's Bank.
func (a *App) ConfluenceRemove(project, confluence string) (bool, error) {
	p, err := a.Registry.Resolve(project)
	if err != nil {
		return false, err
	}
	linked, err := a.Registry.DetachConfluence(p.ID, confluence)
	if err != nil || !linked {
		return linked, err
	}
	if err := a.mirrorConfluences(p); err != nil {
		return true, fmt.Errorf("confluence unlinked, but project.toml mirror failed: %w", err)
	}
	return true, nil
}
