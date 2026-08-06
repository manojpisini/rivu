# Rivu — Project Flow System
### Consolidated Spec Sheet · Developer Sheet · Production Sheet · Build Sheet

**Document version:** 1.0
**Supersedes:** `lode_deck_project_spec.md` v0.1 and `lode_deck_addendum_v0.2.md`
**Replaces filename:** `rivu_project_spec.md`
**Primary language:** Go
**TUI stack:** Bubble Tea, Lip Gloss, Bubbles
**Tagline:** Rivu — your project filesystem, mapped and flowing.
**Identity statement:** Rivu is a calm TUI control deck for creating, indexing, opening, auditing, and agent-mapping structured project folders.

---

# Part 0 — How to read this document

This single document carries four roles, each previously a separate concern:

```txt
Spec Sheet        what Rivu is, why it exists, what it must do
Developer Sheet    how it's built — language, libraries, schema, module layout
Production Sheet   what it looks like — every TUI screen, copy, visual system
Build Sheet         the order to build it in, day-by-day, with a definition of done
```

Read top to bottom for onboarding; jump to a Part by name once you're building.

---

# Part 1 — Spec Sheet

## 1.1 Naming system

```txt
Project name:       Rivu
CLI binary:         rivu
TUI title:          RIVU
Config folder:      ~/.rivu/
Database:           ~/.rivu/rivu.db
Cache:              ~/.rivu/cache/
Global index:       ~/.rivu/index/
Project metadata:   .metadata/
Agent layer:        .metadata/agent/
```

```txt
Module name:     rivu
Package root:    github.com/<you>/rivu
Binary:          rivu
App title:       RIVU
```

There is deliberately no separate "engine name" the way the earlier draft split `lode-deck` (repo) from `atlas` (binary) from `Project Atlas` (subsystem). Rivu is one name end to end — repo, binary, and product all read the same, which keeps docs, error messages, and muscle memory aligned.

## 1.2 Internal vocabulary

This is the single most important section in the document. Every screen, log line, error message, and CLI help string should use these words consistently, and never their old equivalents (status, category, selection, index, metadata, archive, group).

```txt
Word         Meaning                                  Replaces
─────────    ─────────────────────────────────────    ──────────────────────
Flow         a project's lifecycle stage               "status" / "lifecycle stage"
Source       the act of creating a new project          "new project" / "create"
Channel      the folder/category path a project lives   "lifecycle folder" / "category path"
             in
Current      the active, selected project in context     "selected project" / "open project"
Map          the project + agent index                   "agent index" / "project map"
Bank         the metadata boundary for a project          ".metadata/" / "metadata files"
Delta        archived / completed projects                "archive"
Confluence   a named grouping of related projects         (new concept, no prior equivalent)
```

### 1.2.1 Flow stages (replaces lifecycle folders / numbered prefixes)

The old spec used numbered folders (`01_Inbox`, `02_Active`, `03_Maintenance`, `04_Research`, `90_Archive`) as both the on-disk layout and the status vocabulary in one. Rivu separates the two: **Channels** are on-disk paths, **Flow stages** are the logical state a project is in. A Channel maps to exactly one default Flow stage, but a project's Flow stage is registry truth, not folder truth — this is what allows "registered but not yet physically moved" states to exist cleanly (see §1.4).

```txt
Flow stage     Default Channel        Meaning
───────────    ───────────────────    ─────────────────────────────────
source         00_Source              newly created or discovered, untriaged
active         01_Active              in active development
maintenance    02_Maintenance         stable, occasional upkeep
research       03_Research            exploratory, spec-stage, not yet built
delta          90_Delta               archived, paused indefinitely, or done
```

### 1.2.2 Current (replaces "selected project")

**Current** is a first-class concept, not just cursor position. The TUI always shows which project is Current in the header bar, and most single-target commands (`rivu open`, `rivu flow`, `rivu doctor`, `rivu agent sync`) operate on Current by default when no project argument is given. This mirrors a shell's working-directory model: you can act explicitly on a named project, or implicitly on whatever is Current.

```bash
rivu open                # opens Current
rivu open opencourses    # opens named project, also sets it as Current
```

### 1.2.3 Map (replaces "agent index")

A **Map** is the generated, agent-readable layer for a project: its file index, entrypoints, ignore rules, and read-first ordering. "Generate the agent map" becomes "build the Map." This is a one-word rename with no behavior change from the prior spec's §12.

### 1.2.4 Bank (replaces ".metadata/")

