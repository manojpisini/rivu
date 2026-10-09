# Rivu v1.0.0

**Your project filesystem, mapped and flowing.**

Rivu is a cross-platform Go CLI and Bubble Tea TUI that maps your project
folders and keeps them flowing: point it at a workspace, it scans what is
there into a local SQLite registry, and from then on you create, open,
audit and regroup projects by name — headlessly from scripts or
interactively in the terminal.

## Highlights

- **Registry-backed discovery** — `rivu scan` / `rivu index` reconcile
  the filesystem with the registry without clobbering registry-owned
  fields (name, slug, Flow stage, health, created_at); duplicate names
  in different directories get deterministic numeric slugs.
- **Lifecycle with a safety model** — `source`, `flow` and `delta` are
  Plan → confirm → Apply: `--dry-run` previews, `--yes` applies,
  non-TTY without `--yes` exits 4. Nothing in Rivu deletes a project
  folder.
- **Doctor and health** — weighted checks (README, git, Bank, Map,
  tests, CI, licence, deps) with remedies, `--min-score` exit 5, and
  `--fix` for the simple ones.
- **Bank and Map** — protected `.metadata/` files (create-if-missing for
  human docs, parse-and-rewrite for machine files) and a generated
  `PROJECT_MAP.md` agents can read: ranked inventory, entrypoints,
  scripts, safe commands — secret names listed, contents never read.
- **Portfolio view** — `stats`, `dashboard`, confluences, and
  `rivu db export/import` for backups.
- **TUI** — three-panel explorer (Flow sidebar, project list, detail),
  command palette (`:`), full key reference (`?`), Master Dashboard
  (`g`), mouse optional, responsive down to 60×15 with a graceful
  too-small screen and a plain-text fallback when no terminal is
  attached.
- **Scripting contract** — stdout is data, stderr is messages; exit
  codes 0/1/2/3/4/5; `--json` outputs carry `"schema": 1`.
- **CI** — lint, tests and race detector on Linux, macOS and Windows.

## Install

```bash
go install github.com/manojpisini/rivu/cmd/rivu@latest   # Go 1.24+
# or from source:
git clone https://github.com/manojpisini/rivu.git && cd rivu && go build -o rivu ./cmd/rivu
```

Packaged binaries (scoop, brew, apt) are not published yet — build from
source until the first release tag.

## Quickstart

```bash
rivu init --root ~/dev    # config + database for a workspace
rivu scan                 # discover what is in there
rivu tui                  # explore it
```

See [README.md](README.md) for the vocabulary, essential commands,
TUI keys, configuration, safety rules and FAQ.

## Upgrade

Releases are pre-1.0 source builds; update with `git pull` (or
reinstall at a tag), then:

```bash
go mod tidy && go test ./...
rivu version               # should print the new version
rivu scan                  # rescan reconciles the registry safely
```

Registry schema v1 is frozen — upgrading never rewrites your database
destructively; migrations back the file up first.

## Verify

```bash
rivu version --json        # version, commit, date
rivu doctor --min-score 0  # health baseline for every project
rivu agent sync --check    # exits 5 if any Map needs a sync (CI gate)
```
