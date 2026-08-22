package provider

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseClaudeUsage(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		wantPercent   int
		wantSummary   string
		wantReset     bool
		wantHour      int
		wantMinute    int
		wantWindowLen int
	}{
		{
			name:          "session usage with UTC reset",
			text:          "Current session: 42% used · resets Aug 17, 11:49am (UTC)",
			wantPercent:   42,
			wantReset:     true,
			wantHour:      11,
			wantMinute:    49,
			wantWindowLen: 1,
		},
		{
			name:          "session usage with named timezone",
			text:          "Current session: 7% used · resets Aug 17 at 2:49pm (Asia/Jerusalem)",
			wantPercent:   7,
			wantReset:     true,
			wantHour:      14,
			wantMinute:    49,
			wantWindowLen: 1,
		},
		{
			name:          "session usage without reset",
			text:          "Current session: 0% used",
			wantPercent:   0,
			wantWindowLen: 1,
		},
		{
			name:        "unrecognized text uses first line as summary",
			text:        "Usage is temporarily unavailable\nTry again later",
			wantSummary: "Usage is temporarily unavailable",
		},
		{
			name: "empty text remains empty",
		},
		{
			name:          "invalid reset is omitted but usage is retained",
			text:          "Current session: 18% used · resets sometime later",
			wantPercent:   18,
			wantWindowLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := Snapshot{}
			parseClaudeUsage(&snap, tt.text)

			if len(snap.Windows) != tt.wantWindowLen {
				t.Fatalf("windows = %#v, want %d windows", snap.Windows, tt.wantWindowLen)
			}
			if tt.wantWindowLen == 1 {
				window := snap.Windows[0]
				if window.Name != "session" || window.Percent != tt.wantPercent {
					t.Errorf("window = %#v, want session at %d%%", window, tt.wantPercent)
				}
				if window.HasReset != tt.wantReset {
					t.Errorf("HasReset = %v, want %v", window.HasReset, tt.wantReset)
				}
				if tt.wantReset {
					if window.ResetsAt.Hour() != tt.wantHour || window.ResetsAt.Minute() != tt.wantMinute {
						t.Errorf("reset time = %v, want %02d:%02d", window.ResetsAt, tt.wantHour, tt.wantMinute)
					}
				}
			}
			if snap.Summary != tt.wantSummary {
				t.Errorf("summary = %q, want %q", snap.Summary, tt.wantSummary)
			}
		})
	}
}

func TestParseClaudeUsageWithWeeklyWindow(t *testing.T) {
	text := "You are currently using your subscription to power your Claude Code usage\n\n" +
		"Current session: 92% used · resets Aug 22, 4pm (UTC)\n" +
		"Current week (all models): 51% used · resets Aug 26, 11:59am (UTC)\n\n" +
		"What's contributing to your limits usage?"

	snap := Snapshot{}
	parseClaudeUsage(&snap, text)

	if len(snap.Windows) != 2 {
		t.Fatalf("windows = %#v, want 2 windows", snap.Windows)
	}

	session, week := snap.Windows[0], snap.Windows[1]
	if session.Name != "session" || session.Percent != 92 || !session.HasReset {
		t.Errorf("session window = %#v, want session at 92%% with reset", session)
	}
	if week.Name != "week" || week.Percent != 51 || !week.HasReset {
		t.Errorf("week window = %#v, want week at 51%% with reset", week)
	}
	if week.ResetsAt.Hour() != 11 || week.ResetsAt.Minute() != 59 {
		t.Errorf("week reset time = %v, want 11:59", week.ResetsAt)
	}
	if snap.Summary != "" {
		t.Errorf("summary = %q, want empty summary when windows are parsed", snap.Summary)
	}
}

func TestParseClaudeUsageWithModelSpecificWeeklyWindow(t *testing.T) {
	text := "Current session: 10% used · resets Aug 22, 4pm (UTC)\n" +
		"Current week (all models): 20% used · resets Aug 26, 11:59am (UTC)\n" +
		"Current week (Opus): 30% used · resets Aug 26, 11:59am (UTC)"

	snap := Snapshot{}
	parseClaudeUsage(&snap, text)

	if len(snap.Windows) != 3 {
		t.Fatalf("windows = %#v, want 3 windows", snap.Windows)
	}
	if snap.Windows[1].Name != "week" {
		t.Errorf("all-models window name = %q, want %q", snap.Windows[1].Name, "week")
	}
	if snap.Windows[2].Name != "week (Opus)" || snap.Windows[2].Percent != 30 {
		t.Errorf("Opus window = %#v, want %q at 30%%", snap.Windows[2], "week (Opus)")
	}
}

