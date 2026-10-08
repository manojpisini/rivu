// Package slug turns user-supplied project names into safe, portable
// folder slugs. Names that cannot be represented safely are rejected
// rather than silently mangled: path separators, traversal, characters
// invalid on Windows, and reserved device names all return an error.
package slug

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// ErrInvalid is returned when a name cannot become a safe slug.
var ErrInvalid = errors.New("name cannot be a slug")

// windowsReserved lists device names Windows refuses as a path element,
// with or without an extension (CON.txt is also invalid there).
var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// Make normalizes name into a slug: lowercased, runs of separators and
// punctuation collapsed to single hyphens, hyphen edges trimmed. It fails
// with an error wrapping ErrInvalid for names that are unsafe anywhere:
// empty, path separators, "." / "..", characters invalid on Windows,
// trailing dots, or reserved device names.
func Make(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("%w: %q is empty", ErrInvalid, name)
	}
	if strings.ContainsAny(trimmed, `/\`) {
		return "", fmt.Errorf("%w: %q must not contain path separators", ErrInvalid, name)
	}
	if trimmed == "." || trimmed == ".." {
		return "", fmt.Errorf("%w: %q is a traversal name", ErrInvalid, name)
	}
	if strings.HasSuffix(trimmed, ".") {
		return "", fmt.Errorf("%w: %q must not end with a dot", ErrInvalid, name)
	}
	for _, r := range trimmed {
		if r < 0x20 || strings.ContainsRune(`<>:"|?*`, r) {
			return "", fmt.Errorf("%w: %q contains the invalid character %q", ErrInvalid, name, r)
		}
	}
	base := trimmed
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if windowsReserved[strings.ToLower(base)] {
		return "", fmt.Errorf("%w: %q is a reserved device name on Windows", ErrInvalid, name)
	}

	var b strings.Builder
	for _, r := range strings.ToLower(trimmed) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "", fmt.Errorf("%w: %q normalizes to an empty slug", ErrInvalid, name)
	}
	return out, nil
}
