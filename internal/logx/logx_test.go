package logx

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestOpenWritesJSONLines(t *testing.T) {
	home := t.TempDir()
	l, f, err := Open(home, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	l.Info("hello", "k", 1)
	b, err := os.ReadFile(filepath.Join(home, "logs", "rivu.log"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{`"msg":"hello"`, `"k":1`, `"level":"INFO"`} {
		if !strings.Contains(got, want) {
			t.Errorf("log line %q missing %s", got, want)
		}
	}
}

func TestOpenVerboseLogsDebug(t *testing.T) {
	home := t.TempDir()
	l, f, err := Open(home, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	l.Debug("deep")
	b, err := os.ReadFile(filepath.Join(home, "logs", "rivu.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"msg":"deep"`) {
		t.Errorf("debug line missing: %s", b)
	}
}

func TestRotateKeepsThreeGenerations(t *testing.T) {
	home := t.TempDir()
	logs := filepath.Join(home, "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("x", maxBytes)
	mustWrite(t, filepath.Join(logs, "rivu.log"), big)
	mustWrite(t, filepath.Join(logs, "rivu.log.1"), "one")
	mustWrite(t, filepath.Join(logs, "rivu.log.2"), "two")

	if _, f, err := Open(home, false); err != nil {
		t.Fatal(err)
	} else {
		t.Cleanup(func() { f.Close() })
	}
	if b, err := os.ReadFile(filepath.Join(logs, "rivu.log.1")); err != nil || string(b) != big {
		t.Errorf(".1 = %.20q (err %v), want rotated big log", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(logs, "rivu.log.2")); err != nil || string(b) != "one" {
		t.Errorf(".2 = %q (err %v), want old .1", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(logs, "rivu.log.3")); err != nil || string(b) != "two" {
		t.Errorf(".3 = %q (err %v), want old .2", b, err)
	}
	if _, err := os.Stat(filepath.Join(logs, "rivu.log.4")); !os.IsNotExist(err) {
		t.Errorf(".4 exists (err %v), want generation cap", err)
	}
}

func TestRotateSkipsSmallLog(t *testing.T) {
	home := t.TempDir()
	logs := filepath.Join(home, "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "rivu.log")
	mustWrite(t, path, "keep me")

	l, f, err := Open(home, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	l.Info("next")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "keep me") {
		t.Errorf("small log rotated: %q", b)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf(".1 created for small log (err %v)", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestTailReturnsLastNLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.log")
	mustWrite(t, path, "one\ntwo\nthree\nfour\nfive\n")

	got, err := Tail(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"three", "four", "five"}
	if len(got) != len(want) || got[0] != want[0] || got[2] != want[2] {
		t.Errorf("Tail(3) = %q, want %q", got, want)
	}
	all, err := Tail(path, 100)
	if err != nil || len(all) != 5 {
		t.Errorf("Tail(100) = %q (err %v), want all 5 lines", all, err)
	}
}

func TestTailMissingFileIsEmptyNotError(t *testing.T) {
	got, err := Tail(filepath.Join(t.TempDir(), "nope.log"), 10)
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %q, want empty", got)
	}
}

func TestTailLargeFileDropsPartialWindowLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rivu.log")
	var b strings.Builder
	for i := 0; b.Len() < tailWindow+1000; i++ {
		b.WriteString("line-" + strconv.Itoa(i) + "\n")
	}
	mustWrite(t, path, b.String())

	got, err := Tail(path, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 50 {
		t.Fatalf("got %d lines, want 50", len(got))
	}
	for _, ln := range got {
		if !strings.HasPrefix(ln, "line-") {
			t.Fatalf("partial window line leaked: %q", ln)
		}
	}
	if !strings.HasPrefix(got[len(got)-1], "line-") {
		t.Errorf("last line = %q, want the newest line", got[len(got)-1])
	}
}
