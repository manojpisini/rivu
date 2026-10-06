package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Badge styles are built once from the theme (AGENTS 8): badges are
// the primary status-communication device (spec 3.10).
var (
	badgeGood   = lipgloss.NewStyle().Foreground(theme.Good)
	badgeWarn   = lipgloss.NewStyle().Foreground(theme.Warn)
	badgeBad    = lipgloss.NewStyle().Foreground(theme.Bad)
	badgeAccent = lipgloss.NewStyle().Foreground(theme.Accent)
	badgeMuted  = lipgloss.NewStyle().Foreground(theme.Muted)
	badgeText   = lipgloss.NewStyle().Foreground(theme.Text)
)

// badge renders [label] in the given style.
func badge(style lipgloss.Style, label string) string {
	return style.Render("[" + label + "]")
}

// stageBadge colours a Flow stage: source warns (untriaged), active is
// good, research accents, everything else sits muted.
func stageBadge(stage string) string {
	label := strings.ToUpper(stage)
	switch stage {
	case "active":
		return badge(badgeGood, label)
	case "source":
		return badge(badgeWarn, label)
	case "research":
		return badge(badgeAccent, label)
	default:
		return badge(badgeMuted, label)
	}
}

// languageBadge renders [GO] style stack chips.
func languageBadge(lang string) string {
	if lang == "" {
		lang = "Unknown"
	}
	return badge(badgeText, strings.ToUpper(lang))
}

// gitBadge renders [Git]; colour alone carries presence.
func gitBadge(has bool) string {
	if has {
		return badge(badgeGood, "Git")
	}
	return badge(badgeBad, "Git")
}

// bankBadge renders [Bank:OK] / [Bank:missing].
func bankBadge(has bool) string {
	if has {
		return badge(badgeGood, "Bank:OK")
	}
	return badge(badgeBad, "Bank:missing")
}

// mapBadge renders [Map:OK] / [Map:Missing].
func mapBadge(has bool) string {
	if has {
		return badge(badgeGood, "Map:OK")
	}
	return badge(badgeWarn, "Map:Missing")
}

// healthBadge renders [Health:72] using the spec 3.6 band edges:
// >=70 good, 50-69 warn, <50 bad.
func healthBadge(score int) string {
	st := badgeBad
	switch {
	case score >= 70:
		st = badgeGood
	case score >= 50:
		st = badgeWarn
	}
	return badge(st, fmt.Sprintf("Health:%d", score))
}
