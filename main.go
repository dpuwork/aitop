// Command aitop is a terminal dashboard showing current usage across
// Claude Code, OpenCode, and Codex from one place.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dpuwork/aitop/internal/provider"
	"github.com/dpuwork/aitop/internal/ui"
)

// pollInterval is how often every provider is polled.
const pollInterval = 120 * time.Second

// scanTimeout bounds how long the startup provider scan can take before the
// TUI launches, so a single hung CLI process can't block startup forever.
const scanTimeout = 20 * time.Second

const startupLoaderInterval = 120 * time.Millisecond

var startupLoaderFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const startupLoaderLabel = "\033[1;38;5;39m[aitop]\033[0m"

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	allFlag := flag.Bool("all", false, "show every provider, including ones that fail the startup availability scan")
	flag.Parse()

	if *versionFlag {
		fmt.Println("aitop version " + ui.Version)
		return
	}

	candidates := []provider.Provider{
		provider.NewClaude(),
		provider.NewOpenCodeGo(),
		provider.NewCodex(),
	}

	var providers []provider.Provider
	var initial []provider.Snapshot
	if *allFlag {
		providers = candidates
		initial = nil
	} else {
		fmt.Print("\033[2J\033[H")
		providers, initial = scanProvidersWithLoader(candidates)
		if len(providers) == 0 {
			fmt.Fprintln(os.Stderr, "aitop: no providers detected — install and log into claude, opencode, or codex (or run with --all to see why)")
			os.Exit(1)
		}
	}

	intervals := make([]time.Duration, len(providers))
	for i := range intervals {
		intervals[i] = pollInterval
	}

	m := ui.New(providers, intervals, initial)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "aitop:", err)
		os.Exit(1)
	}
}

func scanProvidersWithLoader(candidates []provider.Provider) ([]provider.Provider, []provider.Snapshot) {
	result := make(chan struct {
		providers []provider.Provider
		initial   []provider.Snapshot
	})
	go func() {
		providers, initial := scanProviders(candidates)
		result <- struct {
			providers []provider.Provider
			initial   []provider.Snapshot
		}{providers, initial}
	}()

	loader := 0
	ticker := time.NewTicker(startupLoaderInterval)
	defer ticker.Stop()
	fmt.Printf("%s %s loading...", startupLoaderFrames[loader], startupLoaderLabel)
	for {
		select {
		case result := <-result:
			// Remove the loader before Bubble Tea takes over the terminal.
			fmt.Printf("\r%s\r", strings.Repeat(" ", 32))
			return result.providers, result.initial
		case <-ticker.C:
			loader = (loader + 1) % len(startupLoaderFrames)
			fmt.Printf("\r%s %s loading...", startupLoaderFrames[loader], startupLoaderLabel)
		}
	}
}

// scanProviders probes every candidate provider once, concurrently, before
// the TUI starts. Providers that fail with provider.ErrUnavailable (CLI
// binary missing, or no credentials found) are dropped entirely, so the
// dashboard only shows panels for tools actually installed and logged into
// on this machine. Any other failure (network, a logged-in CLI/API
// returning an error) still keeps its panel, exactly as it would once
// polled during normal operation — only "not set up" is filtered.
func scanProviders(candidates []provider.Provider) ([]provider.Provider, []provider.Snapshot) {
	snaps := make([]provider.Snapshot, len(candidates))

	var wg sync.WaitGroup
	for i, p := range candidates {
		wg.Add(1)
		go func(i int, p provider.Provider) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
			defer cancel()
			snaps[i] = p.Poll(ctx)
		}(i, p)
	}
	wg.Wait()

	var providers []provider.Provider
	var kept []provider.Snapshot
	for i, p := range candidates {
		if snaps[i].Status == provider.StatusError && provider.IsUnavailable(snaps[i].Err) {
			continue
		}
		providers = append(providers, p)
		kept = append(kept, snaps[i])
	}
	return providers, kept
}
