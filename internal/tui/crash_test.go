package tui

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestRunRecoversCrashWithLoggedStack (P3.26): a panic escaping the
// event loop becomes a normal error carrying a log pointer, and the
// crash report lands in the slog handler with a stack.
func TestRunRecoversCrashWithLoggedStack(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	var err error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				logCrash(rec, &err)
			}
		}()
		panic("view exploded")
	}()

	if err == nil || !strings.Contains(err.Error(), "tui crashed: view exploded") {
		t.Fatalf("err = %v, want a wrapped crash error", err)
	}
	if !strings.Contains(err.Error(), "Rivu log") {
		t.Errorf("err must point at the log: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"tui panic", "view exploded", "stack", "goroutine"} {
		if !strings.Contains(out, want) {
			t.Errorf("crash report missing %q: %s", want, out)
		}
	}
}
