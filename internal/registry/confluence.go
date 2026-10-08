package registry

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/manojpisini/rivu/internal/slug"
)

// Confluence is a named cross-cutting grouping of projects (spec 1.2.6):
// tags with a browsing view, never folders. Membership is many-to-many and
// deleting a confluence only unlinks it — no project is ever touched
// (safety rule 1).
type Confluence struct {
	ID, Name, Notes string
	Members         int
}

// confluenceCols is the list/find projection; Members comes from the
// count. IFNULL keeps rows created by AttachConfluence (no notes) scannable.
const confluenceCols = `c.id,c.name,IFNULL(c.notes,'')`

// normalizeConfluenceName validates a user-supplied confluence name the
// same way project names are validated (safety rule 4): through slug.Make.
func normalizeConfluenceName(name string) (string, error) {
	s, err := slug.Make(strings.TrimSpace(name))
	if err != nil {
		return "", fmt.Errorf("invalid confluence name: %w", err)
	}
	return s, nil
}

// CreateConfluence inserts a confluence with a unique, slug-normalised
// name. An existing name returns a plain error naming it.
func (r *Registry) CreateConfluence(name, notes string) (Confluence, error) {
	n, err := normalizeConfluenceName(name)
	if err != nil {
		return Confluence{}, err
	}
	res, err := r.DB.Exec(`INSERT INTO confluences(id,name,notes) VALUES(?,?,?) ON CONFLICT(name) DO NOTHING`, uuid.NewString(), n, notes)
	if err != nil {
		return Confluence{}, fmt.Errorf("create confluence: %w", err)
	}
	if n2, _ := res.RowsAffected(); n2 == 0 {
		return Confluence{}, fmt.Errorf("confluence %q already exists", n)
	}
	c := Confluence{Name: n, Notes: notes}
	if err := r.DB.QueryRow(`SELECT id FROM confluences WHERE name=?`, n).Scan(&c.ID); err != nil {
		return Confluence{}, err
	}
	return c, nil
}

// Confluence finds one confluence by name or id with its member count.
func (r *Registry) Confluence(q string) (Confluence, error) {
	row := r.DB.QueryRow(`SELECT `+confluenceCols+`,(SELECT count(*) FROM project_confluences pc WHERE pc.confluence_id=c.id) FROM confluences c WHERE c.name=? OR c.id=?`, q, q)
	var c Confluence
	if err := row.Scan(&c.ID, &c.Name, &c.Notes, &c.Members); err != nil {
		return Confluence{}, confluenceLookupErr(q, err)
	}
	return c, nil
}

// ListConfluences returns every confluence with its member count, by name.
func (r *Registry) ListConfluences() ([]Confluence, error) {
	rows, err := r.DB.Query(`SELECT ` + confluenceCols + `,count(pc.project_id) FROM confluences c LEFT JOIN project_confluences pc ON pc.confluence_id=c.id GROUP BY c.id ORDER BY c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Confluence
	for rows.Next() {
		var c Confluence
		if err := rows.Scan(&c.ID, &c.Name, &c.Notes, &c.Members); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RenameConfluence renames the confluence found by name or id; the new
// name is slug-normalised and must not be taken. It returns the stored
// name so callers can print what was actually written.
func (r *Registry) RenameConfluence(q, newName string) (string, error) {
	c, err := r.Confluence(q)
	if err != nil {
		return "", err
	}
	n, err := normalizeConfluenceName(newName)
	if err != nil {
		return "", err
	}
	if n == c.Name {
		return c.Name, nil
	}
	var taken int
	if err := r.DB.QueryRow(`SELECT count(*) FROM confluences WHERE name=?`, n).Scan(&taken); err != nil {
		return "", err
	}
	if taken > 0 {
		return "", fmt.Errorf("confluence %q already exists", n)
	}
	if _, err := r.DB.Exec(`UPDATE confluences SET name=? WHERE id=?`, n, c.ID); err != nil {
		return "", fmt.Errorf("rename confluence: %w", err)
	}
	return n, nil
}

// DeleteConfluence removes the confluence and its membership links only;
// member projects are never touched (safety rule 1). Membership rows go
// with it through the schema's ON DELETE CASCADE.
func (r *Registry) DeleteConfluence(q string) error {
	c, err := r.Confluence(q)
	if err != nil {
		return err
	}
	if _, err := r.DB.Exec(`DELETE FROM confluences WHERE id=?`, c.ID); err != nil {
		return fmt.Errorf("delete confluence: %w", err)
	}
	return nil
}

// ConfluenceMembers returns the member projects of the confluence found
// by name or id, in lifecycle order.
func (r *Registry) ConfluenceMembers(q string) ([]Project, error) {
	c, err := r.Confluence(q)
	if err != nil {
		return nil, err
	}
	rows, err := r.DB.Query(`SELECT p.`+strings.ReplaceAll(cols, ",", ",p.")+` FROM projects p JOIN project_confluences pc ON pc.project_id=p.id WHERE pc.confluence_id=? ORDER BY `+lifecycleOrderBy, c.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		p, e := scanProject(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ProjectConfluences returns the confluence names a project belongs to,
// by name — the Detail screen and the Bank mirror read this (spec 3.5).
func (r *Registry) ProjectConfluences(projectID string) ([]string, error) {
	rows, err := r.DB.Query(`SELECT c.name FROM confluences c JOIN project_confluences pc ON pc.confluence_id=c.id WHERE pc.project_id=? ORDER BY c.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ConfluenceProjectIDs returns the project IDs that are members of the
// confluence identified by name or id. An unknown confluence matches
// nothing: filters narrow a list, they do not fail it.
func (r *Registry) ConfluenceProjectIDs(q string) ([]string, error) {
	rows, err := r.DB.Query(`SELECT pc.project_id FROM project_confluences pc JOIN confluences c ON c.id=pc.confluence_id WHERE c.name=? OR c.id=?`, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// AttachConfluence links projectID to the confluence named name (slug-
// normalised), creating the confluence when it does not exist. Repeating
// a membership is a no-op; an invalid name is an error.
func (r *Registry) AttachConfluence(projectID, name string) error {
	n, err := normalizeConfluenceName(name)
	if err != nil {
		return err
	}
	if _, err := r.DB.Exec(`INSERT INTO confluences(id,name) VALUES(?,?) ON CONFLICT(name) DO NOTHING`, uuid.NewString(), n); err != nil {
		return fmt.Errorf("create confluence: %w", err)
	}
	_, err = r.DB.Exec(`INSERT INTO project_confluences(project_id,confluence_id) SELECT ?,id FROM confluences WHERE name=? ON CONFLICT DO NOTHING`, projectID, n)
	return err
}

// DetachConfluence unlinks projectID from the confluence found by name or
// id — membership only; neither side is deleted. It reports whether a
// link was actually removed.
func (r *Registry) DetachConfluence(projectID, q string) (bool, error) {
	c, err := r.Confluence(q)
	if err != nil {
		return false, err
	}
	res, err := r.DB.Exec(`DELETE FROM project_confluences WHERE project_id=? AND confluence_id=?`, projectID, c.ID)
	if err != nil {
		return false, fmt.Errorf("unlink confluence: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func confluenceLookupErr(q string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no confluence matches %q", ErrConfluenceNotFound, q)
	}
	return err
}
