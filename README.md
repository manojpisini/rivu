# Rivu

**Your project filesystem, mapped and flowing.**

[![CI](https://github.com/manojpisini/rivu/actions/workflows/ci.yml/badge.svg)](https://github.com/manojpisini/rivu/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8.svg)](https://go.dev/dl/)

Rivu is a cross-platform Go CLI and Bubble Tea TUI for creating, discovering, indexing, opening, auditing, and agent-mapping structured project folders.

## Features

- SQLite project registry (pure Go, no CGO) with Current-project context
- Workspace scanning with Go, Rust, Python, Node, Git, and Rivu Bank marker detection
- Safe Source workflow with dry runs — nothing is written without a preview
- Flow transitions across `source`, `active`, `maintenance`, `research`, and `delta`
- Protected `.metadata/` Bank generation for every project
- Deterministic agent Map generation (`PROJECT_MAP.md`, `AGENTS.md`)
- Doctor health scoring and portfolio stats
- Headless CLI plus a responsive three-panel Bubble Tea explorer
- Windows, macOS, and Linux builds from one codebase

## Install

Build from source (requires [Go 1.23+](https://go.dev/dl/)):

```bash
git clone https://github.com/manojpisini/rivu.git
cd rivu
go build -o rivu ./cmd/rivu
```

Or, once a release tag is published:

```bash
go install github.com/manojpisini/rivu/cmd/rivu@latest
```

## Start

```bash
rivu config show
rivu scan
rivu list
rivu tui
```

The first run creates `~/.rivu/config.toml` with defaults. Set `[workspace].root` to your project workspace before scanning.

Set `RIVU_HOME` to relocate Rivu's home (config + database) and `RIVU_CONFIG` to point at a specific config file — useful for isolated testing or per-project setups. Global flags `--home` and `--config` do the same per invocation.

## Essential commands

```bash
rivu source "My Project" --flow active --git   # create a structured project
rivu source "Preview" --dry-run                # preview only, writes nothing
rivu open my-project                           # open Current or named project
rivu flow my-project --to maintenance --yes    # move between Flow stages
rivu flow my-project --to maintenance --dry-run
rivu doctor my-project                         # health checks with remedies
rivu agent sync my-project                     # build the agent Map
rivu stats                                     # portfolio metrics
rivu dashboard                                 # dashboard snapshot
```

## Safety

- **Nothing in Rivu deletes a project folder — ever.**
- Moving commands require `--yes`, or preview with `--dry-run`; without confirmation nothing is written.
- Bank files (`overview.md`, `decisions.md`, `tasks.md`, `project.toml`) are create-if-missing; Rivu never silently overwrites existing content.
- Secret files (`.env*`, `*.pem`, `*.key`) are listed by name in the Map — their contents are never read or printed.

## Development

```bash
go test ./...
go vet ./...
gofmt -l .        # must print nothing
go build ./cmd/rivu
```

Vocabulary used throughout Rivu: Flow, Source, Channel, Current, Map, Bank, Delta, Confluence.

## License

MIT — see [LICENSE](LICENSE).
