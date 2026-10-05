package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemovalOnlyAllowListed enforces safety rule 1: nothing in non-test
// code may call os.Remove/RemoveAll without a `rivu-allow-remove` sentinel
// naming what the call is allowed to delete (currently only rollbackSource
// and config.Save's own temp file). Test files are exempt: they only delete
// their own TempDir fixtures.
func TestRemovalOnlyAllowListed(t *testing.T) {
	root := "../.."
	var offenders []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if !strings.Contains(line, "os.Remove(") && !strings.Contains(line, "os.RemoveAll(") {
				continue
			}
			if strings.Contains(line, "rivu-allow-remove") {
				continue
			}
			if i > 0 && strings.Contains(lines[i-1], "rivu-allow-remove") {
				continue
			}
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(line)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Errorf("os.Remove/RemoveAll outside an allow-listed helper:\n  %s\nEvery call needs a // rivu-allow-remove sentinel naming what it may delete",
			strings.Join(offenders, "\n  "))
	}
}
