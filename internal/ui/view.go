package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/dpuwork/aitop/internal/provider"
)

const panelWidth = 72

// Version is set at build time via -ldflags "-X github.com/dpuwork/aitop/internal/ui.Version=...".
var Version = "0.0.1"

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	panels := make([]string, len(m.providers))
	for i, p := range m.providers {
		panels[i] = renderPanel(p.Name(), m.snaps[i], m.polling[i], m.relativeDates)
	}
	body := lipgloss.JoinVertical(lipgloss.Left, panels...)

	header := titleStyle.Render("[aitop]") + " " + dimStyle.Render("v"+Version)
	help := helpStyle.Render("Refresh: r | Dates: d | Quit: q")

	footerWidth := panelWidth + 2 // match panels' outer width (content + padding + border)
	gap := footerWidth - lipgloss.Width(header) - lipgloss.Width(help)
	if gap < 1 {
		gap = 1
	}
	footer := header + strings.Repeat(" ", gap) + help

	lines := []string{body, ""}
	if last := lastPollTime(m.snaps); !last.IsZero() {
		lines = append(lines, dimStyle.Render("last poll: "+formatTime(last, m.relativeDates, "15:04:05")))
	}
	lines = append(lines, footer)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// lastPollTime returns the most recent UpdatedAt across all snapshots, or
// the zero time if none have been polled yet.
func lastPollTime(snaps []provider.Snapshot) time.Time {
	var latest time.Time
	for _, s := range snaps {
		if s.UpdatedAt.After(latest) {
			latest = s.UpdatedAt
		}
	}
	return latest
}

// formatTime renders t either as a short relative offset ("in 4d 6h 32m" /
// "3m ago") or an absolute local time, depending on the user's
// date-display preference.
func formatTime(t time.Time, relative bool, absoluteLayout string) string {
	if relative {
		if d := time.Until(t); d >= 0 {
			return "in " + shortDuration(d)
		} else {
			return shortDuration(-d) + " ago"
		}
	}
	return t.Local().Format(absoluteLayout)
}

// shortDuration renders d as up to three whitespace-separated units
// (days/hours/minutes, or bare seconds under a minute), e.g. "4d 6h 32m".
func shortDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	d = d.Round(time.Minute)

	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if days > 0 || hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	parts = append(parts, fmt.Sprintf("%dm", minutes))
	return strings.Join(parts, " ")
}

func renderPanel(name string, snap provider.Snapshot, polling bool, relativeDates bool) string {
	var b strings.Builder

	var status string
	switch {
	case polling:
		status = dimStyle.Render("polling…")
	case snap.Status == provider.StatusOK:
		status = okStyle.Render("ok")
	case snap.Status == provider.StatusError:
		status = errStyle.Render("error")
	default:
		status = dimStyle.Render("waiting for first poll")
	}
	title := panelTitleStyle.Render(name)
	contentWidth := panelWidth - 2 // account for panelStyle's Padding(0, 1)
	gap := contentWidth - lipgloss.Width(title) - lipgloss.Width(status)
	if gap < 1 {
		gap = 1
	}
	b.WriteString(title + strings.Repeat(" ", gap) + status + "\n")

	if snap.Status == provider.StatusError && snap.Err != nil {
		b.WriteString(errStyle.Render("! "+snap.Err.Error()) + "\n")
	}

	if snap.Summary != "" {
		b.WriteString(snap.Summary + "\n")
	}

	for _, w := range snap.Windows {
		remaining := w.Remaining()
		style := remainingStyle(remaining)

		reset := ""
		if w.HasReset {
			reset = dimStyle.Render("resets " + formatTime(w.ResetsAt, relativeDates, "Jan 2 15:04"))
		}

		b.WriteString(fmt.Sprintf(
			"%-9s %s %s remaining  %s\n",
			w.Name,
			percentBar(remaining, 20),
			style.Render(fmt.Sprintf("%3d%%", remaining)),
			reset,
		))
	}

	for _, mt := range snap.Metrics {
		b.WriteString(fmt.Sprintf("%-26s %s\n", mt.Label+":", mt.Value))
	}

	content := strings.TrimRight(b.String(), "\n")
	if content == "" {
		content = dimStyle.Render("no data yet")
	}
	return panelStyle.Width(panelWidth).Render(content)
}
