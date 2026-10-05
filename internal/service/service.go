package service

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/manojpisini/rivu/internal/bank"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/editorlaunch"
	"github.com/manojpisini/rivu/internal/mapgen"
	"github.com/manojpisini/rivu/internal/pathsafe"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/scanner"
	"github.com/manojpisini/rivu/internal/slug"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
)

type App struct {
	Config   config.Config
	Registry *registry.Registry
	// ConfigWarnings lists unrecognized config keys found at load time.
	ConfigWarnings []string
}

// Service is the use-case surface the TUI and CLI depend on: typed
// results, never printing (spec 1.4.4 — every TUI action has a CLI
// equivalent over the same layer). *App implements it; tests use the
// hand-written fake in internal/service/fake.
type Service interface {
	List(f Filter) ([]registry.Project, error)
	Scan() (ScanResult, error)
	Doctor(q string) ([]doctor.Report, error)
	Source(name string, o SourceOpts) (SourceResult, error)
	Flow(q, to string, flatten, dry bool) (FlowResult, error)
	Map(q string) error
	OpenProject(q, editor string) error
	Current() (registry.Project, bool)
}

var _ Service = (*App)(nil)

func Open() (*App, error) {
	c, warns, e := config.Load()
	if e != nil {
		return nil, e
	}
	if e = config.Validate(c); e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(c.Data.DBPath), 0755); e != nil {
		return nil, e
	}
	r, e := registry.Open(c.Data.DBPath)
	if e != nil {
		return nil, e
	}
	return &App{Config: c, Registry: r, ConfigWarnings: warns}, nil
}
func (a *App) Close() error { return a.Registry.Close() }

// Current returns the current project and whether one is set.
func (a *App) Current() (registry.Project, bool) {
	p, err := a.Registry.Current()
	if err != nil {
		return registry.Project{}, false
	}
	return p, true
}
func (a *App) Scan() (ScanResult, error) {
	if e := config.ValidateRoot(a.Config); e != nil {
		return ScanResult{}, e
	}
	s := scanner.New(a.Config.Scanner.Ignore, a.Config.Scanner.MaxDepth)
	var all []registry.Project
	var warnings []string
	roots := append([]string{a.Config.Workspace.Root}, a.Config.Workspace.SecondaryRoots...)
	for i, root := range roots {
		ps, scanWarns, e := s.Scan(root)
		for _, w := range scanWarns {
			warnings = append(warnings, w.Error())
		}
		if e != nil {
			if i == 0 {
				return ScanResult{}, e
			}
			warnings = append(warnings, fmt.Sprintf("secondary root skipped: %v", e))
			continue
		}
		all = append(all, ps...)
	}
	warns, e := a.Registry.ApplyDiscovery(all)
	for _, w := range warns {
		warnings = append(warnings, w.Error())
	}
	if e != nil {
		return ScanResult{}, fmt.Errorf("scan could not be committed: %w", e)
	}
	return ScanResult{Projects: all, Warnings: warnings}, nil
}
func channel(flow string) string { return registry.ChannelForFlow(flow) }

// Stages are the five Flow stages in lifecycle order. validFlow, the
// CLI's --flow validation and stats all read this one list.
var Stages = []string{"source", "active", "maintenance", "research", "delta"}

func validFlow(s string) bool { return slices.Contains(Stages, s) }

// SourceOpts are the Source inputs from the CLI flags and the TUI
// wizard (O-04). The zero value is a plain `source`-stage project with
// git init.
type SourceOpts struct {
	Flow        string // initial stage; "" means "source"
	Git         bool   // run git init
	Adopt       bool   // register an existing directory as-is
	Dry         bool   // plan only, write nothing
	Domain      string // extra folder level: Channel/Domain/slug
	Type        string // project classification, stored in project.toml
	Language    string // registry language override
	Template    string // starter template name ("" uses [templates].default)
	Description string // one-line purpose, stored in project.toml
	Confluence  []string
	Bridge      bool // bridge owns git init for this project (spec 1.7)
}

