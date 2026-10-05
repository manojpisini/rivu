package registry

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Project struct {
	ID, Name, Slug, Path, Channel, FlowStage, Language string
	Stack                                              []string
	Root                                               string
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

	// v2 — read-path indexes (P1.19).
	`CREATE INDEX IF NOT EXISTS idx_projects_flow_stage ON projects(flow_stage);
CREATE INDEX IF NOT EXISTS idx_projects_last_opened ON projects(last_opened_at);
CREATE INDEX IF NOT EXISTS idx_snapshots_project_taken ON health_snapshots(project_id,taken_at);`,

	// v3 — workspace root each project was discovered under (S-08).
	`ALTER TABLE projects ADD COLUMN root TEXT NOT NULL DEFAULT '';`,
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

var (
	updateDiscoverySQL = `UPDATE projects SET channel=?,language=?,stack=?,has_git=?,has_bank=?,has_map=?,last_scanned_at=?,root=?,on_disk=1 WHERE ` + pathMatchClause
	insertProjectSQL   = `INSERT INTO projects(id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered,root) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	markMissingSQL     = `UPDATE projects SET on_disk=0 WHERE on_disk=1`
)

// ApplyDiscovery reconciles every scanned project in one transaction with
// prepared statements: everything first flagged missing, then refreshed or
// inserted. Per-project failures come back as warnings instead of aborting
// the scan (B-02); a non-nil error means nothing was committed.
func (r *Registry) ApplyDiscovery(ps []Project) ([]error, error) {
	tx, err := r.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err = tx.Exec(markMissingSQL); err != nil {
		return nil, fmt.Errorf("mark missing: %w", err)
	}
	upd, err := tx.Prepare(updateDiscoverySQL)
	if err != nil {
		return nil, err
	}
	defer upd.Close()
	ins, err := tx.Prepare(insertProjectSQL)
	if err != nil {
		return nil, err
	}
	defer ins.Close()

	var warns []error
	for _, p := range ps {
		if err := discoverTx(tx, upd, ins, p); err != nil {
			warns = append(warns, fmt.Errorf("scan %s: %w", p.Path, err))
		}
	}
	return warns, tx.Commit()
}

func discoverTx(tx *sql.Tx, upd, ins *sql.Stmt, p Project) error {
	updated, err := updatedDiscovery(upd.Exec(discoveryUpdateArgs(p)...))
	if err != nil {
		return err
	}
	if updated {
		return nil
	}
	p.Path = canonical(p.Path)
	if p.Slug, err = uniqueSlug(tx, p.Slug, p.Path); err != nil {
		return err
	}
	st, _ := json.Marshal(p.Stack)
	_, err = ins.Exec(p.ID, p.Name, p.Slug, p.Path, p.Channel, p.FlowStage, p.Language, string(st), b(p.HasGit), b(p.HasBank), b(p.HasMap), p.HealthScore, p.CreatedAt, p.LastOpenedAt, p.LastScannedAt, b(p.OnDisk), b(p.Registered), p.Root)
	return err
}

// discoveryUpdateArgs builds the arguments for updateDiscoverySQL.
func discoveryUpdateArgs(p Project) []any {
	st, _ := json.Marshal(p.Stack)
	return []any{p.Channel, p.Language, string(st), b(p.HasGit), b(p.HasBank), b(p.HasMap), p.LastScannedAt, p.Root, canonical(p.Path)}
}

func updatedDiscovery(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
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
	_, err = tx.Exec(`INSERT INTO projects(id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered,root) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(path) DO UPDATE SET name=excluded.name,channel=excluded.channel,flow_stage=excluded.flow_stage,language=excluded.language,stack=excluded.stack,has_git=excluded.has_git,has_bank=excluded.has_bank,has_map=excluded.has_map,last_scanned_at=excluded.last_scanned_at,on_disk=1,registered=1,root=excluded.root`, p.ID, p.Name, p.Slug, p.Path, p.Channel, p.FlowStage, p.Language, string(st), b(p.HasGit), b(p.HasBank), b(p.HasMap), p.HealthScore, p.CreatedAt, p.LastOpenedAt, p.LastScannedAt, b(p.OnDisk), b(p.Registered), p.Root)
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
	err := s.Scan(&p.ID, &p.Name, &p.Slug, &p.Path, &p.Channel, &p.FlowStage, &p.Language, &st, &hg, &hb, &hm, &p.HealthScore, &p.CreatedAt, &p.LastOpenedAt, &p.LastScannedAt, &od, &reg, &p.Root)
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

const cols = `id,name,slug,path,channel,flow_stage,language,stack,has_git,has_bank,has_map,health_score,created_at,last_opened_at,last_scanned_at,on_disk,registered,root`

func (r *Registry) List() ([]Project, error) {
	// Lifecycle order: source, active, maintenance, research, delta (spec 1.2.1).
	rows, err := r.DB.Query(`SELECT ` + cols + ` FROM projects ORDER BY CASE flow_stage WHEN 'source' THEN 0 WHEN 'active' THEN 1 WHEN 'maintenance' THEN 2 WHEN 'research' THEN 3 WHEN 'delta' THEN 4 ELSE 5 END, name`)
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

// ChannelForFlow maps a Flow stage to its workspace channel folder (spec 3.1).
func ChannelForFlow(flow string) string {
	switch flow {
	case "active":
		return "01_Active"
	case "maintenance":
		return "02_Maintenance"
	case "research":
		return "03_Research"
	case "delta":
		return "90_Delta"
	}
	return "00_Source"
}

// FlowForChannel maps a workspace channel folder to its default Flow stage.
func FlowForChannel(c string) string {
	switch c {
	case "01_Active":
		return "active"
	case "02_Maintenance":
		return "maintenance"
	case "03_Research":
		return "research"
	case "90_Delta":
		return "delta"
	}
	return "source"
}

// States groups projects into the three registry/filesystem mismatch
// categories of spec 1.4.2. Never silently resolved — only reported.
type States struct {
	Missing       []Project // registered anywhere but not on disk anymore
	Unregistered  []Project // on disk, not yet registered with rivu
	StageMismatch []Project // flow_stage disagrees with the channel folder
}

// States reports the current mismatch states across all projects.
func (r *Registry) States() (States, error) {
	ps, err := r.List()
	if err != nil {
		return States{}, err
	}
	var s States
	for _, p := range ps {
		switch {
		case !p.OnDisk:
			s.Missing = append(s.Missing, p)
		case !p.Registered:
			s.Unregistered = append(s.Unregistered, p)
		case FlowForChannel(p.Channel) != p.FlowStage:
			s.StageMismatch = append(s.StageMismatch, p)
		}
	}
	return s, nil
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

// ApplyFlow records a completed Flow move in one transaction: path,
// channel, stage, asset flags and the activity log (P1.43).
func (r *Registry) ApplyFlow(id, path, channel, flow string, hasBank, hasMap bool) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE projects SET path=?,channel=?,flow_stage=?,has_bank=?,has_map=? WHERE id=?`,
		canonical(path), channel, flow, b(hasBank), b(hasMap), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("project not found")
	}
	if _, err := tx.Exec(`INSERT INTO activity_log(id,project_id,event,occurred_at) VALUES(?,?,?,?)`,
		uuid.NewString(), id, "flowed", time.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateFlags refreshes has_bank/has_map immediately after Bank or Map
// work (B-06) instead of waiting for the next scan to rediscover them.
func (r *Registry) UpdateFlags(id string, hasBank, hasMap bool) error {
	_, err := r.DB.Exec(`UPDATE projects SET has_bank=?,has_map=? WHERE id=?`, b(hasBank), b(hasMap), id)
	return err
}

// SlugOrPathTaken reports whether any project already owns this slug or
// path — the Source preflight check (O-01).
func (r *Registry) SlugOrPathTaken(slug, path string) (bool, error) {
	var n int
	err := r.DB.QueryRow(`SELECT count(*) FROM projects WHERE slug=? OR `+pathMatchClause, slug, canonical(path)).Scan(&n)
	return n > 0, err
}

// MarkOpened records that a project was opened just now (R-09).
func (r *Registry) MarkOpened(id string) error {
	_, err := r.DB.Exec(`UPDATE projects SET last_opened_at=? WHERE id=?`, time.Now(), id)
	return err
}

// MarkMissing flags one project as absent from disk, so lists show the
// missing mismatch until the next scan finds it again (E-03).
func (r *Registry) MarkMissing(id string) error {
	_, err := r.DB.Exec(`UPDATE projects SET on_disk=0 WHERE id=?`, id)
	return err
}

// LogActivity appends an event to activity_log (R-08). Events are short
// vocabulary words: opened, sourced, flowed, health, map.
func (r *Registry) LogActivity(projectID, event string) error {
	_, err := r.DB.Exec(`INSERT INTO activity_log(id,project_id,event,occurred_at) VALUES(?,?,?,?)`,
		uuid.NewString(), projectID, event, time.Now())
	return err
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

// Typed sentinel errors for project lookup.
var (
	ErrNotFound  = errors.New("project not found")
	ErrAmbiguous = errors.New("ambiguous project reference")
)

func (r *Registry) Find(q string) (Project, error) {
	row := r.DB.QueryRow(`SELECT `+cols+` FROM projects WHERE slug=? OR id=? OR name=? LIMIT 1`, q, q, q)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, fmt.Errorf("%w: no project matches %q", ErrNotFound, q)
	}
	return p, err
}

// Resolve finds a project for a user-supplied query. With an empty query it
// returns Current. Otherwise it walks a ladder: exact slug, exact id, unique
// id prefix, case-insensitive name, unique prefix, then fuzzy substring.
// Exactly one match returns the project; several return ErrAmbiguous with
// the candidates listed; none return ErrNotFound with a did-you-mean hint.
func (r *Registry) Resolve(q string) (Project, error) {
	if q == "" {
		p, err := r.Current()
		if errors.Is(err, ErrNotFound) {
			return Project{}, err
		}
		if err != nil {
			return Project{}, fmt.Errorf("no Current project; pass a project name or use rivu open <project>")
		}
		return p, nil
	}
	ps, err := r.List()
	if err != nil {
		return Project{}, err
	}
	qf := strings.ToLower(q)
	for _, match := range []func(Project) bool{
		func(p Project) bool { return p.Slug == q },
		func(p Project) bool { return p.ID == q },
		func(p Project) bool { return strings.HasPrefix(p.ID, q) },
		func(p Project) bool { return strings.EqualFold(p.Name, q) },
		func(p Project) bool {
			return strings.HasPrefix(p.Slug, qf) || strings.HasPrefix(p.ID, q) || strings.HasPrefix(strings.ToLower(p.Name), qf)
		},
	} {
		var hits []Project
		for _, p := range ps {
			if match(p) {
				hits = append(hits, p)
			}
		}
		switch len(hits) {
		case 1:
			return hits[0], nil
		case 0:
			// fall through to the next rung
		default:
			return Project{}, ambiguousErr(q, hits)
		}
	}
	var hits []Project
	for _, p := range ps {
		if strings.Contains(strings.ToLower(p.Slug), qf) || strings.Contains(strings.ToLower(p.Name), qf) {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return Project{}, fmt.Errorf("%w: no project matches %q%s", ErrNotFound, q, didYouMean(qf, ps))
	default:
		return Project{}, ambiguousErr(q, hits)
	}
}

func ambiguousErr(q string, hits []Project) error {
	names := make([]string, 0, len(hits))
	for _, p := range hits {
		names = append(names, p.Slug)
	}
	sort.Strings(names)
	return fmt.Errorf("%w: %q matches %d projects: %s", ErrAmbiguous, q, len(hits), strings.Join(names, ", "))
}

// didYouMean suggests up to three slugs within edit distance 2 of q.
func didYouMean(qf string, ps []Project) string {
	type cand struct {
		slug string
		d    int
	}
	var cs []cand
	for _, p := range ps {
		if d := levenshtein(qf, strings.ToLower(p.Slug)); d <= 2 {
			cs = append(cs, cand{p.Slug, d})
		}
	}
	if len(cs) == 0 {
		return ""
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
	if len(cs) > 3 {
		cs = cs[:3]
	}
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.slug
	}
	return fmt.Sprintf(" — did you mean: %s?", strings.Join(names, ", "))
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
