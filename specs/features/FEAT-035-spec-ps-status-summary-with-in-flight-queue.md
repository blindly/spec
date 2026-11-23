---
covers: [main.go, commands.go, help.go, completion.go, spec_test.go, README.md]
created: 2026-11-23
id: FEAT-035
status: implemented
title: spec ps — status summary with in-flight queue
type: feature
---

# spec ps — status summary with in-flight queue

## Summary

`spec ps` prints everything `spec status` does, plus an `In flight:`
section naming the specs that are not yet done — the spec equivalent
of `ps` listing running processes rather than every binary on disk.

## Motivation

`spec status` reports counts by status ("accepted 2") but not which
specs those are; `spec ls` lists every spec with no status filter.
`ps` closes that gap: one command for the summary and the work queue.

## Requirements

Addresses: REQ-011

## Design

- **`spec ps` is its own command**, not an alias: it runs the status
  report and then lists, under `In flight:`, every spec that is not
  archived, not an inbox idea, not in a `done_statuses` status, and not
  `implemented` — sorted by configured status order (unknown statuses
  last), then id. Empty queue prints `In flight: none`. Columns: id,
  type, status, title.
- `spec status` is unchanged; `ps` shares its implementation via a
  `statusReport(..., showInFlight)` helper.
- Help and completion list `ps` as a real command (its own doc block,
  not an alias mapping).

## Acceptance Criteria

- [x] `spec ps` prints the status summary followed by `In flight:`
  entries for each non-archived, non-idea spec whose status is neither
  done nor `implemented`, sorted by status order then id
- [x] `In flight: none` when the queue is empty
- [x] `spec status` output is unchanged; `spec help ps` and completion
  treat `ps` as a first-class command
- [x] README documents `ps`; tests cover both commands
