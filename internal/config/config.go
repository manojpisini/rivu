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
		GUI         []string          `toml:"gui"`
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
	Health struct {
		Weights map[string]int `toml:"weights"`
	} `toml:"health"`
	Git struct {
		DefaultBranch string `toml:"default_branch"`
	} `toml:"git"`
	Bridge struct {
		Enabled      bool   `toml:"enabled"`
		OwnsGitInit  bool   `toml:"owns_git_init"`
		ScaffoldMark string `toml:"scaffold_marker"`
	} `toml:"bridge"`
	Templates struct {
		Default string `toml:"default"`
	} `toml:"templates"`
	Keybindings struct {
		Profile string `toml:"profile"`
	} `toml:"keybindings"`
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
		return Expand(h), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".rivu"), nil
}
func Path() (string, error) {
	if p := os.Getenv("RIVU_CONFIG"); p != "" {
		return Expand(p), nil
	}
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// Expand resolves $VAR/${VAR} references and a leading ~, then makes
// the path absolute so downstream code never mixes relative and absolute.
func Expand(s string) string {
	s = os.Expand(s, func(k string) string {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
		if k == "HOME" {
			if h, e := os.UserHomeDir(); e == nil {
				return h
			}
		}
		return ""
	})
	if s == "~" || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "~\\") {
		if h, e := os.UserHomeDir(); e == nil {
			if s == "~" {
				s = h
			} else {
				s = filepath.Join(h, s[2:])
			}
		}
	}
	if abs, err := filepath.Abs(s); err == nil {
		return abs
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
	c.Editors.GUI = []string{"code", "code-insiders", "zed", "subl", "sublime_text", "idea", "webstorm", "pycharm", "cursor", "windsurf"}
	c.Automation.BridgeOwnsGitInit = true
	c.Automation.CreateBank = true
	c.Automation.BuildMap = true
	c.Flow.StaleThresholdDays = 45
	c.Flow.SourceSLADays = 14
	c.Scanner.Ignore = []string{"node_modules", ".git", "dist", "build", ".next", "target", "vendor", "coverage", ".cache", ".venv", "__pycache__"}
	c.Scanner.MaxDepth = 6
	c.Health.Weights = map[string]int{"readme": 20, "git": 15, "bank": 15, "map": 15, "tests": 10, "ci": 10, "license": 10, "deps_fresh": 5}
	c.Git.DefaultBranch = "main"
	c.Bridge.Enabled = false
	c.Bridge.OwnsGitInit = true
	c.Bridge.ScaffoldMark = ".lode"
	c.Templates.Default = "empty"
	c.Keybindings.Profile = "default"
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
	if sum := weightsSum(c.Health.Weights); c.Health.Weights != nil && sum != 100 {
		problems = append(problems, fmt.Sprintf("health.weights sums to %d, must be 100", sum))
	}
	if c.Git.DefaultBranch == "" {
		problems = append(problems, "git.default_branch is empty, set it (usually \"main\")")
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("invalid config in %s:\n  - %s\nFix the listed values and run again", mustPath(), strings.Join(problems, "\n  - "))
}

func weightsSum(w map[string]int) int {
	sum := 0
	for _, v := range w {
		sum += v
	}
	return sum
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
	// Map tables in the file are the whole truth: nil the defaults so
	// a partial [health.weights] or [editors.per_language] replaces
	// instead of merging (a merge can sum weights past 100 and make
	// Validate reject a perfectly valid file). Tables the file omits
	// get their defaults back below.
	perLang, weights := c.Editors.PerLanguage, c.Health.Weights
	c.Editors.PerLanguage, c.Health.Weights = nil, nil
	md, err := toml.DecodeFile(p, &c)
	if err != nil {
		return c, nil, fmt.Errorf("decode config: %w", err)
	}
	if !md.IsDefined("editors", "per_language") {
		c.Editors.PerLanguage = perLang
	}
	if !md.IsDefined("health", "weights") {
		c.Health.Weights = weights
	}
	var warnings []string
	for _, key := range md.Undecoded() {
		warnings = append(warnings, fmt.Sprintf("unknown config key %q in %s — fix the typo or remove it", key, p))
	}
	c.Workspace.Root = Expand(c.Workspace.Root)
	for i, r := range c.Workspace.SecondaryRoots {
		c.Workspace.SecondaryRoots[i] = Expand(r)
	}
	c.Data.DBPath = Expand(c.Data.DBPath)
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
		os.Remove(tmp) // rivu-allow-remove: Save's own temp file
		return fmt.Errorf("encode config: %w", err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp) // rivu-allow-remove: Save's own temp file
		return fmt.Errorf("flush config: %w", err)
	}
	if err = f.Close(); err != nil {
		os.Remove(tmp) // rivu-allow-remove: Save's own temp file
		return fmt.Errorf("close config: %w", err)
	}
	if err = os.Chmod(tmp, 0600); err != nil {
		os.Remove(tmp) // rivu-allow-remove: Save's own temp file
		return fmt.Errorf("secure config: %w", err)
	}
	if err = os.Rename(tmp, p); err != nil {
		os.Remove(tmp) // rivu-allow-remove: Save's own temp file
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
