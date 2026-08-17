package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ResolveOpenCodeGoAPIKey finds the OpenCode Go API key, preferring an
// explicit environment variable and falling back to OpenCode's own
// credential store (`opencode auth login` writes here).
func ResolveOpenCodeGoAPIKey() (string, error) {
	if v := os.Getenv("OPENCODE_GO_API_KEY"); v != "" {
		return v, nil
	}
	if v := os.Getenv("OPENCODE_API_KEY"); v != "" {
		return v, nil
	}

	path, err := opencodeAuthPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no OPENCODE_GO_API_KEY set and could not read %s: %w", path, err)
	}

	var store map[string]struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	if entry, ok := store["opencode-go"]; ok && entry.Key != "" {
		return entry.Key, nil
	}
	return "", errors.New("no opencode-go credentials found; run `opencode auth login` or set OPENCODE_GO_API_KEY")
}

func opencodeAuthPath() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "opencode", "auth.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json"), nil
}
