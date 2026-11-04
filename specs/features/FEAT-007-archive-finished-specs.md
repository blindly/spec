---
covers: [archive.go, commands.go, help.go, inbox.go, main.go, store.go, web.go, agent.go, config.go, templates.go]
created: 2026-11-01
depends_on: [FEAT-006, FEAT-008]
id: FEAT-007
status: implemented
title: Archive finished specs
type: feature
verified: 425321963e1a1691e1d9a56b9eebe3c8e91bae46
---

# Archive finished specs

## Summary

Add `spec archive <id>...`, which moves specs that are no longer valid
(deprecated, rejected) and discarded ideas into `specs/archive/`. Archived
specs disappear from the board, `apply`, `status`, and `roadmap`, but stay
in the repo, so they remain searchable, referenceable, and in git history.
`spec unarchive` moves them back. Implemented specs are living
documentation and are not archived (ADR-003).

## Motivation

Specs that no longer describe the system (deprecated, rejected) and ideas
that were decided against pile up and clutter every view. Archiving retires
them while keeping the history. "Finished" here means "no longer valid", not
"implemented": an implemented spec stays visible for as long as it's true,
and is updated when the system changes (FEAT-010).

## Requirements

Addresses: REQ-012, REQ-011

## Design

- **Layout:** an archived spec moves to `specs/archive/<type dir>/` with the
  same filename, e.g. `specs/archive/features/FEAT-002-….md`. Its id never
  changes.
- **`spec archive <id>...`** sets frontmatter `archived: <date>` and moves
  the file. Only specs whose status is in `done_statuses` (default
  deprecated, rejected) can be archived, plus ideas whose status is
  `discarded` (FEAT-008); anything else needs `--force`. Archiving an
  implemented spec explains that it's living documentation and should be
  updated or deprecated instead.
- **`spec unarchive <id>...`** removes `archived:` and moves the file back.
- **Loading:** the store still loads archived specs, so ids stay unique,
  `nextID` never reuses an archived id, and `depends_on` references to them
  still resolve. A spec counts as archived when its path is under
  `specs/archive/`.
- **Excluded from:** the board (even with `?all=1`), the inbox (FEAT-008),
  `spec apply` (naming an archived id is an error suggesting `unarchive`),
  `spec status` counts (it prints a single "archived: N" line instead), and
  `spec roadmap`.
- **Still included in:**
  - `spec search`, with results marked `[archived]`
  - `spec trace`, where archived features still count as implementing a
    REQ and are marked `(archived)`
  - `spec validate`
  - spec pages in the web UI, reached by URL or a new `/archive` list page
    linked from the board footer
- **Validate:** the directory check accepts `specs/archive/<type dir>/` for
  each type.
- Moves use `os.Rename` within the same filesystem, after the frontmatter
  update is written with `writeFileAtomic`.

## Acceptance Criteria

- [x] `spec archive FEAT-002` moves the file to `specs/archive/features/` and sets `archived:`
- [x] Archiving a non-done spec fails without `--force`; deprecated/rejected specs and discarded ideas can be archived
- [x] Archiving an implemented spec fails without `--force`, explaining it's living documentation
- [x] `spec unarchive` restores the original location and removes `archived:`
- [x] Archived specs don't appear on the board, even with `?all=1`
- [x] `/archive` lists archived specs with links to their pages
- [x] `spec apply <archived id>` errors and suggests `unarchive`
- [x] `spec status` shows an "archived: N" line and excludes archived specs from other counts
- [x] `spec search` marks archived results; `spec trace` counts archived features as coverage
- [x] `nextID` never reuses an archived id
- [x] `spec validate` accepts archived specs in `specs/archive/<type dir>/`
- [x] Tests cover archive, unarchive, `--force`, and each command's handling
- [x] Docs updated (help text, README, SKILL.md, web help)
