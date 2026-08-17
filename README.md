<div align="center">

<picture>
  <source media="(prefers-color-scheme: light)" srcset="/assets/aitop-light.png">
  <img alt="aitop screenshot" src="/assets/aitop-dark.png" width="75%" height="75%">
</picture>

<br/>

aitop: terminal dashboard showing current usage/quota across Claude Code, OpenCode Go, and Codex in one place.
</div>

---

## install

```bash
brew install dpuwork/tap/aitop
```

Or via [mise](https://mise.jdx.dev/):

```bash
mise use -g github:dpuwork/aitop
```

Or via Go:

```bash
go install github.com/dpuwork/aitop@latest
```

Or build from source:

```bash
go build -o aitop .
```

## usage

```bash
aitop
```

Each provider polls its own remote account data on a fixed interval — it does not read local history or logs.

### shortcuts

| Hotkey  | Function                             |
| ------- | ------------------------------------- |
| `r`     | Force-refresh all providers now       |
| `d`     | Toggle relative / absolute reset dates |
| `q`     | Quit (also `ctrl+c`, `esc`)           |
