package service

import (
	"strings"
	"testing"
)

// TestMountWarn covers the spec's WSL row: /mnt/... workspaces get the
// slow-scan/cross-FS advisory; everything else stays silent.
func TestMountWarn(t *testing.T) {
	for _, tc := range []struct{ root, want string }{
		{"/mnt/c/Users/me/ws", "slow"},
		{"/mnt/c", "cross filesystems"},
		{"C:\\Users\\me\\ws", ""},
		{"/home/me/ws", ""},
		{"/mnt2/data", ""},
		{"/mnt", ""},
	} {
		got := mountWarn(tc.root)
		if tc.want == "" {
			if got != "" {
				t.Errorf("mountWarn(%q) = %q, want silent", tc.root, got)
			}
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("mountWarn(%q) = %q, want it to mention %q", tc.root, got, tc.want)
		}
	}
}
