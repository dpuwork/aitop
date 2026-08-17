package provider

import (
	"context"
	"encoding/json"
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
		snap.Err = describeExecErr(err)
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

// parseClaudeResetTime parses strings like "Aug 17, 11:49am (UTC)". The
// year is not included, so the current year is assumed and rolled forward
// a year if that puts the reset implausibly far in the past (e.g. parsing
// a December reset in early January).
func parseClaudeResetTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	loc := time.UTC
	if strings.HasSuffix(s, "(UTC)") {
		s = strings.TrimSpace(strings.TrimSuffix(s, "(UTC)"))
	}

	now := time.Now().UTC()
	candidate := fmt.Sprintf("%d %s", now.Year(), s)
	t, err := time.ParseInLocation("2006 Jan 2, 3:04pm", candidate, loc)
	if err != nil {
		return time.Time{}, false
	}
	if t.Before(now.Add(-48 * time.Hour)) {
		t = t.AddDate(1, 0, 0)
	}
	return t, true
}