The **Bank** is the protected zone of human- and Rivu-authored files for a project: `project.toml`, `overview.md`, `decisions.md`, `tasks.md`, and the Map underneath `agent/`. "Bank files are protected from silent overwrite" replaces "protected files" language throughout. Calling it a Bank (a boundary that holds something of value, that you don't casually edit) is more semantically loaded than "a metadata folder," and it gives the doctor/health system a clean phrase: "Bank is intact," "Bank is incomplete," "Bank is missing."

### 1.2.5 Delta (replaces "archive")

**Delta** is both a Flow stage and a verb: "delta a project" means archive it. Using a single word for both noun and verb keeps CLI and UI copy terse (`rivu archive opencourses` becomes `rivu delta opencourses`).

### 1.2.6 Confluence (new concept)

A **Confluence** is a named, user-defined grouping of related projects that may live in different Channels and have different Flow stages, but conceptually belong together — for example, a monorepo's logical sub-projects, a "Heap & Stack publication" family (the newsletter repo, the YouTube asset repo, the brand template), or all projects under a shared domain like `devtools`. Confluences are tags with a dedicated browsing view (see Production Sheet §3.7), not folders — a project can belong to more than one Confluence without being physically moved or duplicated.

```toml
# .metadata/project.toml
[rivu]
confluences = ["heap-and-stack", "devtools"]
```

## 1.3 Product thesis

Rivu should make a personal project folder feel like a single, legible system rather than a pile of directories. It answers:

- What projects exist, and which Channel and Flow stage is each one in?
- Which project is Current, and what should happen to it next?
- What Confluences exist, and which projects belong to them?
- What stack, toolchain, and dependencies does each project use?
- What's incomplete — missing Bank, missing Map, missing git, missing README?
- What should an AI agent read first before touching a project, and what must it never touch?

Rivu does not just render a folder tree. It builds and maintains a **registry** — the database is the source of operational truth, and the filesystem is what gets discovered and reconciled against it.

## 1.4 Core principles

### 1.4.1 Structure first, agents second

Rivu is not an AI wrapper. It produces deterministic structure — Banks, Maps, Flow stages, Confluences — and the agentic layer (Map generation, agent preflight, MCP exposure) reads that structure rather than replacing it. An agent is only as useful as the structure it's handed.

### 1.4.2 Registry is truth, filesystem is discovery

A project can exist on disk and not be registered. A project can be registered and missing on disk. A project's Flow stage in the registry can disagree with which Channel it physically sits in (e.g. it was marked `active` but hasn't been moved into `01_Active` yet). All three mismatch states must be visible and reconcilable, never silently resolved.

### 1.4.3 No destructive action without confirmation

Move, delta, rename, and mass-Bank writes always show a dry-run preview and require explicit confirmation. Nothing in Rivu permanently deletes a project folder.

### 1.4.4 Everything important works headlessly

Every TUI action has a CLI equivalent. The TUI is a view over the same command layer the CLI calls — never a separate code path.

### 1.4.5 Small deterministic build steps

Build in layers, narrowest first. See the Build Sheet (Part 4) for the enforced order — config and registry before any TUI, TUI list before wizard, wizard before Map generation, Map before agentic features.

## 1.5 Product scope

### 1.5.1 MVP

1. Configure a workspace root.
2. Scan the root, detect likely project directories.
3. Store projects in the registry (SQLite).
4. Show projects in the TUI with Flow stage and Channel.
5. Filter/search projects.
6. Show a project detail panel.
7. Open Current in the configured editor.
8. Source a new project through a guided form.
9. Write the Bank (`.metadata/`).
10. Run `git init` where applicable (subject to the Rivu/git-init exclusivity rule, §1.7).
11. Build an initial Map.
12. Run basic doctor checks.

### 1.5.2 v1

1. `rivu flow` — move a project between Flow stages (and physically between Channels).
2. `rivu delta` — archive a project.
3. Package ecosystem detection (node, go, rust, python).
4. Toolchain/package-manager version checks.
5. Outdated-dependency detection where feasible.
6. Test/build/lint script detection.
7. Health snapshots over time.
8. Project tags and Confluences.
9. Custom templates for Source.
10. CLI parity for every TUI action.

### 1.5.3 Later

1. Filesystem watch mode.
2. Background indexing daemon.
3. Embeddings/vector search across Maps.
4. MCP server exposing the registry to agents.
5. Agent task handoff bundles.
6. Local web dashboard mirroring the Master Dashboard.
7. Time tracking per project.
8. Changelog automation.
9. GitHub integration.
10. Dependency-update PR generation.

### 1.5.4 Non-goals for MVP

