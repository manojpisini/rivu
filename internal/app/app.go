package app

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/manojpisini/rivu/internal/bank"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/doctor"
	"github.com/manojpisini/rivu/internal/mapgen"
	"github.com/manojpisini/rivu/internal/pathsafe"
	"github.com/manojpisini/rivu/internal/registry"
	"github.com/manojpisini/rivu/internal/scanner"
	"github.com/manojpisini/rivu/internal/slug"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type App struct {
	Config   config.Config
	Registry *registry.Registry
	// ConfigWarnings lists unrecognized config keys found at load time.
	ConfigWarnings []string
	// ScanWarnings lists per-project failures from the last Scan.
	ScanWarnings []string
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
func (a *App) Source(name, flow string, gitInit, dry bool) (registry.Project, error) {
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
	if dry {
		return p, nil
	}
	if e := os.MkdirAll(path, 0755); e != nil {
		return p, e
	}
	if gitInit {
		cmd := exec.Command("git", "init")
		cmd.Dir = path
		if out, e := cmd.CombinedOutput(); e != nil {
			return p, fmt.Errorf("git init: %w: %s", e, out)
		}
		p.HasGit = true
	}
	if a.Config.Automation.CreateBank {
		if e := bank.Build(p, "rivu"); e != nil {
			return p, e
		}
		p.HasBank = true
	}
	if a.Config.Automation.BuildMap {
		if e := mapgen.Build(p, a.Config.Scanner.Ignore); e != nil {
			return p, e
		}
		p.HasMap = true
	}
	if e := a.Registry.Upsert(p); e != nil {
		return p, e
	}
	_ = a.Registry.SetCurrent(p.ID)
	_ = a.Registry.LogActivity(p.ID, "sourced")
	return p, nil
}
func (a *App) Flow(q, to string, dry bool) (registry.Project, string, error) {
	if !validFlow(to) {
		return registry.Project{}, "", fmt.Errorf("invalid flow stage %q", to)
	}
	p, e := a.Registry.Resolve(q)
	if e != nil {
		return p, "", e
	}
	dest := filepath.Join(a.Config.Workspace.Root, channel(to), filepath.Base(p.Path))
	if dry {
		return p, dest, nil
	}
	if e = os.MkdirAll(filepath.Dir(dest), 0755); e != nil {
		return p, dest, e
	}
	if p.Path != dest {
		if e = os.Rename(p.Path, dest); e != nil {
			return p, dest, e
		}
	}
	p.Path = dest
	p.Channel = channel(to)
	p.FlowStage = to
	if e = bank.Sync(p, "rivu"); e != nil {
		return p, dest, e
	}
	if e = a.Registry.UpdatePathFlow(p.ID, dest, p.Channel, to); e != nil {
		return p, dest, e
	}
	if e = a.Registry.UpdateFlags(p.ID, true, p.HasMap); e != nil {
		return p, dest, e
	}
	return p, dest, nil
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
		r := doctor.Run(p)
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
	_ = a.Registry.SetCurrent(p.ID)
	_ = a.Registry.MarkOpened(p.ID)
	_ = a.Registry.LogActivity(p.ID, "opened")
	editor := a.Config.Editors.Default
	if editor == "${EDITOR}" || editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		if runtime.GOOS == "windows" {
			editor = "code"
		} else {
			editor = "vi"
		}
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], p.Path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}
