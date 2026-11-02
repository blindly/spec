---
covers: [commands.go, config.go, web.go, agent.go, archive.go, help.go, main.go, templates.go, system.go]
created: 2026-11-01
id: FEAT-006
status: implemented
title: Board shows active work by default
type: feature
verified: 8bb4cbfd3650920472fae258012b1050c1712619
---

# Board shows active work by default

## Summary

The web board (at `/board`) hides specs that are no longer valid
(deprecated, rejected) by default, behind a toggle, so the board shows what's
being worked on. Implemented specs aren't hidden either: they're the living
description of the system (ADR-003) and have their own view, `/system`
(FEAT-011), so the board holds only work in progress.

## Motivation

Deprecated and rejected specs no longer describe the system, and they push
valid specs out of view. Implemented specs were originally hidden too, but
that made valid documentation disappear after implementation (see ADR-003
and FEAT-010), so they're now treated as live.

## Requirements

Addresses: REQ-011

## Design

- **Config key `done_statuses`**, default `[deprecated, rejected]`: the
  statuses that mean "no longer valid". It's configurable because
  `statuses:` is too.
- **Board default:** columns for done statuses are hidden. Below the board,
  a line reads "N finished spec(s) hidden — show". It links to
  `/board?all=1`, which shows every column, and that view has a "hide
  finished" link back.
- **Empty columns** stay hidden in both views.
- **`spec status`** shows counts for every status, but lists done statuses
  under a "finished" heading, after active and implemented ones.
- **No change** to `spec apply`, `validate`, `trace`, or `roadmap`.

## Acceptance Criteria

- [x] The board hides columns for statuses in `done_statuses` (default: deprecated, rejected)
- [x] Implemented specs stay visible by default (in `/system`, FEAT-011)
- [x] The board shows how many finished specs are hidden, with a link to `?all=1`
- [x] The board with `?all=1` shows every non-empty column, with a link back
- [x] `done_statuses` in config changes which columns are hidden
- [x] `spec status` groups done statuses under "finished"
- [x] Web and unit tests cover the default, `?all=1`, and a custom `done_statuses`
- [x] README config section documents `done_statuses`
