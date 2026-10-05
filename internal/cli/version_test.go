package cli

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
)

func TestVersionPlain(t *testing.T) {
	var buf bytes.Buffer
	c := versionCmd("1.2.3", "abc1234", "2026-01-02")
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs(nil)
	if err := c.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got, want := buf.String(), "1.2.3 (abc1234, 2026-01-02)\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestVersionJSONShape(t *testing.T) {
	var buf bytes.Buffer
	c := versionCmd("1.2.3", "abc1234", "2026-01-02")
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetArgs([]string{"--json"})
	if err := c.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got versionOutput
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if got.Schema != 1 || got.Version != "1.2.3" || got.Commit != "abc1234" || got.Date != "2026-01-02" {
		t.Errorf("shape = %+v", got)
	}
	if got.Go == "" || got.OS == "" || got.Arch == "" {
		t.Errorf("missing build fields: %+v", got)
	}
}

func TestBuildInfoPassThrough(t *testing.T) {
	v, c, d := buildInfo("1.2.3", "abc", "2026-01-02")
	if v != "1.2.3" || c != "abc" || d != "2026-01-02" {
		t.Errorf("explicit values were rewritten: %s %s %s", v, c, d)
	}
}

// TestBuildInfoFallback: unresolved defaults may stay as-is (no build
// info available) or be replaced with stamped values, but never with
// something malformed.
func TestBuildInfoFallback(t *testing.T) {
	v, c, d := buildInfo("dev", "none", "unknown")
	if v == "" {
		t.Error("version resolved to empty")
	}
	if c != "none" && c != "dev" && !regexp.MustCompile(`^[0-9a-f]{7,40}$`).MatchString(c) {
		t.Errorf("commit = %q, want none/dev or a revision", c)
	}
	if d != "unknown" && !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T`).MatchString(d) {
		t.Errorf("date = %q, want unknown or RFC3339", d)
	}
}