Full AI chat inside the TUI, cloud sync, remote GitHub API integration, a plugin system, a background daemon, vector search, semantic code analysis, automatic dependency upgrades, and any destructive cleanup of files are explicitly out of scope until later phases, if ever.

## 1.6 Terminology cross-reference

```txt
Term            Definition
─────────────   ────────────────────────────────────────────────────
Workspace root  top-level folder containing all Channels
Project         a directory representing one coherent unit of work
Channel         a Flow-stage-mapped folder under the workspace root
Flow stage      a project's logical lifecycle state (source/active/
                maintenance/research/delta)
Current         the active project in TUI/CLI context
Registry        the global SQLite database of known projects
Bank            the .metadata/ boundary for a project
Map             the agent-readable index under .metadata/agent/
Confluence      a named cross-cutting grouping of related projects
Doctor          the health-check and diagnostic subsystem
Rivu bridge     (optional) integration with the separate `lode`
                per-project scaffold/preference tool
```

## 1.7 The Rivu/git-init exclusivity rule

Where a per-project scaffold tool (e.g. `lode`) already performs `git init` as part of its own bootstrap, Rivu must never run a second `git init` against the same project. This is enforced, not advisory:

```txt
Bridge init enabled?   Git init field state         Behavior
────────────────────   ───────────────────────      ──────────────────────────────
yes                     locked → "via bridge"        Rivu skips its own git init;
                                                       the bridge tool owns it
no                      user-controlled (yes/no)      Rivu runs git init itself if yes
```

If a user sets Git init to yes first and then enables the bridge afterward, Source must warn and auto-correct the conflicting field rather than allow both to run. Doctor includes a corresponding check:

```txt
! .git present AND bridge scaffold marker present AND project.toml
  shows git_init_owner = "rivu" — possible double-init, verify history
```

Config:

```toml
[automation]
bridge_owns_git_init = true   # when the bridge tool runs, rivu never calls git init itself
```

---

# Part 2 — Developer Sheet

## 2.1 Required stack

```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
```

```txt
Bubble Tea   application runtime — Model / Init / Update / View, async
             commands, keyboard + resize events, screen composition
Lip Gloss    visual system — borders, panels, status bars, selected-row
             styling, badges, responsive width/height layout
Bubbles      components — lists, tables, text inputs/areas, spinners,
             progress bars, viewport/pager, help view, keymap rendering
```

## 2.2 Supporting packages

```bash
go get github.com/spf13/cobra          # CLI shell
go get github.com/spf13/viper          # config (or koanf, cleaner alternative)
go get modernc.org/sqlite              # pure-Go SQLite, no CGO friction
go get github.com/google/uuid          # project IDs
go get github.com/BurntSushi/toml      # Bank file parsing/writing
go get github.com/fsnotify/fsnotify    # optional, later watch mode
```

`modernc.org/sqlite` is preferred over `mattn/go-sqlite3` specifically because it avoids CGO, which matters for clean cross-compilation to Windows/WSL/macOS from a single build pipeline.

## 2.3 Module layout

```txt
rivu/
  cmd/rivu/              main.go, version vars, ldflags hooks
  internal/cli/          cobra command tree (one file per command)
  internal/tui/           bubbletea models, one subpackage per screen
    dashboard/
    masterdashboard/
    projectlist/
    detail/
    source/               (the "new project" wizard — named after Source)
    settings/
    stats/
    confluence/
    health/
    map/
  internal/registry/      sqlite connector, migrations, repository methods
  internal/scanner/       walk, ignore rules, marker/stack detection
  internal/bank/          .metadata/ writer, idempotency, protected-file
                          guard
  internal/mapgen/        Map (agent index) generator
  internal/doctor/        health checks, scoring, snapshots
  internal/deps/          ecosystem + dependency detection (node/go/
                          rust/python)
  internal/bridge/        optional integration with external scaffold
                          tools (e.g. lode)
  internal/editorlaunch/  editor command rendering + detection
  internal/config/         config model, defaults, load/save, paths
  internal/style/          shared lipgloss style system
  pkg/...                  anything intended for external reuse, if any
```

## 2.4 Configuration design

```toml
# ~/.rivu/config.toml

[workspace]
root = "D:/Projects"
secondary_roots = []
auto_rescan_on_launch = true

[editors]
default = "nvim"
[editors.per_language]
rust = "nvim"
go = "nvim"
typescript = "code"
javascript = "code"

[automation]
bridge_owns_git_init = true
create_bank_by_default = true
build_map_by_default = true

[flow]
stale_threshold_days = 45
source_sla_days = 14

[scanner]
ignore = ["node_modules", ".git", "dist", "build", ".next", "target",
          "vendor", "coverage", ".cache", ".venv", "__pycache__"]
max_depth = 6

[health]
weights = { readme = 20, git = 15, bank = 15, map = 15, tests = 10,
            ci = 10, license = 10, deps_fresh = 5 }

[appearance]
theme = "graphite-violet"
density = "comfortable"

[data]
db_path = "~/.rivu/rivu.db"
snapshot_retention_days = 90
```

