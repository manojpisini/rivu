package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfluenceCommand(t *testing.T) {
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
	run := func(args ...string) int {
		t.Helper()
		return Run("test", "dev", "unknown", args)
	}
	if got := run("init", "--root", ws); got != 0 {
		t.Fatalf("init exit = %d", got)
	}
	for _, name := range []string{"site", "brand"} {
		if got := run("source", name, "--no-git"); got != 0 {
			t.Fatalf("source %s exit = %d", name, got)
		}
	}

	// Empty list: exit 0 and a schema-1 empty array, never null.
	out := captureStdout(t, func() {
		if code := run("confluence", "list", "--json"); code != 0 {
			t.Errorf("empty list exit = %d", code)
		}
	})
	var listed confluenceJSON
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	if listed.Schema != 1 || len(listed.Confluences) != 0 {
		t.Errorf("empty list = %+v, want schema 1 with []", listed)
	}

	// Bare `rivu confluence` prints the list.
	if got := run("confluence"); got != 0 {
		t.Errorf("bare confluence exit = %d", got)
	}

	// new normalises the name; duplicates and invalid names fail (1).
	out = captureStdout(t, func() {
		if code := run("confluence", "new", "Heap & Stack", "--notes", "publication family"); code != 0 {
			t.Errorf("new exit = %d", code)
		}
	})
	if !strings.Contains(out, "created confluence heap-stack") {
		t.Errorf("new = %q, want slug-normalised name", out)
	}
	if got := run("confluence", "new", "Heap & Stack"); got != 1 {
		t.Errorf("duplicate new exit = %d, want 1", got)
	}
	if got := run("confluence", "new", "   "); got != 1 {
		t.Errorf("blank new exit = %d, want 1", got)
	}

	// show works before membership and explains the empty state.
	out = captureStdout(t, func() {
		if code := run("confluence", "show", "heap-stack"); code != 0 {
			t.Errorf("show exit = %d", code)
		}
	})
	if !strings.Contains(out, "no members yet") {
		t.Errorf("show empty = %q, want no-members hint", out)
	}
	if got := run("confluence", "show", "ghost"); got != 3 {
		t.Errorf("show ghost exit = %d, want 3", got)
	}

	// add validates both sides: the confluence must exist (no typo
	// creations) and the project must be registered.
	if got := run("confluence", "add", "ghost", "site"); got != 3 {
		t.Errorf("add to unknown confluence exit = %d, want 3", got)
	}
	if got := run("confluence", "add", "heap-stack", "ghost"); got != 3 {
		t.Errorf("add unknown project exit = %d, want 3", got)
	}
	out = captureStdout(t, func() {
		if code := run("confluence", "add", "heap-stack", "site", "brand"); code != 0 {
			t.Errorf("add exit = %d", code)
		}
	})
	if !strings.Contains(out, "added site to heap-stack") || !strings.Contains(out, "added brand to heap-stack") {
		t.Errorf("add = %q, want one note per project", out)
	}
	// Re-adding is a no-op, not an error.
	if got := run("confluence", "add", "heap-stack", "site"); got != 0 {
		t.Errorf("re-add exit = %d, want 0", got)
	}

	// list --json reports the member count.
	out = captureStdout(t, func() {
		if code := run("confluence", "list", "--json"); code != 0 {
			t.Errorf("list exit = %d", code)
		}
	})
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("list --json: %v\n%s", err, out)
	}
	if len(listed.Confluences) != 1 || listed.Confluences[0].Members != 2 || listed.Confluences[0].Notes != "publication family" {
		t.Errorf("list = %+v, want heap-stack with 2 members", listed.Confluences)
	}

	// show --json carries the members as projects.
	out = captureStdout(t, func() {
		if code := run("confluence", "show", "heap-stack", "--json"); code != 0 {
			t.Errorf("show --json exit = %d", code)
		}
	})
	var shown confluenceShowJSON
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatalf("show --json: %v\n%s", err, out)
	}
	if shown.Confluence.Members != 2 || len(shown.Projects) != 2 {
		t.Errorf("show --json = %+v, want 2 members and 2 projects", shown)
	}

	// remove unlinks; a repeat reports no membership but still exits 0.
	out = captureStdout(t, func() {
		if code := run("confluence", "remove", "heap-stack", "brand"); code != 0 {
			t.Errorf("remove exit = %d", code)
		}
	})
	if !strings.Contains(out, "removed brand from heap-stack") {
		t.Errorf("remove = %q", out)
	}
	out = captureStdout(t, func() {
		if code := run("confluence", "remove", "heap-stack", "brand"); code != 0 {
			t.Errorf("second remove exit = %d", code)
		}
	})
	if !strings.Contains(out, "was not in") {
		t.Errorf("second remove = %q, want not-a-member note", out)
	}
	if got := run("confluence", "remove", "ghost", "brand"); got != 3 {
		t.Errorf("remove unknown confluence exit = %d, want 3", got)
	}

	// rename: gate (4), dry-run (0, unchanged), apply (0, members stay).
	if got := run("confluence", "rename", "heap-stack", "h-and-s"); got != 4 {
		t.Errorf("rename without --yes exit = %d, want 4", got)
	}
	if got := run("confluence", "rename", "heap-stack", "h-and-s", "--dry-run"); got != 0 {
		t.Errorf("rename dry-run exit = %d, want 0", got)
	}
	if got := run("confluence", "show", "heap-stack"); got != 0 {
		t.Errorf("dry-run renamed the confluence, show exit = %d", got)
	}
	out = captureStdout(t, func() {
		if code := run("confluence", "rename", "heap-stack", "H&S", "--yes"); code != 0 {
			t.Errorf("rename --yes exit = %d", code)
		}
	})
	if !strings.Contains(out, "renamed heap-stack -> h-s") {
		t.Errorf("rename = %q, want normalised new name", out)
	}
	if got := run("confluence", "show", "h-s"); got != 0 {
		t.Errorf("renamed confluence not found, show exit = %d", got)
	}
	if got := run("confluence", "rename", "ghost", "x", "--yes"); got != 3 {
		t.Errorf("rename missing exit = %d, want 3", got)
	}

	// rm: gate (4), dry-run (0, kept), apply (0), projects survive.
	if got := run("confluence", "rm", "h-s"); got != 4 {
		t.Errorf("rm without --yes exit = %d, want 4", got)
	}
	if got := run("confluence", "rm", "h-s", "--dry-run"); got != 0 {
		t.Errorf("rm dry-run exit = %d, want 0", got)
	}
	if got := run("confluence", "show", "h-s"); got != 0 {
		t.Errorf("dry-run removed the confluence, show exit = %d", got)
	}
	out = captureStdout(t, func() {
		if code := run("confluence", "rm", "h-s", "--yes"); code != 0 {
			t.Errorf("rm --yes exit = %d", code)
		}
	})
	if !strings.Contains(out, "projects untouched") {
		t.Errorf("rm = %q, want projects-untouched note", out)
	}
	if got := run("confluence", "show", "h-s"); got != 3 {
		t.Errorf("removed confluence still found, exit = %d, want 3", got)
	}
	if got := run("confluence", "rm", "ghost", "--yes"); got != 3 {
		t.Errorf("rm ghost exit = %d, want 3", got)
	}
	// The projects themselves were never touched.
	if got := run("list"); got != 0 {
		t.Errorf("list projects exit = %d", got)
	}
	out = captureStdout(t, func() {
		_ = run("list")
	})
	if !strings.Contains(out, "site") || !strings.Contains(out, "brand") {
		t.Errorf("projects after rm = %q, want both still registered", out)
	}

	// Shell completion: confluence first, projects after (add/remove).
	root := Root("test", "dev", "unknown")
	for _, name := range []string{"show", "rm", "rename", "add", "remove"} {
		c, _, err := root.Find([]string{"confluence", name})
		if err != nil {
			t.Fatalf("find confluence %s: %v", name, err)
		}
		if c.ValidArgsFunction == nil {
			t.Errorf("confluence %s has no ValidArgsFunction", name)
		}
	}
	show, _, err := root.Find([]string{"confluence", "show"})
	if err != nil {
		t.Fatal(err)
	}
	got, directive := show.ValidArgsFunction(show, []string{}, "h")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %d, want NoFileComp", directive)
	}
	if len(got) != 0 {
		t.Errorf("completion for removed confluence = %v, want none", got)
	}
}

