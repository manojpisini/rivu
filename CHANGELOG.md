# Changelog

## 1.0.1 — Explorer Dashboard and Scan Reliability

- Fixed workspace scans failing when separate projects share the same folder/name slug.
- Preserved stable project IDs and slugs when rescanning an existing path.
- Added deterministic numeric slug disambiguation (`admin`, `admin-2`, and so on).
- Rebuilt the TUI as a responsive three-panel project explorer.
- Added portfolio metrics, Flow navigation, project filtering, search, details, capability status, and command hints.
- Added registry regression tests for duplicate names and stable rescans.

## 1.0.0 — 2026-08-06

- Initial production release.
- Added registry-backed project discovery and lifecycle management.
- Added Source, Flow, Delta, Doctor, Map, Stats, Dashboard, Open, and TUI workflows.
- Added protected Bank generation, safety confirmations, configuration bootstrap, tests, and cross-platform CI.