## 2.5 Registry schema (SQLite)

```sql
CREATE TABLE projects (
  id              TEXT PRIMARY KEY,       -- uuid
  name            TEXT NOT NULL,
  slug            TEXT NOT NULL UNIQUE,
  path            TEXT NOT NULL,
  channel         TEXT NOT NULL,          -- e.g. "01_Active"
  flow_stage      TEXT NOT NULL,          -- source|active|maintenance|research|delta
  language        TEXT,
  stack           TEXT,                   -- json array
  has_git         BOOLEAN DEFAULT 0,
  has_bank        BOOLEAN DEFAULT 0,
  has_map         BOOLEAN DEFAULT 0,
  health_score    INTEGER,
  created_at      DATETIME,
  last_opened_at  DATETIME,
  last_scanned_at DATETIME,
  on_disk         BOOLEAN DEFAULT 1,      -- false if registry-only mismatch
  registered      BOOLEAN DEFAULT 1       -- false if disk-only mismatch
);

CREATE TABLE confluences (
  id     TEXT PRIMARY KEY,
  name   TEXT NOT NULL UNIQUE,
  notes  TEXT
);

CREATE TABLE project_confluences (
  project_id     TEXT REFERENCES projects(id),
  confluence_id  TEXT REFERENCES confluences(id),
  PRIMARY KEY (project_id, confluence_id)
);

CREATE TABLE health_snapshots (
  id          TEXT PRIMARY KEY,
  project_id  TEXT REFERENCES projects(id),
  score       INTEGER,
  taken_at    DATETIME
);

CREATE TABLE activity_log (
  id          TEXT PRIMARY KEY,
  project_id  TEXT REFERENCES projects(id),
  event       TEXT,        -- opened|created|flowed|deltaed|map_built|...
  occurred_at DATETIME
);
```

## 2.6 Bank file templates

```markdown
<!-- .metadata/overview.md -->
# Project Overview: {{ .Name }}

## Purpose

## Current status

## Important context

## Next steps
```

```markdown
<!-- .metadata/decisions.md -->
# Decisions

## YYYY-MM-DD — Initial project setup
```

```markdown
<!-- .metadata/tasks.md -->
# Tasks

## Now
## Next
## Later
```

```toml
# .metadata/project.toml
[rivu]
id = "{{ .ID }}"
name = "{{ .Name }}"
slug = "{{ .Slug }}"
flow_stage = "{{ .FlowStage }}"
channel = "{{ .Channel }}"
confluences = []
git_init_owner = "rivu"   # or "bridge"
created_at = "{{ .CreatedAt }}"
```

## 2.7 Map (agent index) templates

```markdown
<!-- .metadata/agent/AGENTS.md -->
# Agent Instructions for {{ .Name }}

## Read first
## Project purpose
## Important rules
## Ignore by default
## Safe commands
```

```markdown
<!-- .metadata/agent/PROJECT_MAP.md -->
# Project Map: {{ .Name }}

## Purpose
## Stack
## Important files
## Important directories
## Generated/vendor directories
## Suggested first-read order
```

## 2.8 Project detection rules

A directory is a project candidate if it contains at least one strong marker:

```txt
Strong markers:  .git  package.json  go.mod  Cargo.toml  pyproject.toml
                 .metadata/project.toml
Weak markers:    README.md alone, a non-empty src/ alone — counted but
                 not sufficient on their own
```

Stack/language is inferred from marker files (`go.mod` → Go, `Cargo.toml` → Rust, `package.json` → Node/TS/JS by further inspection of `type`/`devDependencies`, `pyproject.toml` → Python). Generated/vendor directories (`node_modules`, `.git`, `dist`, `build`, `.next`, `target`, `vendor`, cache folders) are skipped by the scanner by default unless explicitly inspected.

## 2.9 CLI design

