package registry

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"
)

type Project struct {
	ID, Name, Slug, Path, Channel, FlowStage, Language string
	Stack                                              []string
	HasGit, HasBank, HasMap                            bool
	HealthScore                                        int
	CreatedAt, LastOpenedAt, LastScannedAt             time.Time
	OnDisk, Registered                                 bool
}
type Registry struct{ DB *sql.DB }

func Open(path string) (*Registry, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_time_format=sqlite", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Pragmas apply per connection; one connection keeps them in force.
	db.SetMaxOpenConns(1)
	r := &Registry{DB: db}
	if err = r.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}
func (r *Registry) Close() error { return r.DB.Close() }

// migrations[i] upgrades the database from version i to i+1.
// Schema v1 is frozen after P1.10; append only, never edit a released entry.
// New entries follow AGENTS.md §9: one transaction each, back up first.
var migrations = []string{
	// v1 — initial schema (idempotent so pre-versioning databases adopt it).
	`CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,name TEXT NOT NULL,slug TEXT NOT NULL UNIQUE,path TEXT NOT NULL UNIQUE,channel TEXT NOT NULL,flow_stage TEXT NOT NULL,language TEXT,stack TEXT,has_git INTEGER DEFAULT 0,has_bank INTEGER DEFAULT 0,has_map INTEGER DEFAULT 0,health_score INTEGER DEFAULT 0,created_at DATETIME,last_opened_at DATETIME,last_scanned_at DATETIME,on_disk INTEGER DEFAULT 1,registered INTEGER DEFAULT 1);
CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT);
CREATE TABLE IF NOT EXISTS confluences(id TEXT PRIMARY KEY,name TEXT NOT NULL UNIQUE,notes TEXT);
CREATE TABLE IF NOT EXISTS project_confluences(project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,confluence_id TEXT REFERENCES confluences(id) ON DELETE CASCADE,PRIMARY KEY(project_id,confluence_id));
CREATE TABLE IF NOT EXISTS health_snapshots(id TEXT PRIMARY KEY,project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,score INTEGER,taken_at DATETIME);
CREATE TABLE IF NOT EXISTS activity_log(id TEXT PRIMARY KEY,project_id TEXT,event TEXT,occurred_at DATETIME);`,
}

func (r *Registry) migrate() error {
	var version int
	if err := r.DB.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema v%d is newer than this rivu build supports (v%d) — upgrade rivu", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		if err := r.backupBefore(i); err != nil {
			return err
		}
		tx, err := r.DB.Begin()
		if err != nil {
			return fmt.Errorf("migrate to v%d: %w", i+1, err)
		}
		if _, err = tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate to v%d: %w", i+1, err)
		}
		if _, err = tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrate to v%d: %w", i+1, err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("migrate to v%d: %w", i+1, err)
		}
	}
	return nil
}

// backupBefore snapshots an existing schema to <db>.v<i>.bak before migration.
// Fresh (empty) databases need no backup.
func (r *Registry) backupBefore(version int) error {
	var tables int
	if err := r.DB.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		return fmt.Errorf("inspect schema: %w", err)
	}
	if tables == 0 {
		return nil
	}
	var dbfile string
	if err := r.DB.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &dbfile); err != nil || dbfile == "" {
		return nil
	}
	if _, err := r.DB.Exec(`VACUUM INTO ?`, fmt.Sprintf("%s.v%d.bak", dbfile, version)); err != nil {
		return fmt.Errorf("back up database before migration: %w", err)
	}
	return nil
}
func b(v bool) int {
	if v {
		return 1
	}
	return 0
}

// canonical is the one normalisation applied to every stored path:
// Clean always, EvalSymlinks when the path exists.
func canonical(p string) string {
	p = filepath.Clean(p)
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p
}

// pathMatchClause compares paths case-folded where the filesystem does.
var pathMatchClause = func() string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return `path=? COLLATE NOCASE`
	}
	return `path=?`
}()

// Discover registers a scanned project or refreshes its discovery fields.
// Registry-owned fields (health_score, name, slug, flow_stage, created_at)
// are never written by a rescan — registry is truth, filesystem is discovery.
func (r *Registry) Discover(p Project) error {
	updated, err := r.UpdateDiscovery(p)
	if err != nil {
		return err
	}
	if updated {
		return nil
	}
	return r.InsertDiscovered(p)
}

