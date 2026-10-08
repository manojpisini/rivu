package registry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// DumpSchema is the version of the JSON backup written by `rivu db
// export` and accepted by `rivu db import` (P5.13). Bump only for
// breaking shape changes; old files keep their number.
const DumpSchema = 1

// Dump is every user-owned registry row in one portable document. It
// carries data only — schema evolution stays with the migrations and
// user_version.
type Dump struct {
	Schema      int           `json:"schema"`
	DBVersion   int           `json:"db_version"`
	ExportedAt  time.Time     `json:"exported_at"`
	Projects    []Project     `json:"projects"`
	Confluences []Confluence  `json:"confluences"`
	Links       []Link        `json:"project_confluences"`
	Snapshots   []SnapshotRow `json:"health_snapshots"`
	Activity    []ActivityRow `json:"activity_log"`
	Settings    []KV          `json:"settings"`
}

// Link is one project_confluences row.
type Link struct {
	ProjectID    string `json:"project_id"`
	ConfluenceID string `json:"confluence_id"`
}

// SnapshotRow is one health_snapshots row including its ids — the
// display Snapshot type keeps only score and time.
type SnapshotRow struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	Score     int       `json:"score"`
	TakenAt   time.Time `json:"taken_at"`
}

// ActivityRow is one activity_log row (the display Activity joins the
// project name in).
type ActivityRow struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id"`
	Event      string    `json:"event"`
	OccurredAt time.Time `json:"occurred_at"`
}