```bash
rivu                    # shorthand for `rivu tui`
rivu tui                # launch the TUI
rivu new                # alias: rivu source — guided project creation
rivu open [project]     # opens Current, or named project (sets Current)
rivu scan               # rescan workspace root(s)
rivu flow [project] --to <stage>   # move a project between Flow stages
rivu move               # alias for flow, kept for muscle memory
rivu index              # rebuild the registry index from disk + DB
rivu doctor [project]   # health checks; omit project for all projects
rivu stats              # portfolio metrics (see Production Sheet §3.6)
rivu deps [project]     # dependency/ecosystem inspection
rivu agent sync [project]  # build/refresh the Map
rivu archive [project]  # alias: rivu delta
rivu delta [project]    # archive a project
rivu confluence         # manage/browse Confluences
rivu dashboard          # static snapshot of the Master Dashboard
rivu settings           # open Settings directly in the TUI
```

Every command supports `--dry-run` where it performs filesystem or git actions, and every destructive command requires `--yes` to skip the interactive confirmation when run non-interactively.

## 2.10 Bubble Tea architecture

```txt
Root Model
  ├─ current screen (enum: dashboard, masterdashboard, projectlist,
  │   detail, source, settings, stats, confluence, health, map,
  │   search, dryrun, logs, help)
  ├─ shared state: workspace root(s), Current project, registry handle,
  │   config, theme
  └─ per-screen sub-model, swapped via Update() on screen-change msgs
```

Each screen is its own Bubble Tea sub-model implementing `Init/Update/View`, composed into the root model's `View()` via Lip Gloss layout joins. Async work (scan, doctor run, Map build) is always a `tea.Cmd` returning a typed result message — the TUI never blocks on filesystem or git calls.

## 2.11 Lip Gloss style system

```txt
Background:  near-black / graphite
Accent:      violet (Rivu's primary brand color)
Good:        green
Warning:     yellow
Danger:      red
Borders:     subtle gray
Text:        off-white
Muted:        gray
```

Badges use a consistent bracketed format and the new vocabulary throughout:

```txt
[ACTIVE] [GO] [Nvim] [Git] [Bank:OK] [Map:Missing] [Health:72] [Confluence:heap-and-stack]
```

---

# Part 3 — Production Sheet

## 3.1 Screen list

```txt
Dashboard            (daily-driver, project-list-first)
Master Dashboard      (metrics-first home screen)
Project List
Project Detail
Source (new project wizard)
Confluence browser
Stats / Metrics
Settings
Search / Query
Health Report
Map Report
Dry Run / Confirmation
Logs
Help
```

## 3.2 Dashboard layout

```txt
┌──────────────────────────────── RIVU ─────────────────────────────┐
│ Root: D:/Projects                         Projects: 128            │
│ Active: 12   Source: 8   Maint: 5   Delta: 81   Unhealthy: 7       │
├────────────────┬───────────────────────────────────────────────────┤
│ Flow            │ Projects                                         │
│                 │                                                  │
│ ▸ Source        │  OpenCourses          source    TS  Bun   72     │
│   Active        │  MX-Language          source    Spec     60     │
│   Maintenance   │  Rivu                 active    Go        88     │
│   Research      │  Portfolio TUI        active    Go        80     │
│   Delta         │                                                  │
├────────────────┴───────────────────────────────────────────────────┤
│ / search  n source  enter open  d detail  h doctor  a map  g master│
└───────────────────────────────────────────────────────────────────┘
```

## 3.3 Master Dashboard layout

```txt
┌────────────────────────────── RIVU — Master ───────────────────────────────────┐
│ Root: D:/Projects     Roots: 2     Last scan: 4m ago     Health: 74/100        │
├────────────────────────────┬───────────────────────────────────────────────────┤
│ Portfolio                  │ Needs attention                                   │
│ Total projects      128    │ ! 22 projects missing Map                         │
│ Active                12    │ ! 7 stale projects (45d+)                         │
│ Source (untriaged)     8    │ ! 4 missing README                                │
│ Delta                  81   │ ! 2 registry/filesystem mismatches                │
├────────────────────────────┼───────────────────────────────────────────────────┤
│ By language                 │ Recent activity                                  │
│ Go    ████████ 34           │ 09:14  opened    OpenCourses                     │
│ TS    ██████ 27              │ 08:55  health     MX-Language     →  60          │
│ Rust  ████ 18                  │ 08:40  sourced    repo-doctor                    │
│ Py    ███ 14                   │ Yesterday  deltaed  old-prototype                │
├────────────────────────────┴───────────────────────────────────────────────────┤
│ Quick actions: [n] source  [/] search  [s] stats  [h] doctor all  [a] map all   │
└───────────────────────────────────────────────────────────────────────────────┘
```

"Needs attention" is a ranked triage queue, not a passive readout, prioritized:

```txt
1. registry/filesystem mismatch (registered but missing, or on-disk but unregistered)
2. missing git
3. missing Bank / missing Map
4. stale (past the configured threshold)
5. outdated dependencies (once detection lands in v1)
```