func TestParseClaudeUsagePreservesExistingWindows(t *testing.T) {
	snap := Snapshot{Windows: []Window{{Name: "existing"}}}
	parseClaudeUsage(&snap, "not a Claude usage response")

	if len(snap.Windows) != 1 || snap.Windows[0].Name != "existing" {
		t.Fatalf("windows = %#v, want existing window preserved", snap.Windows)
	}
	if snap.Summary != "" {
		t.Errorf("summary = %q, want empty summary", snap.Summary)
	}
}

func TestParseClaudeResetTime(t *testing.T) {
	tests := []struct {
		name string
		text string
		ok   bool
	}{
		{name: "comma and UTC", text: "Aug 17, 11:49am (UTC)", ok: true},
		{name: "at and timezone", text: "Aug 17 at 2:49pm (Asia/Jerusalem)", ok: true},
		{name: "on the hour with no minutes", text: "Aug 22, 4pm (UTC)", ok: true},
		{name: "invalid date", text: "not a date", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseClaudeResetTime(tt.text)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (time %v)", ok, tt.ok, got)
			}
			if tt.ok && got.IsZero() {
				t.Error("successful parse returned zero time")
			}
		})
	}
}

func TestClaudePoll(t *testing.T) {
	claudeDir := t.TempDir()
	installFakeClaude(t, claudeDir)
	t.Setenv("PATH", claudeDir)

	tests := []struct {
		name       string
		output     string
		exitCode   string
		wantStatus Status
		wantErr    string
		wantWindow int
		wantPct    int
	}{
		{
			name:       "successful response",
			output:     `{"is_error":false,"result":"Current session: 63% used"}`,
			wantStatus: StatusOK,
			wantWindow: 1,
			wantPct:    63,
		},
		{
			name:       "successful response with no recognized usage",
			output:     `{"is_error":false,"result":"Claude usage is unavailable"}`,
			wantStatus: StatusOK,
		},
		{
			name:       "malformed JSON",
			output:     `not json`,
			wantStatus: StatusError,
			wantErr:    "parse claude output",
		},
		{
			name:       "reported error",
			output:     `{"is_error":true,"result":"authentication failed"}`,
			wantStatus: StatusError,
			wantErr:    "claude reported an error",
		},
		{
			name:       "command failure",
			output:     `ignored`,
			exitCode:   "7",
			wantStatus: StatusError,
			wantErr:    "exit status 7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CLAUDE_TEST_OUTPUT", tt.output)
			t.Setenv("CLAUDE_TEST_EXIT", tt.exitCode)

			snap := (&Claude{Timeout: time.Second}).Poll(context.Background())
			if snap.Provider != "Claude Code" {
				t.Errorf("provider = %q, want Claude Code", snap.Provider)
			}
			if snap.UpdatedAt.IsZero() {
				t.Error("UpdatedAt is zero")
			}
			if snap.Status != tt.wantStatus {
				t.Errorf("status = %v, want %v", snap.Status, tt.wantStatus)
			}
			if len(snap.Windows) != tt.wantWindow {
				t.Fatalf("windows = %#v, want %d", snap.Windows, tt.wantWindow)
			}
			if tt.wantWindow == 1 && snap.Windows[0].Percent != tt.wantPct {
				t.Errorf("percent = %d, want %d", snap.Windows[0].Percent, tt.wantPct)
			}
			if tt.wantErr != "" && (snap.Err == nil || !strings.Contains(snap.Err.Error(), tt.wantErr)) {
				t.Errorf("error = %v, want substring %q", snap.Err, tt.wantErr)
			}
			if tt.wantErr == "" && snap.Err != nil {
				t.Errorf("error = %v, want nil", snap.Err)
			}
		})
	}
}

func TestClaudePollMissingCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	snap := (&Claude{Timeout: time.Second}).Poll(context.Background())
	if snap.Status != StatusError {
		t.Fatalf("status = %v, want %v", snap.Status, StatusError)
	}
	if !errors.Is(snap.Err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", snap.Err)
	}
}

func installFakeClaude(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nif [ -n \"$CLAUDE_TEST_EXIT\" ]; then exit \"$CLAUDE_TEST_EXIT\"; fi\nprintf '%s' \"$CLAUDE_TEST_OUTPUT\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
}
