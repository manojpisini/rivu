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

// badge renders an icon + label in the given style (P10.04: Nerd
// Font glyph, colour carries the state).
func badge(style lipgloss.Style, label string) string {
	return style.Render(label)
}

func iconBadge(style lipgloss.Style, icon, label string) string {
	return style.Render(icon + " " + label)
}

// stageIcon maps a Flow stage to its Nerd glyph (P10.04).
func stageIcon(stage string) string {
	switch stage {
	case "active":
		return iconActive
	case "source":
		return iconSource
	case "maintenance":
		return iconMaint
	case "research":
		return iconResearch
	case "delta":
		return iconDelta
	default:
		return iconMap
	}
}

// stageBadge colours a Flow stage: source warns (untriaged), active is
// good, research accents, everything else sits muted.
func stageBadge(stage string) string {
	label := strings.ToUpper(stage)
	icon := stageIcon(stage)
	switch stage {
	case "active":
		return iconBadge(badgeGood, icon, label)
	case "source":
		return iconBadge(badgeWarn, icon, label)
	case "research":
		return iconBadge(badgeAccent, icon, label)
	default:
		return iconBadge(badgeMuted, icon, label)
	}
}

// languageBadge renders [GO] style stack chips.
func languageBadge(lang string) string {
	if lang == "" {
		lang = "Unknown"
	}
	return badge(badgeText, strings.ToUpper(lang))
}

// gitBadge renders the git glyph; colour alone carries presence.
func gitBadge(has bool) string {
	if has {
		return iconBadge(badgeGood, iconGit, "Git")
	}
	return iconBadge(badgeBad, iconGit, "Git")
}

// bankBadge renders Bank:OK / Bank:missing.
func bankBadge(has bool) string {
	if has {
		return iconBadge(badgeGood, iconBank, "Bank:OK")
	}
	return iconBadge(badgeBad, iconBank, "Bank:missing")
}

// mapBadge renders Map:OK / Map:Missing.
func mapBadge(has bool) string {
	if has {
		return iconBadge(badgeGood, iconMap, "Map:OK")
	}
	return iconBadge(badgeWarn, iconMap, "Map:Missing")
}

// healthBadge renders Health:72 using the spec 3.6 band edges:
// >=70 good, 50-69 warn, <50 bad.
func healthBadge(score int) string {
	st := badgeBad
	switch {
	case score >= 70:
		st = badgeGood
	case score >= 50:
		st = badgeWarn
	}
	return iconBadge(st, iconHealth, fmt.Sprintf("Health:%d", score))
}

// scoreBar renders a 0-100 score as a 20-cell bar (spec P4.13),
// coloured by the health band; asciiSwap maps the blocks for ASCII
// terminals.
func scoreBar(score int) string {
	score = max(0, min(100, score))
	st := badgeBad
	switch {
	case score >= 70:
		st = badgeGood
	case score >= 50:
		st = badgeWarn
	}
	filled := score / 5
	return st.Render(strings.Repeat("█", filled)) + badgeMuted.Render(strings.Repeat("░", 20-filled))
}
