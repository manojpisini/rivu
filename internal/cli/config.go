package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/BurntSushi/toml"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/editorlaunch"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Inspect or change configuration"}
	c.AddCommand(
		configGetCmd(),
		configSetCmd(),
		configEditCmd(),
		configValidateCmd(),
		configResetCmd(),
		&cobra.Command{Use: "path", Short: "Print the config file path", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
			p, e := config.Path()
			if e == nil {
				fmt.Println(p)
			}
			return e
		}},
		configShowCmd(),
	)
	return c
}

// loadConfig is the shared first step: load, echo typo warnings to
// stderr, return the effective config.
func loadConfig() (config.Config, error) {
	cfg, warns, err := config.Load()
	for _, w := range warns {
		warnf("%s", w)
	}
	return cfg, err
}

func configGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Print one configuration value",
		Long: `Print one configuration value by dotted key:

  rivu config get workspace.root
  rivu config get flow.stale_threshold_days
  rivu config get scanner.ignore

Lists print comma-separated. Sections are not values; unknown keys
exit 3.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			v, err := config.Get(cfg, args[0])
			if err != nil {
				return err
			}
			fmt.Println(v)
			return nil
		},
	}
}

func configSetCmd() *cobra.Command {
	var dry bool
	c := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set one configuration value",
		Long: `Set one configuration value by dotted key and save the config
file atomically. The value is converted against the key's type:
true/false for bools, integers for numbers, comma-separated lists
for arrays. The result must pass config validation or nothing is
written. New entries inside maps (editors.per_language,
health.weights) are added with rivu config edit.

  rivu config set flow.stale_threshold_days 30

--dry-run prints old and new without writing.`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			old, err := config.Get(cfg, key)
			if err != nil {
				return err
			}
			if err := config.Set(&cfg, key, value); err != nil {
				return err
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if dry {
				fmt.Printf("would set %s: %q -> %q (not written)\n", key, old, value)
				return nil
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("set %s = %q\n", key, value)
			return nil
		},
	}
	c.Flags().BoolVar(&dry, "dry-run", false, "Show old and new without writing")
	return c
}

func configEditCmd() *cobra.Command {
	var editor string
	c := &cobra.Command{
		Use:   "edit",
		Short: "Open the config file in an editor",
		Long: `Open the config file in your editor (editors.default, or
--editor). Terminal editors run in the foreground so the TTY stays
with them.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			p, err := config.Path()
			if err != nil {
				return err
			}
			cfg, err := loadConfig() // creates the file with defaults when missing
			if err != nil {
				return err
			}
			if editor == "" {
				editor = editorlaunch.Resolve(cfg, "")
			}
			args, err := editorlaunch.Parse(editor)
			if err != nil {
				return err
			}
			argv := append(args, p)
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if editorlaunch.IsGUI(editor, cfg.Editors.GUI) {
				return cmd.Start()
			}
			return cmd.Run()
		},
	}
	c.Flags().StringVar(&editor, "editor", "", "Editor command (overrides editors.default)")
	return c
}

func configValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check the config file, values and workspace root",
		Long: `Check the config file parses, the values are ones Rivu can work
with (health weights sum to 100, thresholds >= 1, ...) and the
workspace root exists.

Exit codes: 0 valid, 1 problems, 5 only warnings (unknown keys).`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, warns, err := config.Load()
			if err != nil {
				return err
			}
			for _, w := range warns {
				warnf("%s", w)
			}
			var problems []error
			if err := config.Validate(cfg); err != nil {
				problems = append(problems, err)
			}
			if err := config.ValidateRoot(cfg); err != nil {
				problems = append(problems, err)
			}
			if len(problems) > 0 {
				return errors.Join(problems...)
			}
			p, _ := config.Path()
			if len(warns) > 0 {
				return fmt.Errorf("%w: %d unknown key(s) in %s — fix the typo or remove it", ErrWarnings, len(warns), p)
			}
			fmt.Printf("config OK: %s\n", p)
			return nil
		},
	}
}

func configResetCmd() *cobra.Command {
	var yes, dry bool
	c := &cobra.Command{
		Use:   "reset",
		Short: "Reset the config file to defaults",
		Long: `Write the default configuration to the config file. The
current file is backed up to config.toml.bak first. Requires --yes;
--dry-run shows what would happen without writing.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !yes && !dry {
				return fmt.Errorf("config reset requires --yes (or use --dry-run): %w", ErrNeedsConfirm)
			}
			p, err := config.Path()
			if err != nil {
				return err
			}
			raw, readErr := os.ReadFile(p)
			existed := readErr == nil
			backup := p + ".bak"
			if dry {
				if existed {
					fmt.Printf("would reset %s to defaults (current file backed up to %s)\n", p, backup)
				} else {
					fmt.Printf("would create %s with defaults\n", p)
				}
				return nil
			}
			if existed {
				if err := os.WriteFile(backup, raw, 0600); err != nil {
					return fmt.Errorf("back up config: %w", err)
				}
			}
			if err := config.Save(config.Default()); err != nil {
				return err
			}
			if existed {
				fmt.Printf("config reset to defaults (backup: %s)\n", backup)
			} else {
				fmt.Printf("config created with defaults: %s\n", p)
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm the reset")
	c.Flags().BoolVar(&dry, "dry-run", false, "Show what would happen without writing")
	return c
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print every effective configuration value",
		Long: `Print the full effective configuration (file values merged
over defaults) as TOML, with the file path as a header.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			p, _ := config.Path()
			fmt.Printf("# %s\n", p)
			return toml.NewEncoder(os.Stdout).Encode(cfg)
		},
	}
}
