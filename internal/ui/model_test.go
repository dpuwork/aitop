package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dpuwork/aitop/internal/provider"
)

type testProvider struct {
	name  string
	snap  provider.Snapshot
	calls chan struct{}
}

func (p testProvider) Name() string { return p.name }
func (p testProvider) Poll(context.Context) provider.Snapshot {
	if p.calls != nil {
		p.calls <- struct{}{}
	}
	return p.snap
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestNewCopiesInitialSnapshotsAndInitializesState(t *testing.T) {
	initial := []provider.Snapshot{{Provider: "one", Status: provider.StatusOK}}
	m := New([]provider.Provider{testProvider{name: "one"}, testProvider{name: "two"}}, []time.Duration{time.Hour, time.Hour}, initial)
	initial[0].Provider = "changed"
	if m.snaps[0].Provider != "one" || m.snaps[1].Status != provider.StatusUnknown {
		t.Fatalf("unexpected snapshots: %#v", m.snaps)
	}
	if !m.relativeDates || len(m.polling) != 2 {
		t.Fatalf("unexpected initial state: %#v", m)
	}
}

func TestInitSchedulesInitialAndScannedProviders(t *testing.T) {
	calls := make(chan struct{}, 1)
	m := New([]provider.Provider{
		testProvider{name: "scanned"},
		testProvider{name: "new", snap: provider.Snapshot{Status: provider.StatusOK}, calls: calls},
	}, []time.Duration{0, 0}, []provider.Snapshot{{Status: provider.StatusOK}, {}})
	cmd := m.Init()
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 4 {
		t.Fatalf("Init command = %T with %d commands, want 4-command batch", cmd(), len(batch))
	}
	var polled bool
	for _, command := range batch {
		if msg := command(); msg != nil {
			if _, ok := msg.(pollMsg); ok {
				polled = true
			}
		}
	}
	if !polled {
		t.Error("Init did not include an initial poll command")
	}
	select {
	case <-calls:
	default:
		t.Error("initial poll command did not call provider")
	}
}

func TestUpdateKeyHandling(t *testing.T) {
	m := New(nil, nil, nil)
	for _, r := range []rune{'q', 'r', 'd'} {
		updated, _ := m.Update(key(r))
		got := updated.(Model)
		switch r {
		case 'q':
			if !got.quitting {
				t.Error("q did not set quitting")
			}
		case 'r':
			if !got.spinning {
				t.Error("refresh did not start spinner")
			}
		case 'd':
			if got.relativeDates {
				t.Error("d did not toggle relative dates")
			}
		}
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !updated.(Model).quitting || cmd == nil {
		t.Error("ctrl+c did not quit")
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !updated.(Model).quitting || cmd == nil {
		t.Error("esc did not quit")
	}
}

func TestRefreshStartsEveryProviderPoll(t *testing.T) {
	calls := make(chan struct{}, 2)
	m := New([]provider.Provider{
		testProvider{name: "one", calls: calls},
		testProvider{name: "two", calls: calls},
	}, []time.Duration{time.Hour, time.Hour}, nil)
	updated, cmd := m.Update(key('r'))
	m = updated.(Model)
	if cmd == nil || !m.polling[0] || !m.polling[1] || !m.spinning {
		t.Fatal("refresh did not mark every provider as polling")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatalf("refresh command = %T with %d commands, want two polls and spinner", cmd(), len(batch))
	}
	for _, command := range batch[:2] {
		if _, ok := command().(pollMsg); !ok {
			t.Errorf("refresh command returned %T, want pollMsg", command())
		}
	}
	if len(calls) != 2 {
		t.Fatalf("refresh called %d providers, want 2", len(calls))
	}
}

func TestUpdatePollTickAndCompletionScheduleNextPoll(t *testing.T) {
	p := testProvider{snap: provider.Snapshot{Status: provider.StatusOK}}
	m := New([]provider.Provider{p}, []time.Duration{0}, nil)
	updated, cmd := m.Update(tickMsg{index: 0})
	m = updated.(Model)
	if !m.polling[0] || !m.spinning || cmd == nil {
		t.Fatal("tick did not start polling and spinner")
	}
	updated, cmd = m.Update(pollMsg{index: 0, snap: p.snap})
	m = updated.(Model)
	if m.polling[0] || cmd == nil {
		t.Fatal("poll completion did not clear polling or schedule refresh")
	}
	if _, ok := cmd().(tickMsg); !ok {
		t.Fatalf("completion command returned %T, want tickMsg", cmd())
	}
}

func TestUpdateSpinnerStopsOnlyWhenAllPollsFinish(t *testing.T) {
	m := New(nil, nil, nil)
	m.polling = []bool{true, false}
	m.spinning = true
	updated, cmd := m.Update(spinnerTickMsg{})
	if cmd == nil || !updated.(Model).spinning {
		t.Error("spinner stopped while a provider was polling")
	}
	m = updated.(Model)
	m.polling[0] = false
	updated, cmd = m.Update(spinnerTickMsg{})
	if cmd != nil || updated.(Model).spinning {
		t.Error("spinner did not stop after all providers finished")
	}
}

func TestUpdateUnknownMessageIsNoop(t *testing.T) {
	m := New(nil, nil, nil)
	updated, cmd := m.Update(errors.New("ignored"))
	if cmd != nil || updated.(Model).quitting {
		t.Error("unknown message changed model")
	}
}
