package editorlaunch

import (
	"runtime"
	"strings"
	"testing"

	"github.com/manojpisini/rivu/internal/config"
)

func TestResolveChain(t *testing.T) {
	c := config.Default()
	c.Editors.PerLanguage = map[string]string{"go": "nvim", "Go": "ignored-case"}

	if got := Resolve(c, "Go"); got != "nvim" {
		t.Errorf("per-language = %q, want nvim (case-insensitive)", got)
	}

	c.Editors.PerLanguage = map[string]string{"go": "nvim"}
	c.Editors.Default = "subl"
	if got := Resolve(c, "rust"); got != "subl" {
		t.Errorf("non-matching language = %q, want explicit default subl", got)
	}

	// Placeholder default defers to $VISUAL over $EDITOR.
	c.Editors.Default = "${EDITOR}"
	t.Setenv("VISUAL", "emacs -nw")
	t.Setenv("EDITOR", "vim")
	if got := Resolve(c, "rust"); got != "emacs -nw" {
		t.Errorf("VISUAL precedence = %q, want emacs -nw", got)
	}
	t.Setenv("VISUAL", "")
	if got := Resolve(c, "rust"); got != "vim" {
		t.Errorf("EDITOR fallback = %q, want vim", got)
	}

	// Explicit default beats the environment (ladder order).
	c.Editors.Default = "code"
	if got := Resolve(c, "rust"); got != "code" {
		t.Errorf("explicit default = %q, want code", got)
	}

	// Nothing configured or in the environment: OS default.
	c.Editors.Default = ""
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	want := "vi"
	if runtime.GOOS == "windows" {
		want = "code"
	}
	if got := Resolve(c, "rust"); got != want {
		t.Errorf("OS default = %q, want %q", got, want)
	}
}

func TestParseShellWords(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"vim", []string{"vim"}},
		{`"C:\Program Files\Microsoft VS Code\Code.exe" --reuse-window`, []string{`C:\Program Files\Microsoft VS Code\Code.exe`, "--reuse-window"}},
		{"nvim +'set ft=go'", []string{"nvim", "+set ft=go"}},
		{`code "dir with spaces"`, []string{"code", "dir with spaces"}},
	} {
		got, err := Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("Parse(%q) = %q, want %q", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Parse(%q) = %q, want %q", tc.in, got, tc.want)
				break
			}
		}
	}
	if _, err := Parse(""); err == nil {
		t.Error("empty editor must be an error")
	}
}

func TestParseRejectsDanglingQuote(t *testing.T) {
	if _, err := Parse(`code "unterminated`); err == nil {
		t.Fatal("want error for dangling quote")
	} else if !strings.Contains(err.Error(), "[editors]") {
		t.Errorf("error should hint at the config: %v", err)
	}
}

func TestIsGUI(t *testing.T) {
	gui := []string{"code", "zed", "subl", "idea"}
	for _, tc := range []struct {
		editor string
		want   bool
	}{
		{"code", true},
		{`C:\tools\Code.exe`, true},
		{`"C:\Program Files\Microsoft VS Code\Code.exe" --wait`, true},
		{"/usr/bin/code --wait", true},
		{"vim", false},
		{"/usr/bin/vi", false},
		{"", false},
	} {
		if got := IsGUI(tc.editor, gui); got != tc.want {
			t.Errorf("IsGUI(%q) = %v, want %v", tc.editor, got, tc.want)
		}
	}
	if IsGUI("code", nil) {
		t.Error("empty gui list must classify nothing as GUI")
	}
	if IsGUI(`code "`, gui) {
		t.Error("unparsable command must not classify as GUI")
	}
}
