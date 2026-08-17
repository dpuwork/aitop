package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Claude polls usage via the `claude` CLI's non-interactive /usage slash
// command. The command returns a stable JSON envelope, but the usage
// summary itself is human-readable text, so it is parsed defensively:
// anything that fails to match is simply omitted rather than treated as a
// poll failure.
type Claude struct {
	Timeout time.Duration
}

func NewClaude() *Claude {
	return &Claude{Timeout: 20 * time.Second}
}

func (c *Claude) Name() string { return "Claude Code" }

func (c *Claude) Poll(ctx context.Context) Snapshot {
	snap := Snapshot{Provider: c.Name(), UpdatedAt: time.Now()}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", "/usage", "--output-format", "json")
	out, err := cmd.Output()
	if err != nil {
		snap.Status = StatusError
		if errors.Is(err, exec.ErrNotFound) {
			snap.Err = fmt.Errorf("%w: claude CLI not found in PATH", ErrUnavailable)
		} else {
			snap.Err = describeExecErr(err)
		}
		return snap
	}

	var resp struct {
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		snap.Status = StatusError
		snap.Err = fmt.Errorf("parse claude output: %w", err)
		return snap
	}
	if resp.IsError {
		snap.Status = StatusError
		snap.Err = fmt.Errorf("claude reported an error")
		return snap
	}

	parseClaudeUsage(&snap, resp.Result)
	snap.Status = StatusOK
	return snap
}

var claudeSessionRe = regexp.MustCompile(`Current session:\s*(\d+)%\s*used(?:\s*·\s*resets\s*([^\n]+))?`)

// parseClaudeUsage extracts the current-session percentage/reset time out
// of the freeform summary text. The format is not a documented contract,
// so a failed match degrades to showing the raw first line rather than an
// error.
func parseClaudeUsage(snap *Snapshot, text string) {
	if m := claudeSessionRe.FindStringSubmatch(text); m != nil {
		percent, _ := strconv.Atoi(m[1])
		w := Window{Name: "session", Percent: percent}
		if reset := strings.TrimSpace(m[2]); reset != "" {
			if t, ok := parseClaudeResetTime(reset); ok {
				w.ResetsAt = t
				w.HasReset = true
			}
		}
		snap.Windows = append(snap.Windows, w)
	}

	if len(snap.Windows) == 0 {
		if line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0]); line != "" {
			snap.Summary = line
		}
	}
}

// claudeResetTZRe splits a trailing "(TZ)" annotation off a reset string,
// e.g. "Aug 17 at 2:49pm (Asia/Jerusalem)" -> date part + "Asia/Jerusalem".
// The CLI has been observed to emit both "(UTC)" and full IANA zone names
// depending on the user's local timezone.
var claudeResetTZRe = regexp.MustCompile(`^(.*?)\s*\(([^()]+)\)\s*$`)

// parseClaudeResetTime parses strings like "Aug 17, 11:49am (UTC)" or
// "Aug 17 at 2:49pm (Asia/Jerusalem)" — the CLI varies both the
// date/time separator ("," vs " at ") and the timezone annotation by
// locale. The year is not included, so the current year is assumed and
// rolled forward a year if that puts the reset implausibly far in the
// past (e.g. parsing a December reset in early January).
func parseClaudeResetTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)

	loc := time.UTC
	datePart := s
	if m := claudeResetTZRe.FindStringSubmatch(s); m != nil {
		datePart = strings.TrimSpace(m[1])
		if tzName := m[2]; tzName != "UTC" {
			if l, err := time.LoadLocation(tzName); err == nil {
				loc = l
			}
		}
	}
	datePart = strings.Replace(datePart, " at ", ", ", 1)

	now := time.Now().In(loc)
	candidate := fmt.Sprintf("%d %s", now.Year(), datePart)
	t, err := time.ParseInLocation("2006 Jan 2, 3:04pm", candidate, loc)
	if err != nil {
		return time.Time{}, false
	}
	if t.Before(now.Add(-48 * time.Hour)) {
		t = t.AddDate(1, 0, 0)
	}
	return t, true
}
