# Rivu v1.0.1

This maintenance release fixes duplicate project-name scanning and replaces the flat terminal list with a responsive file-explorer dashboard.

## Highlights

- Duplicate project names in different directories are now supported.
- Rescans preserve the original registry identity for a project path.
- The TUI now includes Flow navigation, a searchable project table, portfolio metrics, a project-details panel, and responsive compact behavior.

## TUI keys

- `Tab`, `h/l`, or left/right: switch between Flow and project panels
- `j/k` or arrows: navigate
- `/`: search
- `Esc`: clear search
- `o` or `Enter`: show the exact open command
- `d`: show the Doctor command
- `m`: show the Agent Map command
- `r`: show the rescan command
- `q`: quit

## Upgrade

Replace the v1.0.0 source with this release, then run:

```powershell
go mod tidy
go test ./...
go run ./cmd/rivu scan
go run ./cmd/rivu
```
