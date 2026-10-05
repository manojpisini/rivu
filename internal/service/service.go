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
	"strings"
	"time"
	"unicode"
)

type App struct {
	Config   config.Config
	Registry *registry.Registry
	// ConfigWarnings lists unrecognized config keys found at load time.
	ConfigWarnings []string
	// ScanWarnings lists per-project failures from the last Scan.
	ScanWarnings []string
	// SourceWarnings lists corrections applied by the last Source, such as
	// git init being skipped because the bridge owns it (spec 1.7).
	SourceWarnings []string
}

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
func (a *App) Scan() ([]registry.Project, error) {
	if e := config.ValidateRoot(a.Config); e != nil {
		return nil, e
	}
	s := scanner.New(a.Config.Scanner.Ignore, a.Config.Scanner.MaxDepth)
	var all []registry.Project
	a.ScanWarnings = nil
	roots := append([]string{a.Config.Workspace.Root}, a.Config.Workspace.SecondaryRoots...)
	for i, root := range roots {
		ps, scanWarns, e := s.Scan(root)
		for _, w := range scanWarns {
			a.ScanWarnings = append(a.ScanWarnings, w.Error())
		}
		if e != nil {
			if i == 0 {
				return nil, e
			}
			a.ScanWarnings = append(a.ScanWarnings, fmt.Sprintf("secondary root skipped: %v", e))
			continue
		}
		all = append(all, ps...)
	}
	warns, e := a.Registry.ApplyDiscovery(all)
	for _, w := range warns {
		a.ScanWarnings = append(a.ScanWarnings, w.Error())
	}
	if e != nil {
		return nil, fmt.Errorf("scan could not be committed: %w", e)
	}
	return all, nil
}
func channel(flow string) string { return registry.ChannelForFlow(flow) }
func validFlow(s string) bool {
	switch s {
	case "source", "active", "maintenance", "research", "delta":
		return true
	}
	return false
}