// UpdateDiscovery refreshes only filesystem-derived fields for the row
// matched by p.Path. Returns false when no row matches.
func (r *Registry) UpdateDiscovery(p Project) (bool, error) {
	st, _ := json.Marshal(p.Stack)
	res, err := r.DB.Exec(`UPDATE projects SET language=?,stack=?,has_git=?,has_bank=?,has_map=?,last_scanned_at=?,on_disk=1 WHERE `+pathMatchClause,
		p.Language, string(st), b(p.HasGit), b(p.HasBank), b(p.HasMap), p.LastScannedAt, canonical(p.Path))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// InsertDiscovered writes a full row for a path not yet registered.
func (r *Registry) InsertDiscovered(p Project) error {
	p.Path = canonical(p.Path)
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	p.Slug, err = uniqueSlug(tx, p.Slug, p.Path)
	if err != nil {
		return err
	}
	st, _ := json.Marshal(p.Stack)
	if _, err = tx.Exec(`INSERT INTO projects(id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Slug, p.Path, p.Channel, p.FlowStage, p.Language, string(st), b(p.HasGit), b(p.HasBank), b(p.HasMap), p.HealthScore, p.CreatedAt, p.LastOpenedAt, p.LastScannedAt, b(p.OnDisk), b(p.Registered)); err != nil {
		return err
	}
	return tx.Commit()
}

// Upsert inserts or fully updates a row while preserving identity
// (id, slug, created_at, health_score). Used by registration, not by rescan.
func (r *Registry) Upsert(p Project) error {
	p.Path = canonical(p.Path)
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// A scan creates fresh candidate IDs, but project identity belongs to the
	// existing path. Preserve both ID and slug when the same path is rescanned.
	var existingID, existingSlug, existingPath string
	err = tx.QueryRow(`SELECT id,slug,path FROM projects WHERE `+pathMatchClause, p.Path).Scan(&existingID, &existingSlug, &existingPath)
	switch {
	case err == nil:
		p.ID = existingID
		p.Slug = existingSlug
		// Keep the stored casing so ON CONFLICT(path) hits this row.
		p.Path = existingPath
	case errors.Is(err, sql.ErrNoRows):
		p.Slug, err = uniqueSlug(tx, p.Slug, p.Path)
		if err != nil {
			return err
		}
	default:
		return err
	}

	st, _ := json.Marshal(p.Stack)
	_, err = tx.Exec(`INSERT INTO projects(id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET name=excluded.name,channel=excluded.channel,flow_stage=excluded.flow_stage,language=excluded.language,stack=excluded.stack,has_git=excluded.has_git,has_bank=excluded.has_bank,has_map=excluded.has_map,last_scanned_at=excluded.last_scanned_at,on_disk=1,registered=1`, p.ID, p.Name, p.Slug, p.Path, p.Channel, p.FlowStage, p.Language, string(st), b(p.HasGit), b(p.HasBank), b(p.HasMap), p.HealthScore, p.CreatedAt, p.LastOpenedAt, p.LastScannedAt, b(p.OnDisk), b(p.Registered))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func uniqueSlug(tx *sql.Tx, base, path string) (string, error) {
	if base == "" {
		base = "project"
	}
	candidate := base
	for i := 2; ; i++ {
		var owner string
		err := tx.QueryRow(`SELECT path FROM projects WHERE slug=?`, candidate).Scan(&owner)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return candidate, nil
		case err != nil:
			return "", err
		case owner == path:
			return candidate, nil
		default:
			candidate = fmt.Sprintf("%s-%d", base, i)
		}
	}
}
func scanProject(s interface{ Scan(...any) error }) (Project, error) {
	var p Project
	var st string
	var hg, hb, hm, od, reg int
	err := s.Scan(&p.ID, &p.Name, &p.Slug, &p.Path, &p.Channel, &p.FlowStage, &p.Language, &st, &hg, &hb, &hm, &p.HealthScore, &p.CreatedAt, &p.LastOpenedAt, &p.LastScannedAt, &od, &reg)
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal([]byte(st), &p.Stack)
	p.HasGit = hg != 0
	p.HasBank = hb != 0
	p.HasMap = hm != 0
	p.OnDisk = od != 0
	p.Registered = reg != 0
	return p, nil
}

const cols = `id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered`

func (r *Registry) List() ([]Project, error) {
	rows, err := r.DB.Query(`SELECT ` + cols + ` FROM projects ORDER BY flow_stage,name`)
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
func (r *Registry) Find(q string) (Project, error) {
	row := r.DB.QueryRow(`SELECT `+cols+` FROM projects WHERE slug=? OR id=? OR name=? LIMIT 1`, q, q, q)
	return scanProject(row)
}
func (r *Registry) Current() (Project, error) {
	var id string
	if err := r.DB.QueryRow(`SELECT value FROM settings WHERE key='current_project_id'`).Scan(&id); err != nil {
		return Project{}, err
	}
	return r.Find(id)
}
func (r *Registry) SetCurrent(id string) error {
	_, err := r.DB.Exec(`INSERT INTO settings(key,value) VALUES('current_project_id',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, id)
	return err
}
func (r *Registry) UpdatePathFlow(id, path, channel, flow string) error {
	res, err := r.DB.Exec(`UPDATE projects SET path=?,channel=?,flow_stage=? WHERE id=?`, canonical(path), channel, flow, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("project not found")
	}
	return nil
}
func (r *Registry) SetHealth(id string, score int) error {
	_, err := r.DB.Exec(`UPDATE projects SET health_score=? WHERE id=?`, score, id)
	return err
}
func (r *Registry) MarkMissing() error {
	_, err := r.DB.Exec(`UPDATE projects SET on_disk=0`)
	return err
}
func (r *Registry) Resolve(q string) (Project, error) {
	if q != "" {
		return r.Find(q)
	}
	p, err := r.Current()
	if err != nil {
		return p, fmt.Errorf("no Current project; pass a project name or use rivu open <project>")
	}
	return p, nil
}
