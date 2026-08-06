package registry

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	r := &Registry{DB: db}
	if err = r.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return r, nil
}
func (r *Registry) Close() error { return r.DB.Close() }
func (r *Registry) migrate() error {
	_, err := r.DB.Exec(`PRAGMA foreign_keys=ON; CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY,name TEXT NOT NULL,slug TEXT NOT NULL UNIQUE,path TEXT NOT NULL UNIQUE,channel TEXT NOT NULL,flow_stage TEXT NOT NULL,language TEXT,stack TEXT,has_git INTEGER DEFAULT 0,has_bank INTEGER DEFAULT 0,has_map INTEGER DEFAULT 0,health_score INTEGER DEFAULT 0,created_at DATETIME,last_opened_at DATETIME,last_scanned_at DATETIME,on_disk INTEGER DEFAULT 1,registered INTEGER DEFAULT 1); CREATE TABLE IF NOT EXISTS settings(key TEXT PRIMARY KEY,value TEXT); CREATE TABLE IF NOT EXISTS confluences(id TEXT PRIMARY KEY,name TEXT NOT NULL UNIQUE,notes TEXT); CREATE TABLE IF NOT EXISTS project_confluences(project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,confluence_id TEXT REFERENCES confluences(id) ON DELETE CASCADE,PRIMARY KEY(project_id,confluence_id)); CREATE TABLE IF NOT EXISTS health_snapshots(id TEXT PRIMARY KEY,project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,score INTEGER,taken_at DATETIME); CREATE TABLE IF NOT EXISTS activity_log(id TEXT PRIMARY KEY,project_id TEXT,event TEXT,occurred_at DATETIME);`)
	return err
}
func b(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (r *Registry) Upsert(p Project) error {
	st, _ := json.Marshal(p.Stack)
	_, err := r.DB.Exec(`INSERT INTO projects(id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET name=excluded.name,slug=excluded.slug,channel=excluded.channel,flow_stage=excluded.flow_stage,language=excluded.language,stack=excluded.stack,has_git=excluded.has_git,has_bank=excluded.has_bank,has_map=excluded.has_map,health_score=excluded.health_score,last_scanned_at=excluded.last_scanned_at,on_disk=1,registered=1`, p.ID, p.Name, p.Slug, p.Path, p.Channel, p.FlowStage, p.Language, string(st), b(p.HasGit), b(p.HasBank), b(p.HasMap), p.HealthScore, p.CreatedAt, p.LastOpenedAt, p.LastScannedAt, b(p.OnDisk), b(p.Registered))
	return err
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
	res, err := r.DB.Exec(`UPDATE projects SET path=?,channel=?,flow_stage=? WHERE id=?`, path, channel, flow, id)
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