// KV is one settings row (current_project_id and friends).
type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// queryEach runs q and calls each for every row — the shared read loop
// behind Export.
func queryEach(db *sql.DB, q string, each func(*sql.Rows) error) error {
	rows, err := db.Query(q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := each(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

// Export reads every table into a Dump — the read side of `rivu db
// export`. Slices are always non-nil so the file shows its shape.
func (r *Registry) Export() (Dump, error) {
	d := Dump{
		Schema:      DumpSchema,
		ExportedAt:  time.Now().UTC(),
		Projects:    []Project{},
		Confluences: []Confluence{},
		Links:       []Link{},
		Snapshots:   []SnapshotRow{},
		Activity:    []ActivityRow{},
		Settings:    []KV{},
	}
	if err := r.DB.QueryRow(`PRAGMA user_version`).Scan(&d.DBVersion); err != nil {
		return Dump{}, fmt.Errorf("read schema version: %w", err)
	}
	var err error
	if d.Projects, err = r.List(); err != nil {
		return Dump{}, fmt.Errorf("export projects: %w", err)
	}
	if d.Confluences, err = r.ListConfluences(); err != nil {
		return Dump{}, fmt.Errorf("export confluences: %w", err)
	}
	if err := queryEach(r.DB, `SELECT project_id,confluence_id FROM project_confluences`, func(rows *sql.Rows) error {
		var l Link
		if err := rows.Scan(&l.ProjectID, &l.ConfluenceID); err != nil {
			return err
		}
		d.Links = append(d.Links, l)
		return nil
	}); err != nil {
		return Dump{}, fmt.Errorf("export confluence membership: %w", err)
	}
	if err := queryEach(r.DB, `SELECT id,project_id,score,taken_at FROM health_snapshots`, func(rows *sql.Rows) error {
		var s SnapshotRow
		if err := rows.Scan(&s.ID, &s.ProjectID, &s.Score, &s.TakenAt); err != nil {
			return err
		}
		d.Snapshots = append(d.Snapshots, s)
		return nil
	}); err != nil {
		return Dump{}, fmt.Errorf("export health snapshots: %w", err)
	}
	if err := queryEach(r.DB, `SELECT id,project_id,event,occurred_at FROM activity_log`, func(rows *sql.Rows) error {
		var a ActivityRow
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.Event, &a.OccurredAt); err != nil {
			return err
		}
		d.Activity = append(d.Activity, a)
		return nil
	}); err != nil {
		return Dump{}, fmt.Errorf("export activity: %w", err)
	}
	if err := queryEach(r.DB, `SELECT key,value FROM settings`, func(rows *sql.Rows) error {
		var s KV
		if err := rows.Scan(&s.Key, &s.Value); err != nil {
			return err
		}
		d.Settings = append(d.Settings, s)
		return nil
	}); err != nil {
		return Dump{}, fmt.Errorf("export settings: %w", err)
	}
	return d, nil
}

// ParseDump decodes a backup file and rejects anything this build does
// not understand — a missing schema is a foreign file, not an empty
// registry (P5.13).
func ParseDump(b []byte) (Dump, error) {
	var d Dump
	if err := json.Unmarshal(b, &d); err != nil {
		return Dump{}, fmt.Errorf("not a rivu backup (invalid JSON): %w", err)
	}
	if d.Schema != DumpSchema {
		return Dump{}, fmt.Errorf("backup schema v%d is not supported (rivu reads v%d) — export again with a matching rivu", d.Schema, DumpSchema)
	}
	return d, nil
}

// BackupTo snapshots the database file to <db>.<tag>.bak and returns
// the path. `rivu db import` takes this before it replaces rows; the
// migration path keeps its own versioned backup (backupBefore).
func (r *Registry) BackupTo(tag string) (string, error) {
	var dbfile string
	if err := r.DB.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &dbfile); err != nil {
		return "", fmt.Errorf("locate database file: %w", err)
	}
	if dbfile == "" {
		return "", fmt.Errorf("database has no file to back up")
	}
	path := fmt.Sprintf("%s.%s.bak", dbfile, tag)
	if _, err := r.DB.Exec(`VACUUM INTO ?`, path); err != nil {
		return "", fmt.Errorf("back up database before restore: %w", err)
	}
	return path, nil
}

// Import replaces every registry row with the dump in one transaction:
// clear, then insert in foreign-key order. Nothing outside the
// database is touched — project folders stay exactly as they are
// (safety rule 1). A failed import rolls back completely.
func (r *Registry) Import(d Dump) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, del := range []string{
		`DELETE FROM activity_log`,
		`DELETE FROM health_snapshots`,
		`DELETE FROM project_confluences`,
		`DELETE FROM confluences`,
		`DELETE FROM projects`,
		`DELETE FROM settings`,
	} {
		if _, err := tx.Exec(del); err != nil {
			return fmt.Errorf("clear registry: %w", err)
		}
	}
	insProject, err := tx.Prepare(`INSERT INTO projects(id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered,root) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insProject.Close()
	for _, p := range d.Projects {
		st, err := json.Marshal(p.Stack)
		if err != nil {
			return fmt.Errorf("encode stack for %s: %w", p.Slug, err)
		}
		if _, err := insProject.Exec(p.ID, p.Name, p.Slug, canonical(p.Path), p.Channel, p.FlowStage, p.Language, string(st),
			b(p.HasGit), b(p.HasBank), b(p.HasMap), p.HealthScore, p.CreatedAt, p.LastOpenedAt, p.LastScannedAt,
			b(p.OnDisk), b(p.Registered), p.Root); err != nil {
			return fmt.Errorf("restore project %s: %w", p.Slug, err)
		}
	}
	for _, c := range d.Confluences {
		if _, err := tx.Exec(`INSERT INTO confluences(id,name,notes) VALUES(?,?,?)`, c.ID, c.Name, c.Notes); err != nil {
			return fmt.Errorf("restore confluence %s: %w", c.Name, err)
		}
	}
	for _, l := range d.Links {
		if _, err := tx.Exec(`INSERT INTO project_confluences(project_id,confluence_id) VALUES(?,?)`, l.ProjectID, l.ConfluenceID); err != nil {
			return fmt.Errorf("restore confluence membership: %w", err)
		}
	}
	for _, s := range d.Snapshots {
		if _, err := tx.Exec(`INSERT INTO health_snapshots(id,project_id,score,taken_at) VALUES(?,?,?,?)`, s.ID, s.ProjectID, s.Score, s.TakenAt); err != nil {
			return fmt.Errorf("restore health snapshot: %w", err)
		}
	}
	for _, a := range d.Activity {
		if _, err := tx.Exec(`INSERT INTO activity_log(id,project_id,event,occurred_at) VALUES(?,?,?,?)`, a.ID, a.ProjectID, a.Event, a.OccurredAt); err != nil {
			return fmt.Errorf("restore activity: %w", err)
		}
	}
	for _, kv := range d.Settings {
		if _, err := tx.Exec(`INSERT INTO settings(key,value) VALUES(?,?)`, kv.Key, kv.Value); err != nil {
			return fmt.Errorf("restore setting %s: %w", kv.Key, err)
		}
	}
	return tx.Commit()
}
