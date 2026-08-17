// Package provider implements pollers for the usage data sources shown by
// the dashboard: Claude Code's CLI, OpenCode Go's HTTP API, and Codex's
// app-server JSON-RPC protocol — all backed by remote account data, not
// local history.
package provider

import (
	"context"
	"time"
)

// Status is the outcome of the most recent poll.
type Status int

const (
	StatusUnknown Status = iota
	StatusOK
	StatusError
)

// Window is a single quota window (rolling, weekly, monthly, ...) reported
// by a provider that exposes percent-used and reset-time data.
type Window struct {
	Name     string
	Percent  int // percent used, 0-100
	ResetsAt time.Time
	HasReset bool
}

// Remaining returns the percentage left in the window, clamped to [0, 100].
func (w Window) Remaining() int {
	r := 100 - w.Percent
	switch {
	case r < 0:
		return 0
	case r > 100:
		return 100
	default:
		return r
	}
}

// Metric is a single labeled value for freeform historical stats such as
// session counts, cost, or token totals.
type Metric struct {
	Label string
	Value string
}

// Snapshot is the result of a single poll of a provider.
type Snapshot struct {
	Provider  string
	UpdatedAt time.Time
	Status    Status
	Err       error

	// Summary is a short freeform status line, e.g. Claude's
	// "Current session: 1% used".
	Summary string

	// Windows holds structured percent/reset quota windows, when the
	// provider exposes them (OpenCode Go).
	Windows []Window

	// Metrics holds freeform historical values (sessions, cost, tokens),
	// when the provider only exposes local history (OpenCode local).
	Metrics []Metric
}

// Provider polls a single usage data source.
type Provider interface {
	Name() string
	Poll(ctx context.Context) Snapshot
}
