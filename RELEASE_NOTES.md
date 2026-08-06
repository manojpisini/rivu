# Rivu v1.0.0 Release Notes

Release date: 2026-08-06

## Included

- Complete Go project structure based on the consolidated Rivu specification.
- Cobra CLI with Source/New, Scan, Open, Flow/Move, Doctor, Agent Sync, Stats, Dashboard, List/Index, Config, and TUI commands.
- SQLite-backed registry using pure-Go `modernc.org/sqlite`.
- Bubble Tea and Lip Gloss terminal dashboard.
- Project detection for Go, Rust, Python, Node, Git, and Rivu Banks.
- Protected Bank generation and deterministic agent Map generation.
- Safe dry-run and explicit-confirmation lifecycle moves.
- Unit tests, cross-platform GitHub Actions CI, MIT license, changelog, and documentation.
- Original project specification and visualization/build-plan HTML files.

## Build verification note

The source was formatted successfully with Go 1.23.2. This execution environment had no working DNS access to Go module proxies, so third-party modules could not be downloaded and the final `go test`, `go vet`, and binary build could not run locally. The included CI workflow performs all three checks on Linux, Windows, and macOS once pushed to a networked Git repository.
