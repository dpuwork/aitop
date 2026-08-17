# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`aitop` is a terminal dashboard (Bubble Tea TUI) that shows current usage/quota across three AI coding tools — Claude Code, OpenCode Go, and Codex — in one place. It polls each provider's remote account data on its own interval; it does not read local history/logs.

## Commands

```sh
go build ./...        # build
go run .               # run the TUI
go vet ./...           # lint/vet
go test ./...           # test
```

Go toolchain version is pinned via `mise.toml` (`go = "1.26.6"`); use `mise install` if the pinned version isn't available locally.

## Architecture

Two packages, cleanly separated:

- **`internal/provider`** — defines the `Provider` interface (`Name() string`, `Poll(ctx) Snapshot`) and one implementation per data source. Each provider is independent and pluggable; `main.go` wires up the concrete list.
  - `claude.go` — shells out to `claude -p /usage --output-format json`. The JSON envelope is stable but the `result` field is human-readable prose, so it's parsed defensively with regexes (`parseClaudeUsage`); anything that fails to match is simply omitted rather than surfaced as an error.
  - `codex.go` — spawns `codex app-server` and speaks its newline-delimited JSON-RPC protocol directly over stdin/stdout (`initialize` → `initialized` → `account/rateLimits/read`). There is no plain usage subcommand for Codex, so this is the only way to get the same rate-limit data the Codex TUI shows.
  - `opencode_go.go` — hits OpenCode Go's HTTP quota endpoint (`opencode.ai/zen/go/v1/usage`) directly, the authoritative server-side source for rolling/weekly/monthly usage. Requires an API key, resolved by `credentials.go` (env var first, then OpenCode's own `auth.json` credential store).
  - `types.go` — shared `Snapshot`/`Window`/`Metric` model every provider reports into. `Window` is a percent-used quota with an optional reset time; `Metric` is a freeform label/value pair for data that doesn't fit the window shape.
  - Providers never fail the whole poll for partial/unparsable data — a `Snapshot` degrades gracefully (empty windows/metrics, a raw summary line) rather than reporting `StatusError`, except for actual transport/exec/auth failures.

- **`internal/ui`** — a single Bubble Tea `Model` (`model.go`) driving one bordered panel per provider (`view.go` + `styles.go`). Each provider polls and reschedules itself independently (`pollMsg`/`tickMsg` carry a provider index), so one slow or failing provider never blocks the others' display or refresh cadence. Keybinds: `q`/`ctrl+c`/`esc` to quit, `r` to force-refresh all providers immediately.

Adding a new provider means: implement `provider.Provider` in a new file under `internal/provider`, then add it (with its poll interval) to the slice in `main.go`.

## Regenerating the README screenshots

`assets/aitop-dark.png` / `assets/aitop-light.png` start life as SVG, rendered with [`termframe`](https://github.com/pamburus/termframe) — not `freeze` (charmbracelet), since `freeze -x` does naive raw-byte capture and doesn't interpret cursor movement/alt-screen escape codes, garbling Bubble Tea's full-screen redraws into a mangled image. `termframe` runs a real terminal emulator, so it handles the alt screen correctly. The SVG is then rasterized to PNG with `resvg` and discarded — PNG is what's actually committed, because GitHub (and most Markdown renderers) rasterize embedded SVGs on every view, which is noticeably slower than a PNG.

1. Install termframe and resvg (global mise tools, already in `~/.config/mise/config.toml` as `github:pamburus/termframe` and `aqua:linebender/resvg`):
   ```sh
   mise use -g github:pamburus/termframe
   mise use -g aqua:linebender/resvg@latest
   ```

2. Add a temporary `cmd/screenshot/main.go` *inside* the module (it must live under `github.com/dpuwork/aitop/...` to be allowed to import `internal/ui` and `internal/provider`). It builds three fake `provider.Provider`s returning canned `Snapshot`s (representative windows/percentages — do not capture real account data, since that would leak actual usage numbers into a public screenshot) and runs the normal `tea.Program` with no auto-quit — `termframe` owns the timeout:
   ```go
   package main

   import (
   	"context"
   	"time"

   	tea "github.com/charmbracelet/bubbletea"

   	"github.com/dpuwork/aitop/internal/provider"
   	"github.com/dpuwork/aitop/internal/ui"
   )

   type fakeProvider struct {
   	name string
   	snap provider.Snapshot
   }

   func (f fakeProvider) Name() string                               { return f.name }
   func (f fakeProvider) Poll(ctx context.Context) provider.Snapshot { return f.snap }

   func main() {
   	// ...construct claude/opencode/codex fakeProviders with canned Snapshots...
   	providers := []provider.Provider{claude, opencode, codex}
   	intervals := []time.Duration{time.Hour, time.Hour, time.Hour}

   	m := ui.New(providers, intervals)
   	p := tea.NewProgram(m, tea.WithAltScreen())
   	if _, err := p.Run(); err != nil {
   		panic(err)
   	}
   }
   ```

3. Build it and render both themes to SVG in a scratch location:
   ```sh
   go build -o /tmp/aitop-demo ./cmd/screenshot

   termframe --theme git-hub-dark-default  --mode dark  --title aitop --window-shadow=false -W auto -H auto --timeout 3 -o /tmp/aitop-dark.svg  -- /tmp/aitop-demo
   termframe --theme git-hub-light-default --mode light --title aitop --window-shadow=false -W auto -H auto --timeout 3 -o /tmp/aitop-light.svg -- /tmp/aitop-demo
   ```
   `-W auto -H auto` crops the frame tightly to content instead of leaving blank space. `--timeout 3` is enough for the fake providers (no real polling) to render once.

4. Rasterize to the committed PNGs. `resvg`'s default generic-font mapping expects Windows font names (`Arial`, `Times New Roman`, `Courier New`) and silently drops text if those aren't installed — pass explicit fallbacks:
   ```sh
   resvg --sans-serif-family "DejaVu Sans" --monospace-family "DejaVu Sans Mono" /tmp/aitop-dark.svg  assets/aitop-dark.png
   resvg --sans-serif-family "DejaVu Sans" --monospace-family "DejaVu Sans Mono" /tmp/aitop-light.svg assets/aitop-light.png
   ```

5. Delete `cmd/screenshot/` and the scratch SVGs afterward — the generator is a one-off, not part of the shipped product, and only the PNGs are committed.
