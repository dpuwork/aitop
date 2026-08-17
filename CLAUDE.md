# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`aitop` is a terminal dashboard (Bubble Tea TUI) that shows current usage/quota across three AI coding tools — Claude Code, OpenCode Go, and Codex — in one place. It polls each provider's remote account data on its own interval; it does not read local history/logs.

## Commands

```sh
go build ./...        # build
go run .               # run the TUI
go vet ./...           # lint/vet
go test ./...           # test (no tests currently exist)
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
