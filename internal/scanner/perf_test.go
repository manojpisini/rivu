package scanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeTree creates n project-shaped directories under root (perf pass
// fixture: P6.18 walks 5,000 dirs).
func makeTree(tb testing.TB, root string, n int) {
	tb.Helper()
	for i := range n {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("proj-%04d", i)), 0o755); err != nil {
			tb.Fatalf("mkdir: %v", err)
		}
	}
}

// TestScanProgressWithin200ms is the spec 3.x perf target: a scan must
// show progress within 200 ms so the TUI counter moves. The walk is
// cancelled at the first tick — the target is time-to-first-progress,
// not scan throughput (that is BenchmarkScan5000).
func TestScanProgressWithin200ms(t *testing.T) {
	root := t.TempDir()
	makeTree(t, root, 5000)

	s := New(nil, 6)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	var first time.Duration
	seen := false
	_, _, err := s.ScanContext(ctx, root, func(int) {
		if !seen {
			seen, first = true, time.Since(start)
			cancel()
		}
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("scan: %v", err)
	}
	if !seen {
		t.Fatal("scan reported no progress")
	}
	if first > 200*time.Millisecond {
		t.Errorf("first progress after %v, want <= 200ms", first)
	}
}

// BenchmarkScan5000 is the scan half of the perf pass (P6.18); the
// render half is tui.BenchmarkView5000 (16 ms budget).
func BenchmarkScan5000(b *testing.B) {
	root := b.TempDir()
	makeTree(b, root, 5000)
	s := New(nil, 6)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := s.Scan(root); err != nil {
			b.Fatal(err)
		}
	}
}
