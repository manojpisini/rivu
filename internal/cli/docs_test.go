package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsCommand(t *testing.T) {
	dir := t.TempDir()
	if got := Run("test", "dev", "unknown", []string{"docs", "--dir", dir}); got != 0 {
		t.Fatalf("docs exit = %d", got)
	}
	// Man pages use hyphens (rivu-list.1), markdown underscores
	// (rivu_list.md) — cobra's own conventions.
	for _, name := range []string{"rivu.1", "rivu.md", "rivu_list.md", "rivu-flow.1"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "rivu.1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body[:min(len(body), 40)]), ".TH") || !strings.Contains(string(body[:min(len(body), 40)]), "RIVU") {
		t.Errorf("man page header = %q, want .TH RIVU", string(body[:min(len(body), 40)]))
	}

	// Hidden from help, but discoverable.
	root := Root("test", "dev", "unknown")
	c, _, err := root.Find([]string{"docs"})
	if err != nil || !c.Hidden {
		t.Errorf("docs command hidden = %v (err %v), want true", err == nil && c.Hidden, err)
	}
	for _, sub := range root.Commands() {
		if sub.Name() == "docs" && sub.Name() != "" && !sub.Hidden {
			t.Error("docs must not appear in help")
		}
	}
}