// TestConfluenceCompletion verifies name prefixes complete for the
// first argument and project slugs take over afterwards.
func TestConfluenceCompletion(t *testing.T) {
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
	if got := Run("test", "dev", "unknown", []string{"init", "--root", ws}); got != 0 {
		t.Fatalf("init exit = %d", got)
	}
	for _, args := range [][]string{
		{"source", "alpha", "--no-git"},
		{"confluence", "new", "devtools"},
	} {
		if got := Run("test", "dev", "unknown", args); got != 0 {
			t.Fatalf("%v exit = %d", args, got)
		}
	}
	root := Root("test", "dev", "unknown")
	add, _, err := root.Find([]string{"confluence", "add"})
	if err != nil {
		t.Fatal(err)
	}
	// First argument: confluence names, prefix-filtered.
	got, _ := add.ValidArgsFunction(add, []string{}, "dev")
	if len(got) != 1 || !strings.HasPrefix(got[0], "devtools\t") {
		t.Errorf("first-arg completion = %v, want devtools entry", got)
	}
	// Later arguments: project slugs.
	got, _ = add.ValidArgsFunction(add, []string{"devtools"}, "al")
	if len(got) != 1 || !strings.HasPrefix(got[0], "alpha\t") {
		t.Errorf("project completion = %v, want alpha entry", got)
	}
}
