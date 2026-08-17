package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveOpenCodeGoAPIKeyEnvironmentPrecedence(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("OPENCODE_API_KEY", "generic-key")
	t.Setenv("OPENCODE_GO_API_KEY", "go-key")
	if got, err := ResolveOpenCodeGoAPIKey(); err != nil || got != "go-key" {
		t.Fatalf("key, err = %q, %v; want go-key, nil", got, err)
	}

	t.Setenv("OPENCODE_GO_API_KEY", "")
	if got, err := ResolveOpenCodeGoAPIKey(); err != nil || got != "generic-key" {
		t.Fatalf("key, err = %q, %v; want generic-key, nil", got, err)
	}
}

func TestResolveOpenCodeGoAPIKeyAuthJSON(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
		wantErr  string
	}{
		{name: "valid entry", contents: `{"opencode-go":{"type":"api","key":"auth-key"}}`, want: "auth-key"},
		{name: "other entries ignored", contents: `{"openai":{"key":"wrong"},"opencode":{"key":"also-wrong"}}`, wantErr: "no opencode-go credentials found"},
		{name: "empty key", contents: `{"opencode-go":{"type":"api","key":""}}`, wantErr: "no opencode-go credentials found"},
		{name: "malformed JSON", contents: `{"opencode-go":`, wantErr: "parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_DATA_HOME", dir)
			if err := os.Mkdir(filepath.Join(dir, "opencode"), 0o755); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "opencode", "auth.json")
			if err := os.WriteFile(path, []byte(tt.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("OPENCODE_GO_API_KEY", "")
			t.Setenv("OPENCODE_API_KEY", "")

			got, err := ResolveOpenCodeGoAPIKey()
			if got != tt.want {
				t.Errorf("key = %q, want %q", got, tt.want)
			}
			if tt.wantErr == "" && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Errorf("err = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveOpenCodeGoAPIKeyMissingAuthJSON(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("OPENCODE_GO_API_KEY", "")
	t.Setenv("OPENCODE_API_KEY", "")
	_, err := ResolveOpenCodeGoAPIKey()
	if err == nil || !strings.Contains(err.Error(), "could not read") {
		t.Fatalf("err = %v, want missing auth.json error", err)
	}
}

func TestResolveOpenCodeGoAPIKeyAuthPathUsesXDGDataHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	got, err := opencodeAuthPath()
	want := filepath.Join(dir, "opencode", "auth.json")
	if err != nil || got != want {
		t.Fatalf("path, err = %q, %v; want %q, nil", got, err, want)
	}
}

func TestResolveOpenCodeGoAPIKeyDoesNotTreatWhitespaceAsKey(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("OPENCODE_GO_API_KEY", "   ")
	if got, err := ResolveOpenCodeGoAPIKey(); err != nil || got != "   " {
		t.Fatalf("key, err = %q, %v; want whitespace env value to be returned verbatim", got, err)
	}
}
