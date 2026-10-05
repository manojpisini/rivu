package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// home guards cli.Run tests that forget to isolate RIVU_HOME: Run opens
// the file logger under the Rivu home, and tests must never touch the
// real ~/.rivu (spec 4.4).
var home string

func TestMain(m *testing.M) {
	if os.Getenv("RIVU_HOME") == "" {
		if h, err := os.MkdirTemp("", "rivu-cli-test-home"); err == nil {
			home = h
			os.Setenv("RIVU_HOME", h)
		}
	}
	code := m.Run()
	if home != "" {
		os.RemoveAll(home)
	}
	os.Exit(code)
}

func TestRunLogsCommands(t *testing.T) {
	h := t.TempDir()
	t.Setenv("RIVU_HOME", h)
	t.Setenv("RIVU_CONFIG", "")

	if got := Run("test", "dev", "unknown", []string{"version"}); got != 0 {
		t.Fatalf("version exit = %d", got)
	}
	b, err := os.ReadFile(filepath.Join(h, "logs", "rivu.log"))
	if err != nil {
		t.Fatalf("log file: %v", err)
	}
	got := string(b)
	for _, want := range []string{`"msg":"command"`, `"path":"rivu version"`} {
		if !strings.Contains(got, want) {
			t.Errorf("log %q missing %s", got, want)
		}
	}
}
