package slug

import (
	"errors"
	"testing"
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
