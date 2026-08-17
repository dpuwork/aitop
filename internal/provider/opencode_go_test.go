package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeGoPollMapsUsage(t *testing.T) {
	reset := "2026-08-18T12:00:00Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q, want application/json", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"monthly":{"status":"warning","percent":99,"resetsAt":"` + reset + `"},"rolling":{"status":"ok","percent":12,"resetsAt":"0001-01-01T00:00:00Z"},"weekly":{"status":"limited","percent":45}}}`))
	}))
	defer server.Close()

	p := &OpenCodeGo{URL: server.URL, Client: server.Client(), ResolveAPIKey: func() (string, error) { return "test-key", nil }}
	snap := p.Poll(context.Background())

	if snap.Provider != "OpenCode Go" || snap.Status != StatusOK || snap.Err != nil {
		t.Fatalf("snapshot = %#v, want successful OpenCode Go snapshot", snap)
	}
	if snap.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
	if got, want := len(snap.Windows), 3; got != want {
		t.Fatalf("window count = %d, want %d", got, want)
	}
	wantNames := []string{"rolling", "weekly", "monthly"}
	wantPercents := []int{12, 45, 99}
	for i, w := range snap.Windows {
		if w.Name != wantNames[i] || w.Percent != wantPercents[i] {
			t.Errorf("window %d = %#v, want %s at %d%%", i, w, wantNames[i], wantPercents[i])
		}
	}
	if snap.Windows[0].HasReset || !snap.Windows[0].ResetsAt.IsZero() {
		t.Errorf("rolling reset = %#v, want absent", snap.Windows[0])
	}
	if !snap.Windows[2].HasReset || !snap.Windows[2].ResetsAt.Equal(time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("monthly reset = %#v, want %s", snap.Windows[2], reset)
	}
	if snap.Summary != "weekly: limited  monthly: warning  " {
		t.Errorf("summary = %q, want status summary", snap.Summary)
	}
}

func TestOpenCodeGoPollJSONVariants(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		status      int
		wantStatus  Status
		wantErr     string
		wantWindows int
	}{
		{name: "missing usage is successful empty data", body: `{}`, status: http.StatusOK, wantStatus: StatusOK},
		{name: "null usage is successful empty data", body: `{"usage":null}`, status: http.StatusOK, wantStatus: StatusOK},
		{name: "unknown windows ignored", body: `{"usage":{"daily":{"percent":1},"weekly":{"percent":2}}}`, status: http.StatusOK, wantStatus: StatusOK, wantWindows: 1},
		{name: "malformed JSON", body: `{"usage":`, status: http.StatusOK, wantStatus: StatusError, wantErr: "parse response"},
		{name: "invalid reset time", body: `{"usage":{"weekly":{"resetsAt":"not-a-time"}}}`, status: http.StatusOK, wantStatus: StatusError, wantErr: "parse response"},
		{name: "server error", body: `{"error":"nope"}`, status: http.StatusUnauthorized, wantStatus: StatusError, wantErr: "http 401"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			p := &OpenCodeGo{URL: server.URL, Client: server.Client(), ResolveAPIKey: func() (string, error) { return "key", nil }}
			snap := p.Poll(context.Background())
			if snap.Status != tt.wantStatus {
				t.Fatalf("status = %v, want %v (err: %v)", snap.Status, tt.wantStatus, snap.Err)
			}
			if len(snap.Windows) != tt.wantWindows {
				t.Errorf("window count = %d, want %d", len(snap.Windows), tt.wantWindows)
			}
			if tt.wantErr != "" && (snap.Err == nil || !strings.Contains(snap.Err.Error(), tt.wantErr)) {
				t.Errorf("err = %v, want substring %q", snap.Err, tt.wantErr)
			}
		})
	}
}

func TestOpenCodeGoPollResolverErrorIsUnavailable(t *testing.T) {
	want := errors.New("credentials missing")
	p := &OpenCodeGo{ResolveAPIKey: func() (string, error) { return "", want }}
	snap := p.Poll(context.Background())
	if snap.Status != StatusError || !errors.Is(snap.Err, ErrUnavailable) || !errors.Is(snap.Err, want) {
		t.Fatalf("snapshot = %#v, want unavailable wrapped resolver error", snap)
	}
}
