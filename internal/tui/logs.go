package tui

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/manojpisini/rivu/internal/registry"
)

// logsFollowEvery is the follow-mode poll: how often the screen re-tails
// rivu.log while follow is on (P5.10).
const logsFollowEvery = 1500 * time.Millisecond

// logsLoadedMsg carries one tail + activity snapshot for the screen.
type logsLoadedMsg struct {
	lines []string
	act   []registry.Activity
	err   error
}

// logsTickMsg fires every logsFollowEvery while follow mode is on.
type logsTickMsg struct{}

// loadLogs tails rivu.log and reads activity_log together so one
// message repaints both sources.
func (r Root) loadLogs() tea.Cmd {
	svc := r.svc
	return func() tea.Msg {
		lines, err := svc.LogTail(500)
		act, actErr := svc.RecentActivity(100)
		return logsLoadedMsg{lines: lines, act: act, err: errors.Join(err, actErr)}
	}
}

// logsTickCmd schedules the next follow poll.
func logsTickCmd() tea.Cmd {
	return tea.Tick(logsFollowEvery, func(time.Time) tea.Msg { return logsTickMsg{} })
}

// logsVisible is how many content lines fit the window (title, source,
// blanks and footer around them).
func (r Root) logsVisible() int {
	if r.height == 0 {
		return 20
	}
	return max(5, r.height-6)
}

// logsRows is the content of the selected source.
func (r Root) logsRows() []string {
	if r.logsSrc == 1 {
		return r.activityRows()
	}
	return r.logsLines
}

// logsRowSet clamps a window start so scrolling can never leave the
// content.
func (r Root) logsRowSet(s int) int {
	n := len(r.logsRows())
	return min(max(s, 0), max(0, n-r.logsVisible()))
}

// logsKeys drives the screen: tab switches source, f toggles follow
// (manual scrolling pauses it), r re-tails, esc/q go back (P5.10).
func (r Root) logsKeys(x tea.KeyMsg) (tea.Model, tea.Cmd) {
	setScroll := func(d int) {
		base := r.logsScroll
		if r.logsFollow {
			base = r.logsRowSet(len(r.logsRows()))
		}
		r.logsFollow = false // manual scroll pauses follow
		r.logsScroll = r.logsRowSet(base + d)
	}
	switch x.String() {
	case "ctrl+c":
		return r, tea.Quit
	case "esc", "q":
		r.screen = ScreenDashboard
		return r, nil
	case "tab":
		r.logsSrc = 1 - r.logsSrc
		r.logsScroll = r.logsRowSet(0)
		return r, nil
	case "f":
		r.logsFollow = !r.logsFollow
		if r.logsFollow {
			return r, tea.Batch(r.loadLogs(), logsTickCmd())
		}
		r.logsScroll = r.logsRowSet(len(r.logsRows())) // paused at the newest lines
		return r, nil
	case "j", "down":
		setScroll(1)
	case "k", "up":
		setScroll(-1)
	case "pgdown":
		setScroll(r.logsVisible())
	case "pgup":
		setScroll(-r.logsVisible())
	case "home":
		r.logsFollow, r.logsScroll = false, 0
	case "end":
		setScroll(len(r.logsRows()))
	case "r":
		return r, r.loadLogs()
	}
	return r, nil
}

// logJSON is the slog JSON line the rotating log writes.
type logJSON struct {
	Time  string `json:"time"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// prettyLogLines renders each tail line as "15:04:05 LEVEL message";
// anything that is not valid JSON passes through untouched.
func prettyLogLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, ln := range lines {
		out[i] = prettyLogLine(ln)
	}
	return out
}

func prettyLogLine(s string) string {
	var e logJSON
	if err := json.Unmarshal([]byte(s), &e); err != nil || e.Msg == "" {
		return s
	}
	ts := e.Time
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		ts = t.Format("15:04:05")
	}
	return padCell(ts, 9) + padCell(strings.ToUpper(e.Level), 6) + e.Msg
}

// activityRows words activity_log the way the screen lists it: when,
// what event, which project (P5.10).
func (r Root) activityRows() []string {
	rows := make([]string, 0, len(r.logsAct))
	for _, a := range r.logsAct {
		rows = append(rows, a.OccurredAt.Format("2006-01-02 15:04")+
			"  "+padCell(a.Event, 9)+a.Name+" ("+a.Slug+")")
	}
	return rows
}

// logsView is the Logs screen (plan screen 13): a tail of the rotating
// rivu.log or activity_log, with follow mode pinning the window to the
// newest lines.
func (r Root) logsView() string {
	var b strings.Builder
	b.WriteString(r.styleTitle.Render("LOGS"))
	if !r.logsLoaded {
		b.WriteString("\n\n" + r.styleMuted.Render("Loading…"))
		b.WriteString("\n" + r.styleMuted.Render("esc back"))
		return b.String()
	}
	src, rows := "rivu.log", r.logsLines
	if r.logsSrc == 1 {
		src, rows = "activity_log", r.activityRows()
	}
	b.WriteString("\n" + r.styleMuted.Render(src))
	if r.logsErr != "" {
		b.WriteString("\n" + r.styleErr.Render(r.logsErr))
	}
	if len(rows) == 0 {
		empty := "no log entries yet - rivu writes them as it works"
		if r.logsSrc == 1 {
			empty = "no activity yet - open a project (enter) to record one"
		}
		b.WriteString("\n\n" + r.styleMuted.Render(empty))
	} else {
		start := r.logsRowSet(r.logsScroll)
		vis := r.logsVisible()
		b.WriteString("\n")
		for _, ln := range rows[start:min(start+vis, len(rows))] {
			if r.width > 0 {
				ln = shorten(ln, max(1, r.width-4))
			}
			b.WriteString("\n" + ln)
		}
	}
	follow := "off"
	if r.logsFollow {
		follow = "on"
	}
	b.WriteString("\n\n" + r.styleMuted.Render(
		"tab source  j/k scroll  f follow: "+follow+"  r refresh  esc back"))

	out := b.String()
	if r.width > 0 {
		ls := strings.Split(out, "\n")
		for i, ln := range ls {
			ls[i] = shorten(ln, max(1, r.width-2))
		}
		out = strings.Join(ls, "\n")
	}
	return out
}
