package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestDebugFrame(t *testing.T) {
	r := goldenRoot()
	r, _ = upd(t, r, tea.WindowSizeMsg{Width: 100, Height: 30})
	m := r.dashboard
	t.Logf("flowOrder=%d", len(flowOrder))
	h := bgStyle.Width(100).Render(m.header())
	bh := max(10, 30-lipgloss.Height(h)-lipgloss.Height(m.footer())-1)
	sb := m.sidebar(20, bh)
	for i, l := range strings.Split(sb, "\n") {
		t.Logf("sb%02d h=%d %q", i, lipgloss.Width(l), l)
	}
}
