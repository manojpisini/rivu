package pathsafe

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestContained(t *testing.T) {
	base := filepath.FromSlash("/workspace")
	for _, tc := range []struct {
		target string
		want   bool
	}{
		{"/workspace", true},
		{"/workspace/proj", true},
		{"/workspace/proj/deep/nested", true},
		{"/workspace/../escape", false},
		{"/workspaceother", false},
		{"/workspace/proj/..", true}, // cleans to base itself
		{"/workspace/proj/../..", false},
		{"/workspace/../../etc/passwd", false},
		{"/elsewhere", false},
		{"", false},
	} {
		if got := Contained(base, tc.target); got != tc.want {
			t.Errorf("Contained(%q, %q) = %v, want %v", base, tc.target, got, tc.want)
		}
	}
}

func TestContainedCleansTraversal(t *testing.T) {
	base := filepath.FromSlash("/ws/root")
	if !Contained(base, filepath.FromSlash("/ws/root/ch/../proj")) {
		t.Error("cleaned path inside base should be contained")
	}
	if Contained(base, filepath.FromSlash("/ws/root/../../etc")) {
		t.Error("traversal must not escape base")
	}
}

func FuzzContained(f *testing.F) {
	for _, seed := range [][]string{
		{"/ws", "/ws/proj"},
		{"/ws", "/ws/../etc"},
		{"/ws", "/ws2"},
		{"/ws", "/ws"},
		{"", ""},
		{"base", "base/child"},
		{"base", "basechild"},
		{"/ws", "/ws/../../.."},
		{`C:\ws`, `C:\ws\a\b`},
		{"/ws", ".."},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, base, target string) {
		if !Contained(base, target) {
			return
		}
		// ":" inside a path element is invalid on Windows (drive/ADS syntax);
		// skip degenerate inputs.
		if strings.Contains(base, ":") || strings.Contains(target, ":") {
			return
		}
		ab, err := filepath.Abs(base)
		if err != nil {
			return
		}
		at, err := filepath.Abs(target)
		if err != nil {
			return
		}
		ab, at = filepath.Clean(ab), filepath.Clean(at)
		// Soundness: stdlib Rel, an independent computation, must not climb
		// out of base when Contained says the target is inside.
		if rel, err := filepath.Rel(ab, at); err == nil && (rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			t.Errorf("Contained(%q, %q) but Rel says %q escapes", base, target, rel)
		}
		// Monotonicity: a contained path's child stays contained.
		child := at + string(filepath.Separator) + "child"
		if !Contained(base, child) {
			t.Errorf("Contained(%q, %q) but not its child %q", base, target, child)
		}
	})
}
