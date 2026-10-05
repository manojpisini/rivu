// Package editorlaunch resolves and renders editor commands for `rivu open`.
package editorlaunch

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unicode"

	"github.com/manojpisini/rivu/internal/config"
)

// Resolve picks the editor command for a project (E-02): per-language
// override → configured default → $VISUAL → $EDITOR → OS default. The
// literal "${EDITOR}" in config means "defer to the environment".
func Resolve(c config.Config, language string) string {
	if ed := c.Editors.PerLanguage[strings.ToLower(language)]; ed != "" {
		return ed
	}
	if ed := c.Editors.Default; ed != "" && ed != "${EDITOR}" && ed != "${VISUAL}" {
		return ed
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return "code"
	}
	return "vi"
}

// Parse splits an editor command line into argv (E-01): double and single
// quotes group words, backslashes are literal so Windows paths
// ("C:\Program Files\…\Code.exe" --wait) survive intact. General
// shell-escape semantics are deliberately not implemented — this is an
// editor command, not a shell script.
func Parse(editor string) ([]string, error) {
	var (
		parts   []string
		cur     strings.Builder
		quote   rune
		started bool
	)
	flush := func() {
		if started {
			parts = append(parts, cur.String())
			cur.Reset()
			started = false
		}
	}
	for _, r := range editor {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			started = true
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("editor command %q has an unclosed quote — fix [editors] in the config", editor)
	}
	flush()
	if len(parts) == 0 {
		return nil, fmt.Errorf("editor command is empty — set [editors].default in the config")
	}
	return parts, nil
}

// IsGUI reports whether the first word of an editor command line names a
// GUI editor from the configured [editors].gui list (B-05): GUI editors are
// launched with Start(), terminal editors get the TTY via Run().
func IsGUI(editor string, gui []string) bool {
	parts, err := Parse(editor)
	if err != nil {
		return false
	}
	base := parts[0]
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		base = strings.TrimSuffix(base, ext)
	}
	for _, g := range gui {
		if base == strings.ToLower(g) {
			return true
		}
	}
	return false
}
