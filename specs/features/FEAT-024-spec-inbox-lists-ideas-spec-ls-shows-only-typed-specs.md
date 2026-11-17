---
covers: [inbox.go, commands.go, main.go, help.go, spec_test.go, inbox_test.go]
created: 2026-11-16
id: FEAT-024
status: implemented
title: spec inbox lists ideas; spec ls shows only typed specs
type: feature
verified: 9b9fd8602ea058213c1a7c43e488902638685610
---

# spec inbox lists ideas; spec ls shows only typed specs

## Summary

`spec inbox [--all] [--paths]` lists inbox ideas in the same column format
as `spec ls` — id, status, suggested type when set, title, path — giving
ideas their own view. `spec ls` stops listing ideas entirely, so it shows
only typed specs (project, feature, adr), matching the inbox/board split
the web UI already enforces.

## Motivation

`spec ls` was the only CLI listing that showed ideas — mixed into the spec
list with type `idea`, discarded ones included — while the web UI keeps
inbox and board strictly separate (FEAT-008, FEAT-013) and `spec roadmap`
excludes ideas until promoted. The CLI had no equivalent of the web
inbox page, and no way to hide raw/discarded ideas from the spec listing.

## Requirements

Addresses: REQ-013, REQ-011

## Design

- `cmdInbox` iterates `Store.Specs()`, keeps only `IsIdea()` entries, and
  writes the same tab-aligned columns as `cmdList`: ID, STATUS, TITLE,
  PATH — plus a `→ <type>` marker in a column after the status when
  `suggested_type` is set (a refined idea's next step).
- Consistent with the board and `spec ls`: discarded ideas are hidden by
  default; `--all` shows them, and also unhides archived ideas (marked
  `(archived)` after the status, same as `spec ls --all`). `--paths`
  prints paths only, for scripting.
- `cmdList` skips `IsIdea()` entries unconditionally — `ls` means typed
  specs; `--all` still means "include archived", it does not bring ideas
  back.
- `spec inbox` is registered in `commands`, usage, and `help.go`. An
  empty inbox prints a hint to capture with `spec idea <title>`.

## Acceptance Criteria

- [x] `spec inbox` lists non-discarded, non-archived ideas with id,
  status, suggested type (when set), title, and path
- [x] `spec inbox --all` also shows discarded and archived ideas
- [x] `spec inbox --paths` prints idea paths only
- [x] `spec ls` and `spec ls --all` list no ideas, only typed specs
- [x] `spec inbox` on an empty inbox prints a capture hint
- [x] Unit tests cover both commands' filtering
