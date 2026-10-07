package tui

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestNoStdoutStderrWriteInTUISources (P3.32, static): nothing in the
// TUI package prints while the alt screen is up — the single allowed
// write is the OSC 52 clipboard escape, identified by its call site.
func TestNoStdoutStderrWriteInTUISources(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "osc52(path)") {
				continue // sanctioned escape (P3.29)
			}
			for _, bad := range []string{"fmt.Print", "println(", "os.Stdout", "os.Stderr", "log.Print"} {
				if strings.Contains(line, bad) {
					t.Errorf("%s:%d writes to the terminal: %s", f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}

// captureStdio swaps os.Stdout and os.Stderr for pipes.
func captureStdio(t *testing.T) (done func() (out, errOut string)) {
	t.Helper()
	or, ow, _ := os.Pipe()
	er, ew, _ := os.Pipe()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = ow, ew
	outC, errC := make(chan string, 1), make(chan string, 1)
	go func() { b, _ := io.ReadAll(or); outC <- string(b) }()
	go func() { b, _ := io.ReadAll(er); errC <- string(b) }()
	return func() (string, string) {
		os.Stdout, os.Stderr = oldOut, oldErr
		ow.Close()
		ew.Close()
		return <-outC, <-errC
	}
}

// TestUpdateAndViewKeepTheFrameToThemselves (P3.32, runtime): drive a
// broad sweep of key paths with stdout/stderr piped — the frame must
// not leak a single byte. Commands that would print or launch a GUI
// (copy/reveal/open) are excluded from invocation; everything else runs
// its command and feeds the result back in.
func TestUpdateAndViewKeepTheFrameToThemselves(t *testing.T) {
	finish := captureStdio(t)

	r, _ := rootOf(t)
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	drive := []tea.KeyMsg{
		keyR('j'), keyR('k'), keyR('g'), keyR('G'),
		keyR('2'), keyR('1'),
		runeKey("/"), keyR('s'), keyR('o'), keyR('u'), keyR('r'), keyR('c'), keyR('e'),
		keyEsc(),
		runeKey("d"), keyEsc(),
		runeKey("?"), keyEsc(),
		runeKey("f"), tea.KeyMsg{Type: tea.KeyDown}, keyEsc(),
		runeKey("h"), // doctor (fake service)
		runeKey("a"), // map (fake service)
		runeKey("r"), // scan (fake service)
	}
	for _, k := range drive {
		var cmd tea.Cmd
		r, cmd = upd(t, r, k)
		_ = r.View()
		if cmd == nil {
			continue
		}
		msg := cmd()
		// never invoke launch-capable commands here (y/o/enter): their
		// side effects are covered by copy_reveal/open tests instead
		if _, isCopy := msg.(copyDoneMsg); isCopy {
			continue
		}
		if _, isReveal := msg.(revealDoneMsg); isReveal {
			continue
		}
		if _, isExec := msg.(tea.ExecCommand); isExec {
			continue
		}
		r, _ = upd(t, r, msg)
		_ = r.View()
	}
	_ = r.View()

	out, errOut := finish()
	if out != "" {
		t.Errorf("stdout leaked %d bytes: %q", len(out), out)
	}
	if errOut != "" {
		t.Errorf("stderr leaked %d bytes: %q", len(errOut), errOut)
	}
}
