---
covers: [help.go, system.go, web.go]
created: 2026-11-02
depends_on: FEAT-010
id: FEAT-011
status: implemented
title: System view for implemented specs
type: feature
verified: d23f2b5b854e38256f881e654b212674f42aa8ad
---

# System view for implemented specs

## Summary

Split the web UI into three views, one per stage of a spec's life:

| View | Shows | Answers |
|---|---|---|
| **Inbox** `/` (list) | ideas | What might we do? |
| **Board** `/board` (kanban) | work in progress | What are we working on? |
| **System** `/system` (list) | specs that describe the system today | What does the system do? |

Implemented specs move from the board to the system view, so the board stays
a short list of work in progress and the system view grows into the living
documentation of the system (ADR-003).

## Motivation

After FEAT-010, implemented specs stay visible on the board, which makes the
board grow with every shipped feature and mixes "what we're doing" with "what
exists". They're different questions, and each deserves its own view.

## Requirements

Addresses: REQ-011

## Design

- **System rule** (`inSystem`): a non-archived spec is "in the system" when it
  describes the current system:
  - feature or project with status `implemented`
  - adr with status `accepted` (a standing decision; ADRs are never
    "implemented")
  - imported doc whose status isn't in `done_statuses`
- **Board:** shows every non-archived, non-idea spec that isn't in the
  system and isn't a done status (or is a done status, with `?all=1`). So:
  draft/proposed/accepted features and projects, and draft/proposed ADRs.
  `?all=1` reveals deprecated/rejected only. Below the board: "N specs in
  the system — view system".
- **System page** `/system`: one section per type, in order: Project,
  Features, Decisions (ADRs), Imported docs. Empty sections are omitted. Each
  row: id (link), title, last changed, depends_on. "Last changed" comes from
  the repo's git history (newest commit date per file), falling back to
  the file's modification time outside git. With nothing in the system:
  "Nothing in the system yet — specs appear here once implemented (ADRs once
  accepted)."
- **Nav:** inbox · board · system · new spec · readme · agents · help.
- **No CLI changes.** `spec status` already lists implemented with active
  statuses.
- Update the help page, README, and SKILL.md, and FEAT-006 and FEAT-008
  where they describe the board and nav.

## Acceptance Criteria

- [x] The board excludes implemented features/projects and accepted ADRs; it shows draft/proposed/accepted features and draft/proposed ADRs
- [x] The board's `?all=1` reveals deprecated/rejected specs but never system specs
- [x] The board links to `/system` with the number of system specs
- [x] `/system` lists implemented features and projects, accepted ADRs, and imported docs, grouped by type in the order Project, Features, Decisions, Imported docs
- [x] `/system` excludes ideas, archived specs, work in progress, and deprecated/rejected specs
- [x] Each `/system` row shows id link, title, last-changed date, and depends_on
- [x] Last-changed uses git commit dates when available, else file modification time
- [x] Nav reads inbox · board · system · new spec · readme · agents · help
- [x] Tests cover the board/system split, ADRs, imported docs, the date fallback, and nav
- [x] Docs and affected specs (FEAT-006, FEAT-008) updated
