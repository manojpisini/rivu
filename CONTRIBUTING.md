# Contributing to Rivu

Thanks for helping build Rivu — "your project filesystem, mapped and
flowing". This document is the short path from clone to merged PR.

## Setup

- Go **1.24+** (see `go.mod`).
- Optional: `golangci-lint` v2.x, Python 3 (for `scripts/sync_tracker.py`).

```bash
git clone https://github.com/manojpisini/rivu
cd rivu
make check          # fmt + test + vet
```

## The loop

1. Read `AGENTS.md` (repo conventions) and the spec in
   `docs/rivu_project_spec.md` for the behaviour you are touching.
2. Branch: `feature/<slug>` or `fix/<slug>` (phase branches are
   `phase/<n>-<slug>`).
3. Write code **with tests in the same change**.
4. Verify before you push:

   ```bash
   gofmt -l .                     # must print nothing
   go vet ./...
   go test ./... -count=1
   golangci-lint run ./...        # optional locally, required in CI
   python scripts/sync_tracker.py --check   # if you touched the tracker
   ```

5. Open a PR against `master`. CI runs lint + tests on Linux, macOS and
   Windows plus the race detector.

## Ground rules

- **Safety first (spec §4.4).** Nothing in Rivu deletes a project
  folder — ever. Every mutating action is Plan → confirm → Apply
  (`--yes` is the only bypass). Protected files
  (`README.md`, `.metadata/overview.md`, `decisions.md`, `tasks.md`,
  `.metadata/agent/AGENTS.md`) are create-if-missing only.
- **Registry is truth, filesystem is discovery.** Rescans never
  overwrite `health_score`, `name`, `slug`, `flow_stage`, `created_at`.
- **Never read or print secret contents** (`.env*`, `*.pem`, `id_rsa*`,
  `*.key`) — file names only.
- **Vocabulary**: Flow, Source, Channel, Current, Map, Bank, Delta,
  Confluence — not "archive", "status" or "category" in user-facing
  strings.
- **No new dependencies** without discussion in the PR (the allowed
  list lives in `AGENTS.md` §12).
- CLI and TUI share `internal/service` — no business logic in either
  front end.

## Style

- `gofmt` is not optional; doc comments on exported symbols; lowercase
  errors without trailing punctuation; `fmt.Errorf("...: %w", err)`.
- Prefer stdlib (`slices`, `maps`, `errors.Is/As`, `filepath.WalkDir`)
  over new packages.
- Key shortcuts go through the single `keyMap` table in
  `internal/tui/keymap.go` so help and handlers cannot drift.

## Commits and PRs

- Natural summary messages ("Warn when a workspace lives under a
  mounted filesystem"), one concern per PR where practical.
- PR description: what changed, why, how you verified.
- By contributing you agree your work is released under the MIT
  license of this repository.

## Code of conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).
