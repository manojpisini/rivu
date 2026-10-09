# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- SQLite registry (pure-Go driver) with `PRAGMA user_version` migrations
  and file backup before migrating.
- Project discovery: `rivu scan` and `rivu index` reconcile the
  filesystem with the registry without clobbering registry-owned fields
  (name, slug, Flow stage, health, created_at).
- Duplicate project names in different directories, with deterministic
  numeric slug disambiguation (`admin`, `admin-2`, …) and stable
  identities across rescans.
- Lifecycle commands: `rivu source`, `rivu flow`, `rivu delta` — every
  mutation is Plan → confirm → Apply with `--dry-run` previews, `--yes`
  to apply and exit 4 for non-TTY runs without `--yes`.
- `rivu doctor`: weighted health checks (README, git, Bank, Map, tests,
  CI, licence, deps), remedies, `--min-score` (exit 5) and `--fix`.
- Bank and Map generation: protected `.metadata/` files
  (create-if-missing for human docs, parse-and-rewrite for machine
  files) and an agent-readable `PROJECT_MAP.md` with ranked inventory,
  entrypoints, scripts and safe commands — secret names only, contents
  never read.
- `rivu agent sync` with `--all`, `--dry-run` and `--check` (exit 5 on
  Map drift, for CI).
- Portfolio tools: `rivu stats`, `rivu dashboard`, confluences
  (groupings across Channels) and `rivu db export` / `rivu db import`
  backups.
- Configuration system: `~/.rivu/config.toml`, dotted
  `rivu config get|set`, `validate`, `show`, `edit`, env overrides
  (`RIVU_HOME`, `RIVU_CONFIG`, `NO_COLOR`).
- Bubble Tea TUI: three-panel explorer (Flow sidebar, project list,
  detail), command palette (`:`), full key reference (`?`), Master
  Dashboard (`g`), stats, settings, logs, confluences, dry-run previews,
  optional mouse (wheel, click, double-click), responsive layout
  (60–99 single pane, 100–119 two panes, ≥120 three panes), too-small
  rescue screen and a plain-text dashboard fallback with no terminal.
- Scripting contract: stdout data / stderr messages, exit codes
  0/1/2/3/4/5, `--json` with `"schema": 1`.
- Hidden `rivu docs`: man pages and markdown per command plus the
  generated `cli.md` reference.
- Cross-platform CI: lint, tests on Linux/macOS/Windows, race detector
  on Linux/macOS.

### Changed

- The TUI was rebuilt from the flat terminal list into the responsive
  three-panel explorer.
- README rewritten around a 60-second quickstart with verified
  command help and the Flow/Source/Channel/Map/Bank/Delta vocabulary.
- Go requirement is 1.24+; pinned bubbletea v1.3.5, lipgloss, bubbles.

### Fixed

- Workspace scans no longer fail when separate projects share the same
  folder name/slug.
- Rescans preserve the original registry identity for a project path.

[Unreleased]: https://github.com/manojpisini/rivu/compare/v1.0.0...HEAD
