package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap is the single table of shortcuts: Update matches against these
// bindings and the footer renders them through the help component, so
// handlers and help text cannot drift apart (AGENTS section 8).
type keyMap struct {
	Quit         key.Binding
	SwitchPanel  key.Binding
	Search       key.Binding
	SearchDone   key.Binding
	SearchCancel key.Binding
	Clear        key.Binding
	Up           key.Binding
	Down         key.Binding
	Top          key.Binding
	Bottom       key.Binding
	Refresh      key.Binding
	Open         key.Binding
	Doctor       key.Binding
	Map          key.Binding
}

// ShortHelp feeds the footer (help.Model.View).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.SwitchPanel, k.Up, k.Down, k.Search, k.Clear, k.Open, k.Refresh, k.Quit}
}

// FullHelp feeds the future help screen (spec 3.x); grouped by column.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.SwitchPanel, k.Up, k.Down, k.Top, k.Bottom},
		{k.Search, k.Clear, k.Open, k.Doctor, k.Map, k.Refresh},
		{k.Quit},
	}
}

var keys = keyMap{
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	SwitchPanel:  key.NewBinding(key.WithKeys("tab", "left", "h", "right", "l"), key.WithHelp("tab", "switch panel")),
	Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	SearchDone:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "done")),
	SearchCancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Clear:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
	Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k", "up")),
	Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j", "down")),
	Top:          key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
	Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	Refresh:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Open:         key.NewBinding(key.WithKeys("enter", "o"), key.WithHelp("o", "open")),
	Doctor:       key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "doctor")),
	Map:          key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "build map")),
}
