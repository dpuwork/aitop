package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Codex polls usage via the `codex` CLI's app-server JSON-RPC protocol
// (`codex app-server`, method `account/rateLimits/read`). This is the same
// account-level rate limit data the Codex TUI and IDE integrations show;
// there is no plain usage subcommand. Authentication is whatever the
// installed `codex` CLI is already logged in with (ChatGPT OAuth or API
// key) — this provider never touches credentials directly.
type Codex struct {
	Timeout time.Duration
}

func NewCodex() *Codex {
	return &Codex{Timeout: 20 * time.Second}
}

func (c *Codex) Name() string { return "Codex" }

func (c *Codex) Poll(ctx context.Context) Snapshot {
	snap := Snapshot{Provider: c.Name(), UpdatedAt: time.Now()}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	raw, err := queryCodexRateLimits(ctx)
	if err != nil {
		snap.Status = StatusError
		snap.Err = err
		return snap
	}

	if err := parseCodexRateLimits(&snap, raw); err != nil {
		snap.Status = StatusError
		snap.Err = err
		return snap
	}

	snap.Status = StatusOK
	return snap
}

type jsonrpcMsg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// queryCodexRateLimits spawns a short-lived `codex app-server` process,
// performs the minimal initialize handshake over its newline-delimited
// JSON-RPC stdio protocol, and requests account/rateLimits/read.
func queryCodexRateLimits(ctx context.Context) (json.RawMessage, error) {
	cmd := exec.CommandContext(ctx, "codex", "app-server")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%w: codex CLI not found in PATH", ErrUnavailable)
		}
		return nil, describeExecErr(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	send := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = stdin.Write(append(b, '\n'))
		return err
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	// Reads lines until one is a response (has a matching id); notifications
	// and unrelated messages are skipped.
	waitForID := func(id int) (jsonrpcMsg, error) {
		for scanner.Scan() {
			var msg jsonrpcMsg
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil || msg.ID == nil {
				continue
			}
			var gotID int
			if err := json.Unmarshal(msg.ID, &gotID); err != nil || gotID != id {
				continue
			}
			if msg.Error != nil {
				// Observed verbatim from a logged-out `codex app-server` responding
				// to account/rateLimits/read: {"error":{"code":-32600,"message":
				// "codex account authentication required to read rate limits"}}.
				if strings.Contains(msg.Error.Message, "authentication required") {
					return jsonrpcMsg{}, fmt.Errorf("%w: %s", ErrUnavailable, msg.Error.Message)
				}
				return jsonrpcMsg{}, fmt.Errorf("codex app-server: %s", msg.Error.Message)
			}
			return msg, nil
		}
		if err := scanner.Err(); err != nil {
			return jsonrpcMsg{}, err
		}
		if ctx.Err() != nil {
			return jsonrpcMsg{}, ctx.Err()
		}
		return jsonrpcMsg{}, io.ErrUnexpectedEOF
	}

	if err := send(map[string]any{
		"id":     1,
		"method": "initialize",
		"params": map[string]any{
			"clientInfo": map[string]any{"name": "aitop", "version": "0.1.0"},
		},
	}); err != nil {
		return nil, err
	}
	if _, err := waitForID(1); err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	if err := send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return nil, err
	}

	if err := send(map[string]any{"id": 2, "method": "account/rateLimits/read", "params": map[string]any{}}); err != nil {
		return nil, err
	}
	resp, err := waitForID(2)
	if err != nil {
		return nil, fmt.Errorf("account/rateLimits/read: %w", err)
	}
	return resp.Result, nil
}

type codexRateLimitWindow struct {
	UsedPercent        int    `json:"usedPercent"`
	ResetsAt           *int64 `json:"resetsAt"`
	WindowDurationMins *int64 `json:"windowDurationMins"`
}

type codexRateLimitsResult struct {
	RateLimits struct {
		PlanType  *string               `json:"planType"`
		Primary   *codexRateLimitWindow `json:"primary"`
		Secondary *codexRateLimitWindow `json:"secondary"`
	} `json:"rateLimits"`
}

func parseCodexRateLimits(snap *Snapshot, raw json.RawMessage) error {
	var payload codexRateLimitsResult
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("parse rate limits: %w", err)
	}

	if p := payload.RateLimits.PlanType; p != nil && *p != "" && *p != "unknown" {
		snap.Summary = "plan: " + *p
	}

	addWindow := func(w *codexRateLimitWindow) {
		if w == nil {
			return
		}
		win := Window{Name: codexWindowLabel(w), Percent: w.UsedPercent}
		if w.ResetsAt != nil {
			win.ResetsAt = time.Unix(*w.ResetsAt, 0)
			win.HasReset = true
		}
		snap.Windows = append(snap.Windows, win)
	}
	addWindow(payload.RateLimits.Primary)
	addWindow(payload.RateLimits.Secondary)

	return nil
}

// codexWindowLabel turns a window's duration into a short label like "5h"
// or "weekly" since the API identifies windows by duration, not by name.
func codexWindowLabel(w *codexRateLimitWindow) string {
	if w.WindowDurationMins == nil || *w.WindowDurationMins <= 0 {
		return "window"
	}
	mins := *w.WindowDurationMins
	switch {
	case mins%(60*24*7) == 0:
		if weeks := mins / (60 * 24 * 7); weeks == 1 {
			return "weekly"
		} else {
			return fmt.Sprintf("%dw", weeks)
		}
	case mins%(60*24) == 0:
		return fmt.Sprintf("%dd", mins/(60*24))
	case mins%60 == 0:
		return fmt.Sprintf("%dh", mins/60)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}
