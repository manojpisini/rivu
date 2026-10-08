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
	Bottom       key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	HalfUp       key.Binding
	HalfDown     key.Binding
	Home         key.Binding
	End          key.Binding
	Refresh      key.Binding
	Open         key.Binding
	Detail       key.Binding
	Doctor       key.Binding
	Map          key.Binding
	Flow         key.Binding
	Delta        key.Binding
	Pick         key.Binding
	Actions      key.Binding
	Untriaged    key.Binding
	Source       key.Binding
	Copy         key.Binding
	Reveal       key.Binding
	Master       key.Binding
	Stats        key.Binding
	Settings     key.Binding
	Logs         key.Binding
	Confluence   key.Binding
	Help         key.Binding
	StageDigit   key.Binding
	StagePrev    key.Binding
	StageNext    key.Binding
}

// ShortHelp feeds the footer (help.Model.View).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.SwitchPanel, k.Up, k.Down, k.Search, k.Clear, k.Open, k.Refresh, k.Quit}
}

// FullHelp feeds the future help screen (spec 3.x); grouped by column.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.SwitchPanel, k.Up, k.Down, k.PageUp, k.PageDown, k.HalfUp, k.HalfDown, k.Bottom, k.Home, k.End, k.StageDigit, k.StagePrev, k.StageNext},
		{k.Master, k.Stats, k.Settings, k.Logs, k.Search, k.Clear, k.Open, k.Detail, k.Confluence, k.Doctor, k.Map, k.Flow, k.Delta, k.Pick, k.Actions, k.Untriaged, k.Source, k.Refresh, k.Copy, k.Reveal},
		{k.Help, k.Quit},
	}
}

var keys = keyMap{
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	SwitchPanel:  key.NewBinding(key.WithKeys("tab", "left", "right"), key.WithHelp("tab", "switch panel")),
	Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
	SearchDone:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "done")),
	SearchCancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Clear:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter")),
	Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k", "up")),
	Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j", "down")),
	Bottom:       key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
	PageUp:       key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	PageDown:     key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
	HalfUp:       key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half up")),
	HalfDown:     key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half down")),
	Home:         key.NewBinding(key.WithKeys("home"), key.WithHelp("home", "first row")),
	End:          key.NewBinding(key.WithKeys("end"), key.WithHelp("end", "last row")),
	Refresh:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
	Detail:       key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "detail")),
	Doctor:       key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "doctor")),
	Map:          key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "map report")),
	Flow:         key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "flow")),
	Delta:        key.NewBinding(key.WithKeys("A"), key.WithHelp("A", "delta (archive)")),
	Pick:         key.NewBinding(key.WithKeys(" "), key.WithHelp("space", "pick")),
	Actions:      key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "actions menu")),
	Untriaged:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "untriaged filter")),
	Source:       key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "source")),
	Copy:         key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy path")),
	Reveal:       key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "reveal")),
	Master:       key.NewBinding(key.WithKeys("g", "m"), key.WithHelp("g", "master")),
	Stats:        key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stats")),
	Settings:     key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "settings")),
	Logs:         key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "logs")),
	Confluence:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "confluences")),
	Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	StageDigit:   key.NewBinding(key.WithKeys("1", "2", "3", "4", "5"), key.WithHelp("1-5", "switch stage")),
	StagePrev:    key.NewBinding(key.WithKeys("left"), key.WithHelp("left", "prev stage")),
	StageNext:    key.NewBinding(key.WithKeys("right"), key.WithHelp("right", "next stage")),
}
