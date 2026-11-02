---
created: 2026-11-01
depends_on: [FEAT-006, FEAT-008]
id: FEAT-007
status: proposed
title: Archive finished specs
type: feature
---

# Archive finished specs

## Summary

Add `spec archive <id>...`, which moves finished specs into
`specs/archive/`. Archived specs disappear from the board, `apply`,
`status`, and `roadmap`, but stay in the repo, so they remain searchable,
referenceable, and in git history. `spec unarchive` moves them back.

## Motivation

Hiding finished specs on the board (FEAT-006) fixes day-to-day clutter, but
the files keep piling up under `specs/features/`, and every command keeps
loading them as live work. Archiving is the long-term answer: once an
area is stable and folded into the project spec's requirements, its
feature specs can be retired.

## Requirements

Addresses: REQ-012, REQ-011

## Design

- **Layout:** an archived spec moves to `specs/archive/<type dir>/` with the
  same filename, e.g. `specs/archive/features/FEAT-002-….md`. Its id never
  changes.
- **`spec archive <id>...`** sets frontmatter `archived: <date>` and moves
  the file. Only specs whose status is in `done_statuses` can be archived,
  plus ideas whose status is `discarded` (FEAT-008); anything else needs
  `--force`.
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

- [ ] `spec archive FEAT-002` moves the file to `specs/archive/features/` and sets `archived:`
- [ ] Archiving a non-done spec fails without `--force`; discarded ideas can be archived
- [ ] `spec unarchive` restores the original location and removes `archived:`
- [ ] Archived specs don't appear on the board, even with `?all=1`
- [ ] `/archive` lists archived specs with links to their pages
- [ ] `spec apply <archived id>` errors and suggests `unarchive`
- [ ] `spec status` shows an "archived: N" line and excludes archived specs from other counts
- [ ] `spec search` marks archived results; `spec trace` counts archived features as coverage
- [ ] `nextID` never reuses an archived id
- [ ] `spec validate` accepts archived specs in `specs/archive/<type dir>/`
- [ ] Tests cover archive, unarchive, `--force`, and each command's handling
- [ ] Docs updated (help text, README, SKILL.md, web help)
