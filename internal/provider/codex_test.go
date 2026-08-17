package provider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQueryCodexRateLimitsProtocol(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantResult string
		wantErr    string
		wantIs     error
	}{
		{
			name:       "success skips notifications malformed and unrelated responses",
			mode:       "success",
			wantResult: `{"rateLimits":{"planType":"pro","primary":{"usedPercent":23}}}`,
		},
		{
			name:    "initialize error",
			mode:    "init-error",
			wantErr: "initialize: codex app-server: bad initialize",
		},
		{
			name:    "rate limit error",
			mode:    "rate-error",
			wantErr: "account/rateLimits/read: codex app-server: rate limit failed",
		},
		{
			name:    "authentication error is unavailable",
			mode:    "auth-error",
			wantErr: "account/rateLimits/read: provider unavailable: codex account authentication required to read rate limits",
			wantIs:  ErrUnavailable,
		},
		{
			name:    "malformed stream ends unexpectedly",
			mode:    "malformed",
			wantErr: "initialize: unexpected EOF",
		},
		{
			name:    "oversized response is a transport error",
			mode:    "oversized",
			wantErr: "initialize: bufio.Scanner: token too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installFakeCodex(t, tt.mode)
			got, err := queryCodexRateLimits(context.Background())

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("queryCodexRateLimits() error = %v", err)
				}
				if string(got) != tt.wantResult {
					t.Fatalf("result = %s, want %s", got, tt.wantResult)
				}
			} else {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Fatalf("error = %v, want errors.Is(_, %v)", err, tt.wantIs)
				}
			}

			if tt.mode == "success" {
				assertCodexRequests(t)
			}
		})
	}
}

func TestQueryCodexRateLimitsCancellation(t *testing.T) {
	installFakeCodex(t, "cancel")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := queryCodexRateLimits(ctx)
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("query did not stop after context cancellation")
	}
}

func TestQueryCodexRateLimitsMissingCLI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	_, err := queryCodexRateLimits(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
	if !strings.Contains(err.Error(), "codex CLI not found in PATH") {
		t.Fatalf("error = %v, want missing CLI message", err)
	}
}

func TestCodexPoll(t *testing.T) {
	installFakeCodex(t, "success")
	snap := (&Codex{Timeout: time.Second}).Poll(context.Background())

	if snap.Status != StatusOK || snap.Err != nil {
		t.Fatalf("snapshot status/error = %v/%v, want OK/nil", snap.Status, snap.Err)
	}
	if snap.Provider != "Codex" || snap.UpdatedAt.IsZero() {
		t.Fatalf("snapshot identity/timestamp = %q/%v", snap.Provider, snap.UpdatedAt)
	}
	if len(snap.Windows) != 1 || snap.Windows[0].Percent != 23 || snap.Windows[0].Name != "window" {
		t.Fatalf("windows = %+v", snap.Windows)
	}
	if snap.Summary != "plan: pro" {
		t.Fatalf("summary = %q, want %q", snap.Summary, "plan: pro")
	}
}

func TestCodexPollTimeout(t *testing.T) {
	installFakeCodex(t, "cancel")
	snap := (&Codex{Timeout: 20 * time.Millisecond}).Poll(context.Background())

	if snap.Status != StatusError || !errors.Is(snap.Err, context.DeadlineExceeded) {
		t.Fatalf("status/error = %v/%v, want error/deadline exceeded", snap.Status, snap.Err)
	}
}

func TestParseCodexRateLimits(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		windows []Window
		summary string
	}{
		{
			name: "all fields and duration labels",
			raw:  `{"rateLimits":{"planType":"team","primary":{"usedPercent":42,"resetsAt":1700000000,"windowDurationMins":300},"secondary":{"usedPercent":101,"windowDurationMins":10080}}}`,
			windows: []Window{
				{Name: "5h", Percent: 42, ResetsAt: time.Unix(1700000000, 0), HasReset: true},
				{Name: "weekly", Percent: 101},
			},
			summary: "plan: team",
		},
		{name: "unknown plan and absent windows", raw: `{"rateLimits":{"planType":"unknown"}}`},
		{name: "invalid JSON", raw: `{`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := Snapshot{}
			err := parseCodexRateLimits(&snap, json.RawMessage(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if snap.Summary != tt.summary || len(snap.Windows) != len(tt.windows) {
				t.Fatalf("snapshot = %+v, want summary %q and windows %+v", snap, tt.summary, tt.windows)
			}
			for i := range tt.windows {
				if snap.Windows[i] != tt.windows[i] {
					t.Errorf("window %d = %+v, want %+v", i, snap.Windows[i], tt.windows[i])
				}
			}
		})
	}
}

func installFakeCodex(t *testing.T, mode string) {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "requests.log")
	script := `#!/bin/sh
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$CODEX_LOG"
  case "$CODEX_MODE:$line" in
    success:*'"id":1'*)
      printf '%s\n' '{"method":"notification"}' 'not json' '{"id":99,"result":{}}' '{"id":1,"result":{}}'
      ;;
    success:*'"id":2'*) printf '%s\n' '{"id":2,"result":{"rateLimits":{"planType":"pro","primary":{"usedPercent":23}}}}' ;;
    init-error:*'"id":1'*) printf '%s\n' '{"id":1,"error":{"message":"bad initialize"}}' ;;
    rate-error:*'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    rate-error:*'"id":2'*) printf '%s\n' '{"id":2,"error":{"message":"rate limit failed"}}' ;;
    auth-error:*'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    auth-error:*'"id":2'*) printf '%s\n' '{"id":2,"error":{"message":"codex account authentication required to read rate limits"}}' ;;
    malformed:*) printf '%s\n' 'not json'; exit 0 ;;
    oversized:*) printf '{"id":1,"result":"'; printf '%05000000d' 0; printf '%s\n' '"}' ;;
    cancel:*'"id":1'*) printf '%s\n' '{"id":1,"result":{}}' ;;
    cancel:*'"id":2'*) exec sleep 30 ;;
  esac
done
`
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_MODE", mode)
	t.Setenv("CODEX_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func assertCodexRequests(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("CODEX_LOG"))
	if err != nil {
		t.Fatal(err)
	}
	var requests []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var request map[string]any
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			t.Fatalf("request %q is not JSON: %v", line, err)
		}
		requests = append(requests, request)
	}
	if len(requests) != 3 {
		t.Fatalf("got %d requests, want initialize, initialized, rate limit read", len(requests))
	}
	if requests[0]["id"] != float64(1) || requests[0]["method"] != "initialize" {
		t.Fatalf("initialize request = %#v", requests[0])
	}
	if requests[0]["params"].(map[string]any)["clientInfo"].(map[string]any)["name"] != "aitop" {
		t.Fatalf("client info = %#v", requests[0]["params"])
	}
	if requests[1]["method"] != "initialized" || requests[1]["id"] != nil {
		t.Fatalf("initialized request = %#v", requests[1])
	}
	if requests[2]["id"] != float64(2) || requests[2]["method"] != "account/rateLimits/read" {
		t.Fatalf("rate limit request = %#v", requests[2])
	}
}
