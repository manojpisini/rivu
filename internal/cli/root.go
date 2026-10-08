// Package cli builds the cobra command tree: one file per command,
// shared app-opening helper, stdout for data and stderr for messages.
// cmd/rivu/main.go only wires version info and executes the root.
package cli

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/config"
	"github.com/manojpisini/rivu/internal/logx"
	"github.com/manojpisini/rivu/internal/service"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
)

// Root assembles the full command tree. version/commit/date come from
// main, where the ldflags targets live; unresolved defaults fall back
// to build info (X-05).
func Root(version, commit, date string) *cobra.Command {
	v, c, d := buildInfo(version, commit, date)
	return newRoot(v, c, d, new(bool))
}

// Run executes the command tree with args and returns the process exit
// code (X-03: 0 ok, 1 error, 2 usage, 3 not found, 4 needs --yes,
// 5 warnings).
func Run(version, commit, date string, args []string) int {
	defer setupLog(args)()
	ran := new(bool)
	v, c, d := buildInfo(version, commit, date)
	r := newRoot(v, c, d, ran)
	r.SetArgs(args)
	return ExitCode(r.Execute(), *ran)
}

// setupLog points slog at the rotating JSON file under the Rivu home
// (P2.27) and returns a closer Run defers — Windows refuses to delete
// open files, so tests cannot clean up otherwise. Best effort: a
// broken log dir warns on stderr and the run continues without file
// logging. -v/--verbose is peeked from raw args because cobra has not
// parsed yet; slugs cannot start with a dash, so no positional can be
// mistaken for the flag.
func setupLog(args []string) func() {
	nop := func() {}
	dir, err := config.Dir()
	if err != nil {
		warnf("rivu home: %v", err)
		return nop
	}
	l, f, err := logx.Open(dir, slices.Contains(args, "-v") || slices.Contains(args, "--verbose"))
	if err != nil {
		warnf("log file: %v", err)
		return nop
	}
	slog.SetDefault(l)
	return func() { f.Close() }
}

func newRoot(version, commit, date string, ran *bool) *cobra.Command {
	r := &cobra.Command{Use: "rivu", Short: "Your project filesystem, mapped and flowing", Version: fmt.Sprintf("%s (%s, %s)", version, commit, date), SilenceUsage: true}
	var home, cfgFile string
	r.PersistentFlags().StringVar(&home, "home", "", "set Rivu home directory (overrides RIVU_HOME)")
	r.PersistentFlags().StringVar(&cfgFile, "config", "", "set config file path (overrides RIVU_CONFIG)")
	r.PersistentFlags().Bool("json", false, "output JSON where supported (schema 1)")
	r.PersistentFlags().Bool("no-color", false, "disable ANSI colour output (same as NO_COLOR)")
	verbose := new(bool)
	r.PersistentFlags().BoolVarP(verbose, "verbose", "v", false, "print config and root diagnostics to stderr")
	quiet := new(bool)
	r.PersistentFlags().BoolVarP(quiet, "quiet", "q", false, "suppress warning notes on stderr (exit codes still report them)")
	r.PersistentFlags().BoolP("yes", "y", false, "skip confirmation prompts (Plan → confirm → Apply)")
	r.PersistentFlags().Bool("dry-run", false, "preview changes without writing")
	r.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Marks that validation passed: failures before this point are
		// usage errors (exit 2), after it they are real errors (exit 1+).
		*ran = true
		slog.Info("command", "path", cmd.CommandPath(), "args", args)
		if home != "" {
			if err := os.Setenv("RIVU_HOME", home); err != nil {
				return fmt.Errorf("apply --home: %w", err)
			}
		}
		if cfgFile != "" {
			if err := os.Setenv("RIVU_CONFIG", cfgFile); err != nil {
				return fmt.Errorf("apply --config: %w", err)
			}
		}
		if flagBool(cmd, "no-color") {
			lipgloss.SetColorProfile(termenv.Ascii)
		}
		// warnf has no cmd handle (it runs inside withApp closures), so
		// -q travels through the environment. Set or cleared every run,
		// keeping in-process invocations deterministic.
		if flagBool(cmd, "quiet") {
			if err := os.Setenv("RIVU_QUIET", "1"); err != nil {
				return fmt.Errorf("apply --quiet: %w", err)
			}
		} else {
			os.Unsetenv("RIVU_QUIET")
		}
		return nil
	}
	r.RunE = withApp(func(a *service.App, _ []string) error {
		return launchTUI(a)
	})
	r.AddCommand(tuiCmd(), scanCmd(), indexCmd(), sourceCmd(), openCmd(), flowCmd(), deltaCmd(), doctorCmd(), mapCmd(), listCmd(), statsCmd(), configCmd(), dashboardCmd(), confluenceCmd(), dbCmd(), versionCmd(version, commit, date), initCmd(), pathCmd(), docsCmd())
	return r
}

// requireFlag validates a required flag during the args stage, which
// runs before PersistentPreRunE — cobra's own required-flag check runs
// after it and would misreport usage errors as exit 1. Keep
// MarkFlagRequired too so help still shows "(required)".
func requireFlag(name string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, _ []string) error {
		if !cmd.Flags().Changed(name) {
			return fmt.Errorf("required flag --%s not set", name)
		}
		return nil
	}
}

// exclusiveAll rejects --all combined with a positional project
// during the args stage (exit 2), before any I/O runs.
func exclusiveAll(all *bool) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if *all && len(args) > 0 {
			return fmt.Errorf("give either a project or --all, not both")
		}
		return nil
	}
}

// enumFlag validates a flag's value during the args stage, so a bad
// value exits 2 (usage) before any I/O runs. Empty means "not set".
func enumFlag(name string, allowed ...string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, _ []string) error {
		v := cmd.Flags().Lookup(name)
		if v == nil || v.Value.String() == "" {
			return nil
		}
		for _, a := range allowed {
			if v.Value.String() == a {
				return nil
			}
		}
		return fmt.Errorf("--%s must be one of: %s", name, strings.Join(allowed, ", "))
	}
}

// flagBool reads a bool flag from the command being run. cmd.Flags()
// holds the command's own flags plus the root's persistent flags after
// parsing, so one call covers both local and global positions
// (rivu --json list and rivu list --json).
func flagBool(cmd *cobra.Command, name string) bool {
	v, err := cmd.Flags().GetBool(name)
	return err == nil && v
}

// warnf prints a warning to stderr unless -q is set. Warnings still
// surface through exit code 5, so scripts stay correct in quiet mode.
func warnf(format string, a ...any) {
	if os.Getenv("RIVU_QUIET") != "" {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", a...)
}

// withApp opens the service, prints config warnings to stderr, runs fn
// with the command's positional args, and closes the app. No globals —
// every command reads its args from here (X-01).
func withApp(fn func(*service.App, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		a, e := service.Open()
		if e != nil {
			return e
		}
		defer a.Close()
		if flagBool(cmd, "verbose") {
			p, err := config.Path()
			if err != nil {
				p = "(not created yet)"
			}
			fmt.Fprintf(os.Stderr, "rivu: config %s\n", p)
			fmt.Fprintf(os.Stderr, "rivu: root %s\n", a.Config.Workspace.Root)
		}
		for _, w := range a.ConfigWarnings {
			warnf("%s", w)
		}
		if err := fn(a, args); err != nil {
			return err
		}
		if n := len(a.ConfigWarnings); n > 0 {
			return fmt.Errorf("%w: %d config warning(s)", ErrWarnings, n)
		}
		return nil
	}
}
