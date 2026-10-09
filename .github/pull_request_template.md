## Summary

<!-- What changed and why. One or two sentences. -->

## How verified

<!-- Commands you ran and their result, e.g. go test ./... -count=1 (ok), golangci-lint (0 issues). -->

## Checklist

- [ ] `gofmt -l .` is empty and `go vet ./...` passes
- [ ] Tests added or extended in the same change; `go test ./...` green
- [ ] No new dependency (or justified in the summary above)
- [ ] User-facing strings follow the Flow/Source/Channel/Map/Bank/Delta vocabulary
- [ ] Safety invariants kept (spec §4.4): no project-folder deletion, Plan → confirm → Apply, protected files create-if-missing
- [ ] Docs updated if behaviour or flags changed (`docs/` + README)
- [ ] Tracker updated (`sync_tracker.py --check` in sync) if this was a tracked task