// Source creates a structured project, or with adopt registers an existing
// directory as-is (Bank only — existing files are never touched, O-06).
func (a *App) Source(name, flow string, gitInit, adopt, dry bool) (registry.Project, error) {
	if name == "" {
		return registry.Project{}, fmt.Errorf("name is required")
	}
	if flow == "" {
		flow = "source"
	}
	if !validFlow(flow) {
		return registry.Project{}, fmt.Errorf("invalid flow stage %q", flow)
	}
	s, e := slug.Make(name)
	if e != nil {
		return registry.Project{}, fmt.Errorf("invalid project name: %w", e)
	}
	ch := channel(flow)
	chDir := filepath.Join(a.Config.Workspace.Root, ch)
	path := filepath.Join(chDir, s)
	if !pathsafe.Contained(chDir, path) {
		return registry.Project{}, fmt.Errorf("destination %s escapes channel folder %s", path, chDir)
	}
	p := registry.Project{ID: uuid.NewString(), Name: name, Slug: s, Path: path, Channel: ch, FlowStage: flow, CreatedAt: time.Now(), LastScannedAt: time.Now(), OnDisk: true, Registered: true}

	// Spec 1.7 exclusivity: when the bridge owns git init, rivu never runs
	// its own — warn and auto-correct rather than double-init.
	// Adopt registers as-is: no git init, no Map, just Bank (O-06).
	a.SourceWarnings = nil
	if adopt && gitInit {
		gitInit = false
		a.SourceWarnings = append(a.SourceWarnings,
			"--adopt registers and writes the Bank only — skipped git init for "+s)
	}
	bridgeOwns := a.Config.Automation.BridgeOwnsGitInit && a.Config.Bridge.Enabled
	gitOwner := "none"
	if bridgeOwns {
		gitOwner = "bridge"
		if gitInit {
			gitInit = false
			a.SourceWarnings = append(a.SourceWarnings,
				"bridge owns git init (automation.bridge_owns_git_init) — skipped rivu git init for "+s)
		}
	} else if gitInit {
		gitOwner = "rivu"
	}

	// Preflight (O-01): every check runs before the first write, so a
	// rejected Source never leaves half-created folders behind.
	plan := SourcePlan{Name: name, Slug: s, Channel: ch, FlowStage: flow, ChannelDir: chDir, Path: path}
	if taken, err := a.Registry.SlugOrPathTaken(s, path); err != nil {
		return p, err
	} else if taken {
		return p, fmt.Errorf("slug %q or path %s is already registered — pick another name, or open the existing project", s, path)
	}
	createdRoot, wasEmpty := false, false
	switch fi, err := os.Lstat(path); {
	case err == nil && !fi.IsDir():
		return p, fmt.Errorf("%s exists and is not a directory", path)
	case err == nil:
		entries, rerr := os.ReadDir(path)
		if rerr != nil {
			return p, rerr
		}
		if len(entries) > 0 && !adopt {
			return p, fmt.Errorf("%s already exists and is not empty — re-run with --adopt to register it as-is", path)
		}
		wasEmpty = len(entries) == 0
	case os.IsNotExist(err):
		createdRoot = true
	default:
		return p, err
	}
	if gitInit {
		if _, err := exec.LookPath("git"); err != nil {
			return p, fmt.Errorf("git is not on PATH — install git or drop --git: %w", err)
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
	if a.Config.Automation.BuildMap && !adopt {
		plan.Write = append(plan.Write, ".metadata/agent/AGENTS.md", ".metadata/agent/PROJECT_MAP.md")
	}
	plan.Registry = append(plan.Registry, "register "+s)

	if dry {
		return p, nil
	}
	gitRan, metaRan := false, false
	fail := func(err error) (registry.Project, error) {
		rollbackSource(path, createdRoot, wasEmpty, gitRan, metaRan)
		return p, err
	}
	if createdRoot {
		if e := os.MkdirAll(path, 0755); e != nil {
			return fail(e)
		}
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
			if e := os.WriteFile(gi, []byte(gitignoreFor(a.Config.Templates.Default)), 0644); e != nil {
				return fail(fmt.Errorf("write .gitignore: %w", e))
			}
		}
	}
	if a.Config.Automation.CreateBank {
		metaRan = true
		if e := bank.Build(p, gitOwner); e != nil {
			return fail(e)
		}
		p.HasBank = true
	}
	if a.Config.Automation.BuildMap && !adopt {
		metaRan = true
		if e := mapgen.Build(p, a.Config.Scanner.Ignore); e != nil {
			return fail(e)
		}
		p.HasMap = true
	}
	if e := a.Registry.Upsert(p); e != nil {
		return fail(fmt.Errorf("register project: %w", e))
	}
	_ = a.Registry.SetCurrent(p.ID)
	_ = a.Registry.LogActivity(p.ID, "sourced")
	return p, nil
}

// rollbackSource removes only what this Source run created: the project
// directory if this run made it, otherwise just .metadata/.git inside a
// directory that was empty at preflight. A directory with pre-existing
// content (adopt mode) is never touched — safety rule 1.
func rollbackSource(path string, createdRoot, wasEmpty, gitRan, metaRan bool) {
	if createdRoot {
		_ = os.RemoveAll(path) // rivu-allow-remove: root this run created
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
// returned string is a human note for the caller to print.
func (a *App) Flow(q, to string, flatten, dry bool) (registry.Project, string, error) {
	if !validFlow(to) {
		return registry.Project{}, "", fmt.Errorf("invalid flow stage %q", to)
	}
	p, e := a.Registry.Resolve(q)
	if e != nil {
		return p, "", e
	}
	if p.FlowStage == to {
		return p, fmt.Sprintf("%s is already in %s — nothing to move", p.Name, to), nil
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
		return p, "", fmt.Errorf("project folder is not on disk — run `rivu scan` to reconcile: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return p, "", fmt.Errorf("%s is a symlink — move it manually, then rescan", p.Path)
	}
	if !fi.IsDir() {
		return p, "", fmt.Errorf("%s is not a directory", p.Path)
	}
	if !a.knownRoot(p.Path) {
		return p, "", fmt.Errorf("%s is outside the configured workspace roots — add its root to workspace.root or secondary_roots first", p.Path)
	}
	switch _, err := os.Lstat(dest); {
	case err == nil:
		return p, "", fmt.Errorf("destination %s already exists — move it out of the way first", dest)
	case !os.IsNotExist(err):
		return p, "", err
	}
	if filepath.VolumeName(p.Path) != filepath.VolumeName(dest) {
		return p, "", fmt.Errorf("flow cannot cross volumes: %s and %s", p.Path, dest)
	}
	if !pathsafe.Contained(filepath.Join(root, ch), dest) {
		return p, "", fmt.Errorf("destination %s escapes channel folder %s", dest, filepath.Join(root, ch))
	}
	plan := FlowPlan{ID: p.ID, Query: q, FromStage: p.FlowStage, ToStage: to, Src: p.Path, Dst: dest, Root: root, Flatten: flatten,
		Move:     []string{p.Path + " -> " + dest},
		Bank:     []string{".metadata/project.toml"},
		Registry: []string{"update path, stage and flags for " + p.Slug}}
	_ = plan

	if dry {
		return p, fmt.Sprintf("DRY RUN: would move %s: %s -> %s", p.Name, p.Path, dest), nil
	}
	if e := os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
		return p, "", e
	}
	orig := p
	moved := orig.Path != dest
	if moved {
		if e := os.Rename(orig.Path, dest); e != nil {
			return p, "", fmt.Errorf("move failed: %w", e)
		}
	}
	fail := func(err error) (registry.Project, string, error) {
		if moved {
			if re := os.Rename(dest, orig.Path); re != nil {
				return orig, "", fmt.Errorf("%w (restoring %s also failed: %v)", err, orig.Path, re)
			}
		}
		_ = bank.Sync(orig, "rivu") // put the old stage back into the moved-back file
		return orig, "", err
	}
	p.Path, p.Channel, p.FlowStage = dest, ch, to
	if e := bank.Sync(p, "rivu"); e != nil {
		return fail(fmt.Errorf("update bank: %w", e))
	}
	if e := a.Registry.ApplyFlow(p.ID, dest, ch, to, true, p.HasMap); e != nil {
		return fail(fmt.Errorf("record move: %w", e))
	}
	return p, fmt.Sprintf("Flowed %s: %s -> %s", p.Name, orig.Path, dest), nil
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
	if e = bank.Build(p, "rivu"); e != nil {
		return e
	}
	if e = mapgen.Build(p, a.Config.Scanner.Ignore); e != nil {
		return e
	}
	return a.Registry.UpdateFlags(p.ID, true, true)
}
func (a *App) OpenProject(q string) error {
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
	editor := editorlaunch.Resolve(a.Config, p.Language)
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