Pressing enter on any "needs attention" line jumps straight into that project's Detail screen with the relevant fix action pre-highlighted.

## 3.4 Project detail layout

```txt
┌──────────────────────────── OpenCourses ───────────────────────────┐
│ Channel: 00_Source/Undecided/OpenCourses                           │
│ Flow:    source                                                    │
│ Stack:   TypeScript · Astro · Bun · npm                            │
│ Git:     initialized                                                │
│ Bank:    OK                                                         │
│ Map:     missing                                                    │
│ Health:  72/100                                                     │
├──────────────────────────── Actions ────────────────────────────────┤
│ [enter] open editor   [h] doctor       [a] build map                │
│ [f] flow              [r] rescan        [g] git status              │
│ [e] edit bank         [c] confluences                                │
├──────────────────────────── Health ─────────────────────────────────┤
│ ✓ README present                                                     │
│ ✓ package.json present                                               │
│ ✓ .github/workflows present                                          │
│ ! Bank (project.toml) missing                                        │
│ ! Map missing                                                        │
│ ! generated dependency folder is large and ignored                   │
└────────────────────────────────────────────────────────────────────┘
```

## 3.5 Source (new project wizard)

A five-step wizard, named after the vocabulary term:

#### Step 1 — Identity

```txt
Source — Identity
──────────────────
Name:        Repo Doctor
Slug:        repo-doctor
Description: Inspect repository health and metadata
```

#### Step 2 — Classification

```txt
Source — Classification
────────────────────────
Flow stage:   active
Type:         cli
Domain:       devtools
Confluences:  devtools
```

#### Step 3 — Stack

```txt
Source — Stack
───────────────
Language:        go
Template:        go-cli
Package manager: go-mod
```

#### Step 4 — Automation

```txt
Source — Automation
─────────────────────
Create Bank:           yes
Bridge init (lode):    no
Git init:              yes
Build Map:             yes
Open editor after:     yes
Editor:                nvim
```

If Bridge init is set to yes, the Git init field becomes locked and reads `via bridge` per the exclusivity rule in §1.7.

#### Step 5 — Dry run

```txt
Dry Run
────────
Will create:
D:/Projects/01_Active/devtools/repo-doctor

Will write:
.metadata/project.toml
.metadata/overview.md
.metadata/tasks.md
.metadata/agent/AGENTS.md
.metadata/agent/PROJECT_MAP.md

Will run:
git init
rivu agent sync repo-doctor
nvim D:/Projects/01_Active/devtools/repo-doctor

Confirm? y/N
```

## 3.6 Stats / Metrics screen

```txt
┌──────────────────────────── Stats ────────────────────────────────┐
│ Range: [ All time ▾ ]                              Export: csv json│
├─────────────────────────────────────────────────────────────────┤
│ Projects total           128        Avg health score        74    │
│ Active                    12        Median time-to-active   9d    │
│ Source (untriaged)         8        Stale (45d+)              7   │
│ Delta                      81       Missing Map              22   │
│ With git                  119       Missing README             4  │
├─────────────────────────────────────────────────────────────────┤
│ By language                 By flow                  By health   │
│ Go         ████████ 34      active     ██ 12          90-100 ██9 │
│ TypeScript ██████ 27        source     █  8           70-89 ████41│
│ Rust       ████ 18          delta      ████████ 81    50-69 ███30 │
│ Python     ███ 14           maint.     █ 5             0-49 ██12  │
├─────────────────────────────────────────────────────────────────┤
│ Activity (commits/open events, last 30 days)                      │
│ ▁▂▃▅▇▆▄▃▂▁▂▃▄▆▇▅▃▂▁▁▂▃▄▅▆▇▆▄▃▂                                     │
├─────────────────────────────────────────────────────────────────┤
│ [t] toggle by-language/flow   [e] export   [d] drill into band    │
└─────────────────────────────────────────────────────────────────┘
```

Per-project trend drill-down:

```txt
OpenCourses — Health trend
100 ┤
 80 ┤              ●──●
 60 ┤      ●──●───●
 40 ┤  ●───
    └────────────────────────
     Jan  Feb  Mar  Apr  May
```

## 3.7 Confluence browser

