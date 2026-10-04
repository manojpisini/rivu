package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Workspace struct {
		Root           string   `toml:"root"`
		SecondaryRoots []string `toml:"secondary_roots"`
		AutoRescan     bool     `toml:"auto_rescan_on_launch"`
	} `toml:"workspace"`
	Editors struct {
		Default     string            `toml:"default"`
		PerLanguage map[string]string `toml:"per_language"`
	} `toml:"editors"`
	Automation struct {
		BridgeOwnsGitInit bool `toml:"bridge_owns_git_init"`
		CreateBank        bool `toml:"create_bank_by_default"`
		BuildMap          bool `toml:"build_map_by_default"`
	} `toml:"automation"`
	Flow struct {
		StaleThresholdDays int `toml:"stale_threshold_days"`
		SourceSLADays      int `toml:"source_sla_days"`
	} `toml:"flow"`
	Scanner struct {
		Ignore   []string `toml:"ignore"`
		MaxDepth int      `toml:"max_depth"`
	} `toml:"scanner"`
	Appearance struct {
		Theme   string `toml:"theme"`
		Density string `toml:"density"`
	} `toml:"appearance"`
	Data struct {
		DBPath                string `toml:"db_path"`
		SnapshotRetentionDays int    `toml:"snapshot_retention_days"`
	} `toml:"data"`
}

func Dir() (string, error) {
	if h := os.Getenv("RIVU_HOME"); h != "" {
		return expand(h), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".rivu"), nil
}
func Path() (string, error) {
	if p := os.Getenv("RIVU_CONFIG"); p != "" {
		return expand(p), nil
	}
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}
func expand(s string) string {
	if strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "~\\") {
		if h, e := os.UserHomeDir(); e == nil {
			return filepath.Join(h, s[2:])
		}
	}
	return filepath.Clean(s)
}
func Default() Config {
	var c Config
	h, _ := os.UserHomeDir()
	c.Workspace.Root = filepath.Join(h, "Projects")
	c.Workspace.AutoRescan = true
	if runtime.GOOS == "windows" {
		c.Editors.Default = "code"
	} else {
		c.Editors.Default = "${EDITOR}"
	}
	c.Editors.PerLanguage = map[string]string{}
	c.Automation.BridgeOwnsGitInit = true
	c.Automation.CreateBank = true
	c.Automation.BuildMap = true
	c.Flow.StaleThresholdDays = 45
	c.Flow.SourceSLADays = 14
	c.Scanner.Ignore = []string{"node_modules", ".git", "dist", "build", ".next", "target", "vendor", "coverage", ".cache", ".venv", "__pycache__"}
	c.Scanner.MaxDepth = 6
	c.Appearance.Theme = "graphite-violet"
	c.Appearance.Density = "comfortable"
	d, _ := Dir()
	c.Data.DBPath = filepath.Join(d, "rivu.db")
	c.Data.SnapshotRetentionDays = 90
	return c
}

// Validate reports configuration values Rivu cannot work with.
// Call it after Load; it never mutates c.
func Validate(c Config) error {
	var problems []string
	if c.Scanner.MaxDepth < 1 {
		problems = append(problems, fmt.Sprintf("scanner.max_depth = %d, must be >= 1", c.Scanner.MaxDepth))
	}
	if c.Flow.StaleThresholdDays < 1 {
		problems = append(problems, fmt.Sprintf("flow.stale_threshold_days = %d, must be >= 1", c.Flow.StaleThresholdDays))
	}
	if c.Flow.SourceSLADays < 1 {
		problems = append(problems, fmt.Sprintf("flow.source_sla_days = %d, must be >= 1", c.Flow.SourceSLADays))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid config in %s:\n  - %s\nFix the listed values and run again", mustPath(), strings.Join(problems, "\n  - "))
}

// ValidateRoot checks the workspace root exists; scan cannot run without it.
func ValidateRoot(c Config) error {
	if _, err := os.Stat(c.Workspace.Root); err != nil {
		return fmt.Errorf("workspace root %s does not exist — create it or set [workspace].root in %s", c.Workspace.Root, mustPath())
	}
	return nil
}

func mustPath() string {
	p, err := Path()
	if err != nil {
		return "config.toml"
	}
	return p
}

// Load reads the config file over defaults. The second return lists
// unrecognized keys (typos) that were ignored; fix or remove them.
func Load() (Config, []string, error) {
	c := Default()
	p, err := Path()
	if err != nil {
		return c, nil, err
	}
	if _, err = os.Stat(p); errors.Is(err, os.ErrNotExist) {
		if err = Save(c); err != nil {
			return c, nil, err
		}
		return c, nil, nil
	}
	if err != nil {
		return c, nil, err
	}
	md, err := toml.DecodeFile(p, &c)
	if err != nil {
		return c, nil, fmt.Errorf("decode config: %w", err)
	}
	var warnings []string
	for _, key := range md.Undecoded() {
		warnings = append(warnings, fmt.Sprintf("unknown config key %q in %s — fix the typo or remove it", key, p))
	}
	c.Workspace.Root = expand(c.Workspace.Root)
	c.Data.DBPath = expand(c.Data.DBPath)
	return c, warnings, nil
}
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	tmp := f.Name()
	if err = toml.NewEncoder(f).Encode(c); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("encode config: %w", err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("flush config: %w", err)
	}
	if err = f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close config: %w", err)
	}
	if err = os.Chmod(tmp, 0600); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("secure config: %w", err)
	}
	if err = os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
