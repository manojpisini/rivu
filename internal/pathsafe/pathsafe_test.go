package pathsafe

import (
	"path/filepath"
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