```txt
┌────────────────────────── Confluences ────────────────────────────┐
│ ▸ heap-and-stack     3 projects     newsletter, brand, content     │
│   devtools           6 projects     cli tools, internal utilities  │
│   hsir-runtime       4 projects     core, cli, daemon, docs        │
├──────────────────────────────────────────────────────────────────┤
│ heap-and-stack                                                     │
│   Heap & Stack Site      active      Astro                         │
│   Brand Master Template  delta       Markdown                      │
│   H&S YouTube Assets     source      mixed                         │
├──────────────────────────────────────────────────────────────────┤
│ [n] new confluence  [enter] open project  [e] edit membership      │
└──────────────────────────────────────────────────────────────────┘
```

A project can appear under more than one Confluence; membership is many-to-many, edited from either the Confluence browser or the project Detail screen (`[c] confluences`).

## 3.8 Settings screen

```txt
┌──────────────────────────── Settings ──────────────────────────────┐
│ ▸ General        Workspace root       D:/Projects                  │
│   Editors        Default editor       nvim                         │
│   Flow           Channel mapping      00_Source … 90_Delta          │
│   Bridge         Bridge integration   disabled                     │
│   Templates      Default template     go-cli                       │
│   Health         Score weighting      custom                       │
│   Scanner        Ignore list          node_modules, dist, .venv... │
│   Appearance     Theme                graphite-violet               │
│   Keybindings    Profile              default (vim-style)          │
│   Data           DB location          ~/.rivu/rivu.db               │
│   About          Version              0.1.0                        │
├──────────────────────────────────────────────────────────────────┤
│ tab section  enter edit  ctrl+s save  esc cancel  r reset defaults │
└──────────────────────────────────────────────────────────────────┘
```

Section details (General, Editors, Flow, Bridge, Templates, Health, Scanner, Appearance, Keybindings, Data, About) follow the same field sets specified in Part 2's config schema (§2.4) — every field shown here maps one-to-one onto a `config.toml` key so Settings is never out of sync with the file a person could hand-edit directly.

## 3.9 Keybindings

Global:

```txt
q          quit / back
ctrl+c     quit immediately
?          help
/          search
esc        cancel current modal
r          refresh/rescan
:          command mode (fuzzy-matched action palette)
g          jump to Master Dashboard
```

Project list:

```txt
j/down     move down
k/up       move up
enter      open project in preferred editor (sets Current)
space      multi-select for bulk actions
d          detail
n          source (new project)
h          doctor
f          flow (move between stages)
A          delta (archive)
x          actions menu
a          build map
c          confluences
u          smart filter: untriaged (source stage, past SLA)
```

Source wizard:

```txt
tab        next field
shift+tab  previous field
enter      accept field / next step
ctrl+s     save/create
esc        cancel
```

## 3.10 Visual direction

```txt
Background: near black / graphite
Accent:     violet
Good:       green
Warning:    yellow
Danger:     red
Borders:    subtle gray
Text:       off-white
Muted:      gray
```

Restrained design throughout — minimal ASCII art, dense but readable tables, badges as the primary status-communication device rather than prose. This aligns with the existing Industrial Minimalist / dark, high-contrast, data-dense aesthetic preference already established for other projects in this portfolio.

---

# Part 4 — Build Sheet

## 4.1 Recommended build order

```txt
1.  CLI boot
2.  Config
3.  DB migration
4.  Scanner
5.  Registry upsert/list
6.  Minimal TUI
7.  Project list
8.  Editor open (sets Current)
9.  Project detail
10. Source dry-run
11. Bank writer
12. Source execution
13. Git init (with exclusivity rule, §1.7)
14. Health/doctor checks
15. Map generator
16. Bridge integration (optional, e.g. lode)
16a. Settings screen
16b. Stats/Metrics screen
16c. Master Dashboard screen
16d. Confluence browser
17. Flow / Delta (move/archive)
18. Query language (command palette, §3.9)
19. Tests
20. Polish
```

Settings, Stats, the Master Dashboard, and the Confluence browser are inserted after Bridge integration and before Flow/Delta, since all four depend only on data the earlier phases already produce, and none of them block the Flow/Delta milestone.

## 4.2 First 7-day build schedule

### Day 1

- Bootstrap repository, add Cobra, add config model, add `rivu init`-equivalent (workspace setup), add SQLite open/migrate.

Deliverable: `rivu init --root D:/Projects` (or config bootstrap on first `rivu` run)

### Day 2

- Build scanner, implement ignore rules, implement marker scoring, add `rivu scan`, store projects in SQLite.

Deliverable: `rivu scan` / `rivu list`

### Day 3

- Add minimal Bubble Tea TUI, project list, filtering, detail panel placeholder.

Deliverable: `rivu tui`

### Day 4

- Add editor launcher, `rivu open`, wire Enter key in TUI, update `last_opened_at`, set Current.

Deliverable: `rivu open repo-doctor --editor nvim`

