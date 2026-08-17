package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const openCodeGoUsageURL = "https://opencode.ai/zen/go/v1/usage"

// windowOrder controls display order; the API returns a map so ordering
// isn't guaranteed.
var windowOrder = []string{"rolling", "weekly", "monthly"}

// OpenCodeGo polls OpenCode Go's server-side quota endpoint, the
// authoritative source for rolling/weekly/monthly usage.
type OpenCodeGo struct {
	ResolveAPIKey func() (string, error)
	URL           string
	Client        *http.Client
}

func NewOpenCodeGo() *OpenCodeGo {
	return &OpenCodeGo{
		ResolveAPIKey: ResolveOpenCodeGoAPIKey,
		URL:           openCodeGoUsageURL,
		Client:        &http.Client{Timeout: 10 * time.Second},
	}
}

func (o *OpenCodeGo) Name() string { return "OpenCode Go" }

func (o *OpenCodeGo) Poll(ctx context.Context) Snapshot {
	snap := Snapshot{Provider: o.Name(), UpdatedAt: time.Now()}

	// Re-resolved on every poll rather than cached at construction time,
	// so a credential that wasn't available yet at startup (e.g. auth.json
	// written after aitop launched) is picked up on the next refresh
	// instead of the provider being stuck reporting the original error.
	apiKey, err := o.ResolveAPIKey()
	if err != nil {
		snap.Status = StatusError
		snap.Err = err
		return snap
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.URL, nil)
	if err != nil {
		snap.Status = StatusError
		snap.Err = err
		return snap
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := o.Client.Do(req)
	if err != nil {
		snap.Status = StatusError
		snap.Err = err
		return snap
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		snap.Status = StatusError
		snap.Err = err
		return snap
	}
	if resp.StatusCode != http.StatusOK {
		snap.Status = StatusError
		snap.Err = fmt.Errorf("http %d", resp.StatusCode)
		return snap
	}

	var payload struct {
		Usage map[string]struct {
			Status   string    `json:"status"`
			Percent  int       `json:"percent"`
			ResetsAt time.Time `json:"resetsAt"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		snap.Status = StatusError
		snap.Err = fmt.Errorf("parse response: %w", err)
		return snap
	}

	for _, name := range windowOrder {
		w, ok := payload.Usage[name]
		if !ok {
			continue
		}
		snap.Windows = append(snap.Windows, Window{
			Name:     name,
			Percent:  w.Percent,
			ResetsAt: w.ResetsAt,
			HasReset: !w.ResetsAt.IsZero(),
		})
		if w.Status != "" && w.Status != "ok" {
			snap.Summary += fmt.Sprintf("%s: %s  ", name, w.Status)
		}
	}

	snap.Status = StatusOK
	return snap
}