// Source creates a structured project, or with adopt registers an existing
// directory as-is (Bank only — existing files are never touched, O-06).
func (a *App) Source(name string, o SourceOpts) (SourceResult, error) {
	if name == "" {
		return SourceResult{}, fmt.Errorf("name is required")
	}
	flow := o.Flow
	if flow == "" {
		flow = "source"
	}
	if !validFlow(flow) {
		return SourceResult{}, fmt.Errorf("invalid flow stage %q", flow)
	}
	s, e := slug.Make(name)
	if e != nil {
		return SourceResult{}, fmt.Errorf("invalid project name: %w", e)
	}
	ch := channel(flow)
	base := filepath.Join(a.Config.Workspace.Root, ch)
	dir := base
	if o.Domain != "" {
		d, err := slug.Make(o.Domain)
		if err != nil {
			return SourceResult{}, fmt.Errorf("invalid domain: %w", err)
		}
		dir = filepath.Join(base, d)
	}
	path := filepath.Join(dir, s)
	if !pathsafe.Contained(base, path) {
		return SourceResult{}, fmt.Errorf("destination %s escapes channel folder %s", path, base)
	}
	p := registry.Project{ID: uuid.NewString(), Name: name, Slug: s, Path: path, Channel: ch, FlowStage: flow, Language: o.Language, CreatedAt: time.Now(), LastScannedAt: time.Now(), OnDisk: true, Registered: true}

	// Spec 1.7 exclusivity: when the bridge owns git init, rivu never runs
	// its own — warn and auto-correct rather than double-init.
	// Adopt registers as-is: no git init, no Map, just Bank (O-06).
	gitInit := o.Git
	var warnings []string
	if o.Adopt && gitInit {
		gitInit = false
		warnings = append(warnings,
			"--adopt registers and writes the Bank only — skipped git init for "+s)
	}
	bridgeOwns := o.Bridge || (a.Config.Automation.BridgeOwnsGitInit && a.Config.Bridge.Enabled)
	gitOwner := "none"
	if bridgeOwns {
		gitOwner = "bridge"
		if gitInit {
			gitInit = false
			why := "automation.bridge_owns_git_init"
			if o.Bridge {
				why = "--bridge"
			}
			warnings = append(warnings,
				"bridge owns git init ("+why+") — skipped rivu git init for "+s)
		}
	} else if gitInit {
		gitOwner = "rivu"
	}

	// Preflight (O-01): every check runs before the first write, so a
	// rejected Source never leaves half-created folders behind.
	plan := SourcePlan{Name: name, Slug: s, Channel: ch, FlowStage: flow, ChannelDir: dir, Path: path}
	if taken, err := a.Registry.SlugOrPathTaken(s, path); err != nil {
		return SourceResult{}, err
	} else if taken {
		return SourceResult{}, fmt.Errorf("slug %q or path %s is already registered — pick another name, or open the existing project", s, path)
	}
	createdRoot, wasEmpty, createdDomain := false, false, false
	if o.Domain != "" {
		if _, err := os.Lstat(dir); os.IsNotExist(err) {
			createdDomain = true
		}
	}
	switch fi, err := os.Lstat(path); {
	case err == nil && !fi.IsDir():
		return SourceResult{}, fmt.Errorf("%s exists and is not a directory", path)
	case err == nil:
		entries, rerr := os.ReadDir(path)
		if rerr != nil {
			return SourceResult{}, rerr
		}
		if len(entries) > 0 && !o.Adopt {
			return SourceResult{}, fmt.Errorf("%s already exists and is not empty — re-run with --adopt to register it as-is", path)
		}
		wasEmpty = len(entries) == 0
	case os.IsNotExist(err):
		createdRoot = true
	default:
		return SourceResult{}, err
	}
	if gitInit {
		if _, err := exec.LookPath("git"); err != nil {
			return SourceResult{}, fmt.Errorf("git is not on PATH — install git or drop --git: %w", err)
		}
		plan.Run = append(plan.Run, "git init")
		plan.Write = append(plan.Write, ".gitignore")
	}
	if createdRoot {
		plan.Create = append(plan.Create, path)
	}
	if a.Config.Automation.CreateBank {
		plan.Bank = append(plan.Bank, ".metadata/project.toml", ".metadata/overview.md", ".metadata/decisions.md", ".metadata/tasks.md")
	}
	if a.Config.Automation.BuildMap && !o.Adopt {
		plan.Write = append(plan.Write, ".metadata/agent/AGENTS.md", ".metadata/agent/PROJECT_MAP.md")
	}
	plan.Registry = append(plan.Registry, "register "+s)

	if o.Dry {
		return SourceResult{Project: p, Plan: plan, Warnings: warnings}, nil
	}
	gitRan, metaRan := false, false
	fail := func(err error) (SourceResult, error) {
		rollbackSource(path, dir, createdRoot, createdDomain, wasEmpty, gitRan, metaRan)
		return SourceResult{}, err
	}
	if createdRoot {
		if e := os.MkdirAll(path, 0755); e != nil {
			return fail(e)
		}
	}
	tmpl := o.Template
	if tmpl == "" {
		tmpl = a.Config.Templates.Default
	}
	if gitInit {
		gitRan = true
		branch := a.Config.Git.DefaultBranch
		if branch == "" {
			branch = "main"
		}
		cmd := exec.Command("git", "init", "-b", branch)
		cmd.Dir = path
		if out, e := cmd.CombinedOutput(); e != nil {
			return fail(fmt.Errorf("git init: %w: %s", e, out))
		}
		p.HasGit = true
		// Starter .gitignore for the template — only when missing, the
		// user owns it once created.
		gi := filepath.Join(path, ".gitignore")
		if _, e := os.Stat(gi); os.IsNotExist(e) {
			if e := os.WriteFile(gi, []byte(gitignoreFor(tmpl)), 0644); e != nil {
				return fail(fmt.Errorf("write .gitignore: %w", e))
			}
		}
	}
	if a.Config.Automation.CreateBank {
		metaRan = true
		meta := bank.Meta{Domain: o.Domain, Type: o.Type, Template: o.Template, Description: o.Description, Confluences: o.Confluence}
		if e := bank.Build(p, gitOwner, meta); e != nil {
			return fail(e)
		}
		p.HasBank = true
	}
	if a.Config.Automation.BuildMap && !o.Adopt {
		metaRan = true
		if e := mapgen.Build(p, a.Config.Scanner.Ignore); e != nil {
			return fail(e)
		}
		p.HasMap = true
	}
	if e := a.Registry.Upsert(p); e != nil {
		return fail(fmt.Errorf("register project: %w", e))
	}
	for _, c := range o.Confluence {
		if e := a.Registry.AttachConfluence(p.ID, c); e != nil {
			warnings = append(warnings, fmt.Sprintf("could not attach confluence %q: %v", c, e))
		}
	}
	_ = a.Registry.SetCurrent(p.ID)
	_ = a.Registry.LogActivity(p.ID, "sourced")
	return SourceResult{Project: p, Plan: plan, Warnings: warnings}, nil
}

