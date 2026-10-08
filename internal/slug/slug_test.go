package slug

import (
	"errors"
	"strings"
	"testing"
	"unicode"
)

func TestMakeNormalizes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"My Project", "my-project"},
		{"  padded name  ", "padded-name"},
		{"rivu_tool", "rivu-tool"},
		{"already-slug", "already-slug"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"Under_score__mix", "under-score-mix"},
		{"UPPER", "upper"},
		{"dots-and-dashes", "dots-and-dashes"},
		{"Le Café", "le-café"},
		{"a1 b2", "a1-b2"},
	} {
		got, err := Make(tc.in)
		if err != nil {
			t.Errorf("Make(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Make(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMakeRejectsUnsafeNames(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"../../x",
		"a/b",
		`a\b`,
		".",
		"..",
		"---",
		"!!!",
		"CON",
		"con",
		"nul.txt",
		"COM1",
		"lpt9.backup",
		"foo.",
		"bad:name",
		"question?",
		"star*",
		"pipe|",
		"quote\"",
		"less<",
		"tab\there",
	} {
		if _, err := Make(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("Make(%q) = %v, want ErrInvalid", in, err)
		}
	}
}

func FuzzMake(f *testing.F) {
	for _, seed := range []string{"", "  ", "My Project", "../../x", "CON", "a/b", `a\b`, "foo.", "---", "!!!", "rivu_tool", "Le Café", "..", "nul.txt", "tab\there", "already-slug"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		s, err := Make(name)
		if err != nil {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Make(%q) error %v does not wrap ErrInvalid", name, err)
			}
			return
		}
		if s == "" {
			t.Fatalf("Make(%q) returned an empty slug with no error", name)
		}
		if strings.ContainsAny(s, `/\`) {
			t.Fatalf("Make(%q) = %q contains a path separator", name, s)
		}
		if s != strings.Trim(s, "-") {
			t.Fatalf("Make(%q) = %q has untrimmed hyphens", name, s)
		}
		if s != strings.ToLower(s) {
			t.Fatalf("Make(%q) = %q is not lowercase", name, s)
		}
		if strings.Contains(s, "--") {
			t.Fatalf("Make(%q) = %q has uncollapsed hyphens", name, s)
		}
		for _, r := range s {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
				t.Fatalf("Make(%q) = %q contains forbidden rune %q", name, s, r)
			}
		}
	})
}
