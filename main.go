// Command aitop is a terminal dashboard showing current usage across
// Claude Code, OpenCode, and Codex from one place.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dpuwork/aitop/internal/provider"
	"github.com/dpuwork/aitop/internal/ui"
)

// pollInterval is how often every provider is polled.
const pollInterval = 120 * time.Second

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println("aitop version " + ui.Version)
		return
	}

	providers := []provider.Provider{
		provider.NewClaude(),
		provider.NewOpenCodeGo(),
		provider.NewCodex(),
	}
	intervals := []time.Duration{pollInterval, pollInterval, pollInterval}

	m := ui.New(providers, intervals)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "aitop:", err)
		os.Exit(1)
	}
}
