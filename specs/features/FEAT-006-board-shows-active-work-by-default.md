---
created: 2026-11-01
id: FEAT-006
status: proposed
title: Board shows active work by default
type: feature
---

# Board shows active work by default

## Summary

The web board shows every spec, so it gets longer with each finished
feature. This feature hides finished specs (implemented, deprecated,
rejected) from the board by default, behind a toggle, so the board stays
focused on what still needs attention.

## Motivation

Finished specs are mostly history. Once there are dozens of them, they push
the draft/proposed/accepted work, which is what you actually act on, out of
view.

## Requirements

Addresses: REQ-011

## Design

- **New config key `done_statuses`**, default
  `[implemented, deprecated, rejected]`. It's configurable because
  `statuses:` is too.
- **Board default:** columns for done statuses are hidden. Below the board,
  a line reads "N finished specs hidden — show". It links to the board with
  `?all=1`, which shows every column, and that view has a "hide finished"
  link back. (The board is at `/` today; FEAT-008 moves it to `/board`.)
- **Empty columns** stay hidden in both views, as today.
- **`spec status`** keeps showing counts for every status, but lists done
  statuses after a blank line labelled "finished", so active work comes
  first.
- **No change** to `spec apply`, `validate`, `trace`, or `roadmap`.

## Acceptance Criteria

- [ ] The board hides columns for statuses in `done_statuses`
- [ ] The board shows how many finished specs are hidden, with a link to `?all=1`
- [ ] The board with `?all=1` shows every non-empty column, with a link back
- [ ] `done_statuses` in config changes which columns are hidden
- [ ] `spec status` groups done statuses under "finished"
- [ ] Web and unit tests cover the default, `?all=1`, and a custom `done_statuses`
- [ ] README config section documents `done_statuses`
