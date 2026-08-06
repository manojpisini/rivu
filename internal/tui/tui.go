package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/manojpisini/rivu/internal/registry"
	"strings"
)

type Model struct {
	Projects []registry.Project
	Cursor   int
	Width    int
	Height   int
	Err      error
}

func New(ps []registry.Project) Model { return Model{Projects: ps} }
func (m Model) Init() tea.Cmd         { return nil }
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch x := msg.(type) {
	case tea.KeyMsg:
		switch x.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.Cursor > 0 {
				m.Cursor--
			}
		case "down", "j":
			if m.Cursor < len(m.Projects)-1 {
				m.Cursor++
			}
		}
	case tea.WindowSizeMsg:
		m.Width = x.Width
		m.Height = x.Height
	}
	return m, nil
}

var title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
var selected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
var muted = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(title.Render("RIVU · PROJECT FLOW SYSTEM") + "\n")
	b.WriteString(muted.Render("j/k navigate · q quit") + "\n\n")
	if len(m.Projects) == 0 {
		b.WriteString("No projects registered. Run `rivu scan` or `rivu source`.\n")
		return b.String()
	}
	for i, p := range m.Projects {
		line := fmt.Sprintf("%-2s %-24s [%-11s] %-12s Health:%3d  %s", marker(i == m.Cursor), p.Name, p.FlowStage, p.Language, p.HealthScore, p.Path)
		if i == m.Cursor {
			line = selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	p := m.Projects[m.Cursor]
	b.WriteString("\n" + title.Render("CURRENT VIEW") + "\n")
	b.WriteString(fmt.Sprintf("%s\nFlow: %s · Channel: %s · Git:%t · Bank:%t · Map:%t\n", p.Path, p.FlowStage, p.Channel, p.HasGit, p.HasBank, p.HasMap))
	return b.String()
}
func marker(v bool) string {
	if v {
		return ">"
	}
	return " "
}
func Run(ps []registry.Project) error {
	_, e := tea.NewProgram(New(ps), tea.WithAltScreen()).Run()
	return e
}
