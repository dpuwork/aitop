package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	helpStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	panelStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	panelTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	barFilledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	barEmptyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// remainingStyle picks a color by how much quota is left: green when
// plenty remains, yellow getting close, red running out.
func remainingStyle(remaining int) lipgloss.Style {
	switch {
	case remaining < 20:
		return errStyle
	case remaining < 50:
		return warnStyle
	default:
		return okStyle
	}
}

func percentBar(remaining, width int) string {
	if width < 4 {
		width = 4
	}
	filled := remaining * width / 100
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return barFilledStyle.Render(strings.Repeat("█", filled)) +
		barEmptyStyle.Render(strings.Repeat("░", width-filled))
}
