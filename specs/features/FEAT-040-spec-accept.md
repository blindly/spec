---
covers: [commands.go, main.go, help.go, completion.go, spec_test.go]
created: 2026-11-24
id: FEAT-040
status: implemented
title: spec accept — the approval transition
type: feature
depends_on: [FEAT-001]
---

# FEAT-040: spec accept — the approval transition

## Summary

`spec accept <id>...` marks specs `accepted` — the human approval step
in the lifecycle (draft/proposed → accepted → apply → implemented).
Until now the only way past `proposed` was editing frontmatter by
hand; `accepted` is the gate bare `spec apply` consumes, so it
deserves a named command.

## Requirements

- `spec accept <id>...` sets `status: accepted` on one or more named
  specs, printing the transition
- Refuses (per id, continuing past it) on missing, archived, idea,
  or already-finished (`implemented`/`done_statuses`) specs, and when
  the repo's configured statuses don't include `accepted` for the
  spec's type; `already accepted` is a no-op note, not a failure
- Non-zero exit naming failures when any id fails

## Design

Acceptance is deliberately a *human* act — `spec apply` (no ids) only
runs accepted specs, so accept is the "yes, build this" decision. An
agent refining a spec doesn't self-approve; the human reads the
refined spec and accepts it. Transitions the other direction
(accepted → draft) stay a manual frontmatter edit — demotion is rare
enough not to need a command. No `--force`: accepting an implemented
spec is a mistake, not a choice to permit.

## Acceptance Criteria

- [x] `spec accept` transitions draft/proposed specs to `accepted`
- [x] Missing, archived, idea, and finished specs are refused per-id
- [x] `accepted` missing from the type's configured statuses is refused
- [x] Registered as a first-class command (help, completion, README)
