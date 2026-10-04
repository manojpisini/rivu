# Rivu

**Rivu — your project filesystem, mapped and flowing.**

Rivu is a cross-platform Go CLI and Bubble Tea TUI for creating, discovering, indexing, opening, auditing, and agent-mapping structured project folders.

## v1.0.0 capabilities

- SQLite project registry and Current-project context
- Workspace scanning with Go, Rust, Python, Node, Git, and Rivu marker detection
- Safe project Source workflow with dry runs
- Flow transitions across `source`, `active`, `maintenance`, `research`, and `delta`
- Protected `.metadata/` Bank generation
- Deterministic agent Map generation
- Health/Doctor scoring and portfolio stats
- Headless CLI plus a responsive Bubble Tea dashboard
- Cross-platform, pure-Go SQLite builds

## Install

```bash
go install github.com/manojpisini/rivu/cmd/rivu@latest
```

Or build locally:

```bash
go build -o rivu ./cmd/rivu
```

## Start

```bash
rivu config show
rivu scan
rivu list
rivu tui
```

The first run creates `~/.rivu/config.toml`. Set `[workspace].root` to your project workspace before scanning.

## Essential commands

```bash
rivu source "My Project" --flow active --git
rivu source "Preview" --dry-run
rivu open my-project
rivu flow my-project --to maintenance --dry-run
rivu flow my-project --to maintenance --yes
rivu doctor my-project
rivu agent sync my-project
rivu stats
rivu dashboard
```

Filesystem-moving commands require `--yes` unless run with `--dry-run`. Rivu never permanently deletes project folders.

## Development

```bash
go test ./...
go vet ./...
go build ./cmd/rivu
```

See `docs/rivu_project_spec.md` for the complete product, production, and architecture specification.
