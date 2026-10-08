// Package logx opens the rotating JSON file logger at
// <home>/logs/rivu.log (1 MB, three generations), the log the Logs
// screen tails (P5.10).
package logx

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	// maxBytes is the size that triggers rotation: 1 MB per file.
	maxBytes = 1 << 20
	// generations is how many rotated files are kept (.1 .2 .3).
	generations = 3
)

// Open creates <dir>/logs (0700), rotates rivu.log if it reached 1 MB,
// and returns a JSON slog logger appending to it (0600) plus the file's
// closer — the caller must close it before the process (or test temp
// dir) goes away, Windows refuses to delete open files. verbose lowers
// the level to debug.
//
// Rotation runs once per open: a single long run may grow the file
// past 1 MB and the next invocation rotates it (fine for a CLI; a
// daemon would rotate on write instead).
func Open(dir string, verbose bool) (*slog.Logger, io.Closer, error) {
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0700); err != nil {
		return nil, nil, fmt.Errorf("create %s: %w", logs, err)
	}
	path := Path(dir)
	if err := rotate(path); err != nil {
		return nil, nil, fmt.Errorf("rotate %s: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})), f, nil
}

// Path is where Open writes and where the Logs screen tails (P5.10).
func Path(dir string) string {
	return filepath.Join(dir, "logs", "rivu.log")
}

// tailWindow is how much of the file's end Tail reads; the log rotates
// at 1 MB so 512 KB is at most half of any single generation.
const tailWindow = 512 << 10

// Tail returns the last n lines of path, oldest first. A missing file
// is not an error — the Logs screen shows its empty state instead.
func Tail(path string, n int) ([]string, error) {
	if n <= 0 {
		return nil, nil
	}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	var off int64
	if st.Size() > tailWindow {
		off = st.Size() - tailWindow
	}
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek %s: %w", path, err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if off > 0 && len(lines) > 0 {
		lines = lines[1:] // the window can start mid-line
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// rotate shifts rivu.log -> .1 -> .2 -> .3 when the file reached
// maxBytes; the oldest generation falls off. Best effort: a missing
// file or generation is not an error.
func rotate(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if st.Size() < maxBytes {
		return nil
	}
	// rivu-allow-remove: delete only the oldest rotated log generation (.3)
	if err := os.Remove(fmt.Sprintf("%s.%d", path, generations)); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := generations - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", path, i)
		to := fmt.Sprintf("%s.%d", path, i+1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(path, path+".1")
}