// rollbackSource removes only what this Source run created: the project
// directory if this run made it (plus the domain folder, if this run
// made that and it is now empty), otherwise just .metadata/.git inside a
// directory that was empty at preflight. A directory with pre-existing
// content (adopt mode) is never touched — safety rule 1.
func rollbackSource(path, dir string, createdRoot, createdDomain, wasEmpty, gitRan, metaRan bool) {
	if createdRoot {
		_ = os.RemoveAll(path) // rivu-allow-remove: root this run created
		if createdDomain {
			_ = os.Remove(dir) // rivu-allow-remove: empty domain folder this run created
		}
		return
	}
	if !wasEmpty {
		return
	}
	if metaRan {
		_ = os.RemoveAll(filepath.Join(path, ".metadata")) // rivu-allow-remove: bank this run created
	}
	if gitRan {
		_ = os.RemoveAll(filepath.Join(path, ".git"))    // rivu-allow-remove: git dir this run created
		_ = os.Remove(filepath.Join(path, ".gitignore")) // rivu-allow-remove: gitignore this run created
	}
}

// gitignoreFor returns starter .gitignore content for a Source template
// (O-05). Template names are split into tokens ("go-cli" → go, cli) and the
// first ecosystem token wins; unknown or empty names get a generic list.
func gitignoreFor(template string) string {
	goList := "# Rivu starter (.gitignore)\n.env\n.env.*\n!.env.example\n*.exe\n*.test\n*.out\n\n# build output\n/bin/\n/dist/\n\n# deps\nvendor/\n"
	nodeList := "# Rivu starter (.gitignore)\n.env\n.env.*\n!.env.example\n\n# dependencies and build\nnode_modules/\ndist/\nbuild/\ncoverage/\n\n# logs and OS\n*.log\n.DS_Store\nThumbs.db\n"
	pyList := "# Rivu starter (.gitignore)\n.env\n.env.*\n!.env.example\n\n# python\n__pycache__/\n*.pyc\n.venv/\nvenv/\ndist/\n*.egg-info/\n\n# logs and OS\n*.log\n.DS_Store\n"
	generic := "# Rivu starter (.gitignore)\n.env\n.env.*\n!.env.example\n\n# dependencies and build\nnode_modules/\nvendor/\ndist/\nbuild/\n__pycache__/\n.venv/\n\n# logs and OS\n*.log\n.DS_Store\nThumbs.db\n"
	tokens := strings.FieldsFunc(strings.ToLower(template), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	for _, w := range tokens {
		switch w {
		case "go", "golang":
			return goList
		case "python", "py":
			return pyList
		case "node", "nodejs", "ts", "typescript", "react", "web", "next":
			return nodeList
		}
	}
	return generic
}

// Flow moves a project to another Flow stage: preflight → rename → bank
// sync → one registry transaction; any failure undoes the rename so the
// registry never points at a path that no longer exists (B-08). The
// returned FlowResult carries the applied plan and a human note.
func (a *App) Flow(q, to string, flatten, dry bool) (FlowResult, error) {
	if !validFlow(to) {
		return FlowResult{}, fmt.Errorf("invalid flow stage %q", to)
	}
	p, e := a.Registry.Resolve(q)
	if e != nil {
		return FlowResult{}, e
	}
	if p.FlowStage == to {
		return FlowResult{Project: p, Note: fmt.Sprintf("%s is already in %s — nothing to move", p.Name, to)}, nil
	}

	// Root this project lives under; rows predating P1.29 fall back to the
	// primary workspace root. EvalSymlinks folds 8.3 short names and
	// symlinked dirs so root and the canonical registry path agree.
	root := p.Root
	if root == "" {
		root = a.Config.Workspace.Root
	}
	if !filepath.IsAbs(root) {
		root = filepath.Join(a.Config.Workspace.Root, root)
	}
	root = resolved(root)
	// Relative to the project's current channel folder, so sub-folders are
	// preserved under the destination channel (P1.44).
	rel := filepath.Base(p.Path)
	if !flatten {
		if r, err := filepath.Rel(filepath.Join(root, p.Channel), p.Path); err == nil && r != "." && !strings.HasPrefix(r, "..") {
			rel = r
		}
	}
	ch := channel(to)
	dest := filepath.Join(root, ch, rel)

	// Preflight (P1.42): every hazard checked before anything moves.
	fi, err := os.Lstat(p.Path)
	if err != nil {
		return FlowResult{}, fmt.Errorf("project folder is not on disk — run `rivu scan` to reconcile: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return FlowResult{}, fmt.Errorf("%s is a symlink — move it manually, then rescan", p.Path)
	}
	if !fi.IsDir() {
		return FlowResult{}, fmt.Errorf("%s is not a directory", p.Path)
	}
	if !a.knownRoot(p.Path) {
		return FlowResult{}, fmt.Errorf("%s is outside the configured workspace roots — add its root to workspace.root or secondary_roots first", p.Path)
	}
	switch _, err := os.Lstat(dest); {
	case err == nil:
		return FlowResult{}, fmt.Errorf("destination %s already exists — move it out of the way first", dest)
	case !os.IsNotExist(err):
		return FlowResult{}, err
	}
	if filepath.VolumeName(p.Path) != filepath.VolumeName(dest) {
		return FlowResult{}, fmt.Errorf("flow cannot cross volumes: %s and %s", p.Path, dest)
	}
	if !pathsafe.Contained(filepath.Join(root, ch), dest) {
		return FlowResult{}, fmt.Errorf("destination %s escapes channel folder %s", dest, filepath.Join(root, ch))
	}
	plan := FlowPlan{ID: p.ID, Query: q, FromStage: p.FlowStage, ToStage: to, Src: p.Path, Dst: dest, Root: root, Flatten: flatten,
		Move:     []string{p.Path + " -> " + dest},
		Bank:     []string{".metadata/project.toml"},
		Registry: []string{"update path, stage and flags for " + p.Slug}}

	if dry {
		return FlowResult{Project: p, Plan: plan, Note: fmt.Sprintf("DRY RUN: would move %s: %s -> %s", p.Name, p.Path, dest)}, nil
	}
	if e := os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
		return FlowResult{}, e
	}
	orig := p
	moved := orig.Path != dest
	if moved {
		if e := os.Rename(orig.Path, dest); e != nil {
			return FlowResult{}, fmt.Errorf("move failed: %w", e)
		}
	}
	fail := func(err error) (FlowResult, error) {
		if moved {
			if re := os.Rename(dest, orig.Path); re != nil {
				return FlowResult{}, fmt.Errorf("%w (restoring %s also failed: %v)", err, orig.Path, re)
			}
		}
		_ = bank.Sync(orig, "rivu") // put the old stage back into the moved-back file
		return FlowResult{}, err
	}
	p.Path, p.Channel, p.FlowStage = dest, ch, to
	if e := bank.Sync(p, "rivu"); e != nil {
		return fail(fmt.Errorf("update bank: %w", e))
	}
	if e := a.Registry.ApplyFlow(p.ID, dest, ch, to, true, p.HasMap); e != nil {
		return fail(fmt.Errorf("record move: %w", e))
	}
	return FlowResult{Project: p, Plan: plan, Note: fmt.Sprintf("Flowed %s: %s -> %s", p.Name, orig.Path, dest)}, nil
}

// knownRoot reports whether target sits inside any configured workspace root.
func (a *App) knownRoot(target string) bool {
	target = resolved(target)
	roots := append([]string{a.Config.Workspace.Root}, a.Config.Workspace.SecondaryRoots...)
	for _, r := range roots {
		if pathsafe.Contained(resolved(r), target) {
			return true
		}
	}
	return false
}

// resolved folds symlinks and Windows 8.3 short names when the path exists,
// matching how the registry canonicalises stored paths.
func resolved(p string) string {
	if x, err := filepath.EvalSymlinks(p); err == nil {
		return x
	}
	return p
}
func (a *App) Doctor(q string) ([]doctor.Report, error) {
	var ps []registry.Project
	if q != "" {
		p, e := a.Registry.Resolve(q)
		if e != nil {
			return nil, e
		}
		ps = []registry.Project{p}
	} else {
		var e error
		ps, e = a.Registry.List()
		if e != nil {
			return nil, e
		}
	}
	out := make([]doctor.Report, 0, len(ps))
	for _, p := range ps {
		r := doctor.Run(p, a.Config.Bridge.ScaffoldMark)
		_ = a.Registry.SetHealth(p.ID, r.Score)
		out = append(out, r)
	}
	return out, nil
}
func (a *App) Map(q string) error {
	p, e := a.Registry.Resolve(q)
	if e != nil {
		return e
	}
	if e = bank.Build(p, "rivu", bank.Meta{}); e != nil {
		return e
	}
	if e = mapgen.Build(p, a.Config.Scanner.Ignore); e != nil {
		return e
	}
	return a.Registry.UpdateFlags(p.ID, true, true)
}

// OpenProject opens q (or Current when empty) in editor, or the
// resolved default when editor is "".
func (a *App) OpenProject(q, editor string) error {
	p, e := a.Registry.Resolve(q)
	if e != nil {
		return e
	}
	// E-03: never open a vanished path — flag it missing and point at scan
	// instead of launching an editor at nothing.
	if _, err := os.Stat(p.Path); err != nil {
		_ = a.Registry.MarkMissing(p.ID)
		return fmt.Errorf("%w: %s is not on disk at %s — run `rivu scan` to refresh the registry, or restore the folder", registry.ErrNotFound, p.Slug, p.Path)
	}
	_ = a.Registry.SetCurrent(p.ID)
	_ = a.Registry.MarkOpened(p.ID)
	_ = a.Registry.LogActivity(p.ID, "opened")
	if editor == "" {
		editor = editorlaunch.Resolve(a.Config, p.Language)
	}
	args, err := editorlaunch.Parse(editor)
	if err != nil {
		return err
	}
	cmd := exec.Command(args[0], append(args[1:], p.Path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// B-05: terminal editors need the TTY — Start() would return while vim
	// still owns it. GUI editors are fire-and-forget (Start), terminal
	// editors block until the user exits (Run).
	if editorlaunch.IsGUI(editor, a.Config.Editors.GUI) {
		return cmd.Start()
	}
	return cmd.Run()
}
