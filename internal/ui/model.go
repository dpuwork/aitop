// Package ui implements the Bubble Tea dashboard: one panel per usage
// provider, each polled independently on its own interval so a failure or
// slow response from one provider never blocks the others.
package ui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dpuwork/aitop/internal/provider"
)

type pollMsg struct {
	index int
	snap  provider.Snapshot
}

type tickMsg struct{ index int }

// clockMsg fires on a short fixed interval purely to force a re-render, so
// relative "last poll: Ns ago" / "resets in Xm" labels keep advancing
// between the much longer per-provider poll ticks.
type clockMsg struct{}

type Model struct {
	providers     []provider.Provider
	intervals     []time.Duration
	snaps         []provider.Snapshot
	polling       []bool
	quitting      bool
	relativeDates bool
}

func New(providers []provider.Provider, intervals []time.Duration) Model {
	return Model{
		providers:     providers,
		intervals:     intervals,
		snaps:         make([]provider.Snapshot, len(providers)),
		polling:       make([]bool, len(providers)),
		relativeDates: true,
	}
}

func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, len(m.providers)+1)
	for i := range m.providers {
		m.polling[i] = true
		cmds[i] = m.pollCmd(i)
	}
	cmds[len(m.providers)] = clockCmd()
	return tea.Batch(cmds...)
}

func clockCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return clockMsg{}
	})
}

func (m Model) pollCmd(i int) tea.Cmd {
	p := m.providers[i]
	return func() tea.Msg {
		return pollMsg{index: i, snap: p.Poll(context.Background())}
	}
}

func (m Model) scheduleCmd(i int) tea.Cmd {
	return tea.Tick(m.intervals[i], func(time.Time) tea.Msg {
		return tickMsg{index: i}
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "r":
			cmds := make([]tea.Cmd, len(m.providers))
			for i := range m.providers {
				m.polling[i] = true
				cmds[i] = m.pollCmd(i)
			}
			return m, tea.Batch(cmds...)
		case "d":
			m.relativeDates = !m.relativeDates
			return m, nil
		}

	case pollMsg:
		m.snaps[msg.index] = msg.snap
		m.polling[msg.index] = false
		return m, m.scheduleCmd(msg.index)

	case tickMsg:
		m.polling[msg.index] = true
		return m, m.pollCmd(msg.index)

	case clockMsg:
		return m, clockCmd()
	}

	return m, nil
}
