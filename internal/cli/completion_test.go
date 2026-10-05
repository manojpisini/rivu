package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCompleteProjects(t *testing.T) {
	home := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	t.Setenv("RIVU_HOME", home)
	t.Setenv("RIVU_CONFIG", "")
	ws := filepath.Join(home, "ws")
	if err := os.MkdirAll(ws, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("[workspace]\nroot = %q\n", ws)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	if got := Run("test", "dev", "unknown", []string{"source", "alpha", "--no-git"}); got != 0 {
		t.Fatalf("source alpha exit = %d", got)
	}
	if got := Run("test", "dev", "unknown", []string{"source", "beta", "--no-git"}); got != 0 {
		t.Fatalf("source beta exit = %d", got)
	}

	// Wired onto every project-argument command.
	root := Root("test", "dev", "unknown")
	for _, name := range []string{"open", "doctor", "flow", "delta", "path"} {
		c, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if c.ValidArgsFunction == nil {
			t.Errorf("%s has no ValidArgsFunction", name)
		}
	}

	open, _, err := root.Find([]string{"open"})
	if err != nil {
		t.Fatal(err)
	}
	// Prefix-filtered, tab-labelled, no file completion offered.
	got, directive := open.ValidArgsFunction(open, []string{}, "al")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want NoFileComp", directive)
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "alpha\t") {
		t.Errorf("completion for 'al' = %v, want alpha entry", got)
	}
	got, _ = open.ValidArgsFunction(open, []string{}, "")
	if len(got) != 2 {
		t.Errorf("completion for '' = %v, want 2 entries", got)
	}
}