### Day 5

- Add Source pipeline, Channel placement rules, dry run, Bank writer.

Deliverable: `rivu new --name "Repo Doctor" --domain devtools --language go --dry-run`

### Day 6

- Add Source TUI wizard, git init (with exclusivity check), basic starter files, register created project.

Deliverable: `rivu tui → n → source project`

### Day 7

- Add doctor checks, Map generator, `rivu doctor` and `rivu agent sync`.

Deliverable: `rivu doctor repo-doctor` / `rivu agent sync repo-doctor`

## 4.3 Definition of done for MVP

MVP is done when:

- `rivu` bootstraps config and DB on first run;
- `rivu scan` discovers projects from the workspace root, skipping generated/vendor directories by default;
- `rivu tui` shows a useful project list with Flow stage and Channel;
- search/filter works;
- the Current project opens in `nvim`, `code`, or `zed`;
- the Source wizard creates a project in the correct Channel;
- `.metadata/project.toml` (Bank) is written;
- `.metadata/overview.md` and `.metadata/tasks.md` are written;
- git init runs correctly and respects the exclusivity rule;
- a basic doctor/health report works;
- the Map (agent index) is generated;
- all destructive operations require confirmation;
- core functions have tests.

## 4.4 Safety rules

```txt
1. Never delete a project folder silently — no command does this, ever.
2. Always dry-run flow/delta moves, showing source, destination, files
   affected, registry changes, and Bank changes before acting.
3. Never index generated directories by default (node_modules, .git,
   dist, build, .next, target, coverage, .cache, .venv).
4. Never overwrite Bank/human docs without confirmation — protected:
   README.md, AGENTS.md, .metadata/overview.md, .metadata/decisions.md,
   .metadata/tasks.md.
5. Keep registry and filesystem consistent after every flow/delta:
   update DB, update Bank, rescan, report success/failure explicitly.
```

## 4.5 Versioning and builds

```go
// cmd/rivu/version.go
var Version = "dev"
var Commit = "none"
var Date = "unknown"
```

```bash
go build -ldflags "-X main.Version=0.1.0 -X main.Commit=$(git rev-parse --short HEAD)" -o bin/rivu ./cmd/rivu

GOOS=windows GOARCH=amd64 go build -o dist/rivu.exe ./cmd/rivu
GOOS=linux   GOARCH=amd64 go build -o dist/rivu-linux ./cmd/rivu
GOOS=darwin  GOARCH=arm64 go build -o dist/rivu-darwin-arm64 ./cmd/rivu
```

## 4.6 First implementation checkpoint

The smallest meaningful target — build nothing past this until it works end to end:

```bash
rivu init --root D:/Projects
rivu scan
rivu tui
```

```txt
RIVU

Projects
- OpenCourses       00_Source/Undecided/OpenCourses       typescript bun
- MX-Language       00_Source/Undecided/MX-Language        unknown unknown

Press enter to open, q to quit.
```

Only after this works should the Source wizard be built.

## 4.7 Future agentic layer (post-MVP)

```txt
MCP server tools:    list_projects, get_project, get_project_map,
                      get_project_health, search_projects, open_project
Agent files (Map):    .metadata/agent/TASK_CONTEXT.md
                      .metadata/agent/NEXT_ACTIONS.md
                      .metadata/agent/SAFE_COMMANDS.md
Agent preflight:      rivu agent preflight opencourses
                      → read-first list, do-not-edit list, safe commands
Agent handoff bundle: rivu agent bundle opencourses
                      → .metadata/agent/handoff.md (purpose, status,
                        decisions, open tasks, Map, health warnings)
```

The agentic layer reads the structure Rivu's deterministic layers already produced — Channels, Flow stages, the Bank, the Map — it never substitutes for them.

---

# Part 5 — Source references

```txt
Bubble Tea:  https://github.com/charmbracelet/bubbletea
Lip Gloss:   https://github.com/charmbracelet/lipgloss
Bubbles:     https://github.com/charmbracelet/bubbles
```

Prior documents this consolidates and supersedes: `lode_deck_project_spec.md` (v0.1), `lode_deck_addendum_v0.2.md`.

---

# Part 6 — Summary

Rivu is a personal project operating system in the terminal, built around one consistent vocabulary — Flow, Source, Channel, Current, Map, Bank, Delta, Confluence — applied uniformly across the registry schema, the CLI surface, the TUI copy, and the documentation itself. The most important design choice carried over from the earlier draft still holds: build the deterministic structure first — Channels, Flow stages, the Bank, the Map — and let the agentic layer read that structure rather than replace it.
