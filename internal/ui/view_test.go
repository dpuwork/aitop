package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dpuwork/aitop/internal/provider"
)

func TestShortDuration(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{2*time.Minute + 31*time.Second, "3m"},
		{2*time.Hour + 3*time.Minute, "2h 3m"},
		{4*24*time.Hour + 6*time.Hour + 32*time.Minute, "4d 6h 32m"},
	} {
		if got := shortDuration(tt.d); got != tt.want {
			t.Errorf("shortDuration(%s) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatTimeRelativePastAndFuture(t *testing.T) {
	future := formatTime(time.Now().Add(2*time.Hour+2*time.Minute), true, "15:04")
	if !strings.HasPrefix(future, "in ") || !strings.Contains(future, "2h") {
		t.Errorf("future format = %q", future)
	}
	past := formatTime(time.Now().Add(-3*time.Minute), true, "15:04")
	if !strings.HasSuffix(past, " ago") || !strings.Contains(past, "3m") {
		t.Errorf("past format = %q", past)
	}
}

func TestPercentBarClampsAndHonorsMinimumWidth(t *testing.T) {
	for _, tt := range []struct {
		remaining int
		width     int
		want      int
	}{
		{-10, 20, 20},
		{150, 20, 20},
		{50, 20, 20},
		{50, 2, 4},
	} {
		bar := percentBar(tt.remaining, tt.width)
		if lipgloss.Width(bar) != tt.want {
			t.Errorf("percentBar(%d, %d) width = %d, want %d", tt.remaining, tt.width, lipgloss.Width(bar), tt.want)
		}
	}
}

func TestRenderPanelStatesAndData(t *testing.T) {
	w := provider.Window{Name: "weekly", Percent: 75, HasReset: true, ResetsAt: time.Now().Add(time.Hour)}
	snap := provider.Snapshot{Status: provider.StatusOK, Summary: "Current session: 1% used", Windows: []provider.Window{w}, Metrics: []provider.Metric{{Label: "Cost", Value: "$2"}}}
	for _, tt := range []struct {
		name    string
		snap    provider.Snapshot
		polling bool
		want    string
	}{
		{"waiting", provider.Snapshot{}, false, "waiting for first poll"},
		{"error", provider.Snapshot{Status: provider.StatusError, Err: &testError{"offline"}}, false, "! offline"},
		{"polling", snap, true, "⠋"},
		{"data", snap, false, "weekly"},
	} {
		got := renderPanel("Claude", tt.snap, tt.polling, true, 0)
		if !strings.Contains(got, tt.want) {
			t.Errorf("%s panel does not contain %q: %q", tt.name, tt.want, got)
		}
	}
	if got := renderPanel("Empty", provider.Snapshot{}, false, true, 0); strings.Contains(got, "no data yet") {
		t.Error("panel title prevents the no-data fallback from being rendered")
	}
}

type testError struct{ text string }

func (e *testError) Error() string { return e.text }

func TestViewIncludesPanelsFooterAndQuitIsBlank(t *testing.T) {
	m := New([]provider.Provider{testProvider{name: "Claude"}}, []time.Duration{time.Hour}, nil)
	view := m.View()
	for _, want := range []string{"Claude", "[aitop]", "v" + Version, "Refresh: r | Dates: d | Quit: q"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q", want)
		}
	}
	if lipgloss.Width(view) < panelWidth {
		t.Errorf("View() width = %d, want at least panel width %d", lipgloss.Width(view), panelWidth)
	}
	m.quitting = true
	if m.View() != "" {
		t.Error("quitting View() was not blank")
	}
}
