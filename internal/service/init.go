package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/registry"
)

// InitResult reports what `rivu init` created (P2.09).
type InitResult struct {
	ConfigPath string
	DBPath     string
	Root       string
}

// Init creates the config file and registry database for a workspace
// root (spec 4.6 first checkpoint). It refuses to overwrite an existing
// config unless force is set, validates that root exists on disk, and
// never goes through Open so a first run cannot silently bootstrap a
// default config pointing at another workspace (C-07).
func Init(root string, force bool) (InitResult, error) {
	if root == "" {
		return InitResult{}, fmt.Errorf("workspace root is required — run: rivu init --root <path>")
	}
	p, err := config.Path()
	if err != nil {
		return InitResult{}, err
	}
	_, statErr := os.Stat(p)
	exists := statErr == nil
	if exists && !force {
		return InitResult{}, fmt.Errorf("config already exists at %s — use --force to overwrite it", p)
	}
	expanded := config.Expand(root)
	if _, err := os.Stat(expanded); err != nil {
		return InitResult{}, fmt.Errorf("workspace root %s does not exist — create it first: mkdir %s", expanded, expanded)
	}

	c := config.Default()
	if exists {
		// --force keeps whatever settings the file already has; a broken
		// file falls back to defaults rather than blocking re-init.
		if loaded, _, lerr := config.Load(); lerr == nil {
			c = loaded
		}
	}
	c.Workspace.Root = expanded
	if err := config.Save(c); err != nil {
		return InitResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(c.Data.DBPath), 0755); err != nil {
		return InitResult{}, fmt.Errorf("create data dir: %w", err)
	}
	r, err := registry.Open(c.Data.DBPath)
	if err != nil {
		return InitResult{}, fmt.Errorf("create registry: %w", err)
	}
	if err := r.Close(); err != nil {
		return InitResult{}, fmt.Errorf("close registry: %w", err)
	}
	return InitResult{ConfigPath: p, DBPath: c.Data.DBPath, Root: expanded}, nil
}
