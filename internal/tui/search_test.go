package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

func searchFixture() Model {
	return New([]registry.Project{
		{ID: "1", Name: "OpenCourses", Slug: "opencourses", Path: `C:\ws\courses`,
			Stack: []string{"TypeScript", "Astro", "Bun"}, Language: "TypeScript", FlowStage: "source"},
		{ID: "2", Name: "Rivu", Slug: "rivu", Path: `C:\ws\rivu`,
			Stack: []string{"Go", "Bubble Tea"}, Language: "Go", FlowStage: "active"},
	}, `C:\ws`)
}

// TestFieldQuerySyntax: `field:value` restricts a field, plain tokens
// match the whole record, tokens AND together, unknown fields fall
// back to plain text (P3.13).
func TestFieldQuerySyntax(t *testing.T) {
	cases := []struct {
		query string
		want  []string // names
	}{
		{"", []string{"Rivu", "OpenCourses"}}, // sorted by stage then name
		{"slug:rivu", []string{"Rivu"}},
		{"name:course", []string{"OpenCourses"}},
		{query: `path:courses`, want: []string{"OpenCourses"}},
		{query: "stack:bubble", want: []string{"Rivu"}},
		{query: "lang:typescript", want: []string{"OpenCourses"}},
		{query: "lang:go", want: []string{"Rivu"}},
		{query: "lang:go name:rivu", want: []string{"Rivu"}},
		{query: "lang:go name:course", want: []string{}},
		{query: "nosuchfield:x", want: []string{}}, // unknown field -> literal text
		{query: "rivu", want: []string{"Rivu"}},    // plain haystack (name/slug/path/lang/flow/stack)
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			m := searchFixture()
			m.Query = c.query
			m.applyFilter()
			var got []string
			for _, p := range m.Visible {
				got = append(got, p.Name)
			}
			if len(got) != len(c.want) {
				t.Fatalf("query %q matched %v, want %v", c.query, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("query %q matched %v, want %v", c.query, got, c.want)
				}
			}
		})
	}
}

// TestSearchTextInputFlow: `/` opens the textinput, typed runes drive
// the filter, esc clears and closes, enter keeps the filter active.
func TestSearchTextInputFlow(t *testing.T) {
	m := searchFixture()
	m = update(t, m, runeKey("/"))
	if !m.search.Focused() {
		t.Fatal("/ must open search")
	}
	for _, r := range "slug:riv" {
		m, _ = updateC(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	// The filter is debounced (P3.36): the pending tick applies it.
	m, _ = updateC(t, m, searchTickMsg{gen: m.searchGen})
	if m.Query != "slug:riv" {
		t.Fatalf("input value = %q, want slug:riv", m.Query)
	}
	if len(m.Visible) != 1 || m.Visible[0].Name != "Rivu" {
		t.Fatalf("visible = %+v, want only Rivu", m.Visible)
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.search.Focused() {
		t.Fatal("enter must leave search mode")
	}
	if len(m.Visible) != 1 {
		t.Fatal("filter must stay active after enter")
	}
	m = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.Query != "" || m.search.Value() != "" {
		t.Fatalf("esc must clear input, query=%q input=%q", m.Query, m.search.Value())
	}
	if len(m.Visible) != 2 {
		t.Fatal("cleared search must show every project again")
	}
}
