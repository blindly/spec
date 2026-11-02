---
covers: [agent.go, archive.go, config.go, help.go, main.go, templates.go]
created: 2026-11-02
depends_on: [FEAT-006, FEAT-007]
id: FEAT-010
status: implemented
title: Keep implemented specs visible and current
type: feature
verified: aeb547a901c588465f7629d37525ae76f8c9a61f
---

# Keep implemented specs visible and current

## Summary

Implement ADR-003. Implemented specs stay visible: they're no longer hidden
on the board or archivable without `--force`. Agents applying a spec must
update other specs whose described behavior they change, so implemented
specs stay accurate instead of aging out.

## Motivation

See ADR-003. Hiding implemented specs, together with a prompt that forbade
fixing other specs, made valid documentation disappear and drift.

## Requirements

Addresses: REQ-011, REQ-012, REQ-014

## Design

- **`done_statuses` default** changes from
  `[implemented, deprecated, rejected]` to `[deprecated, rejected]`, so
  implemented specs are never hidden as "finished". They're shown in the
  system view (`/system`, FEAT-011) rather than on the board, which holds
  only work in progress; the board's `?all=1` still reveals deprecated and
  rejected ones. Repos that set `done_statuses` explicitly are unaffected.
- **`spec status`** follows the same split: implemented counts as active,
  and only deprecated and rejected appear under "finished".
- **Archive eligibility:** unchanged in code (done statuses plus discarded
  ideas), so with the new default implemented specs need `--force`. The
  error for an implemented spec explains that implemented specs are living
  documentation and should be updated or deprecated rather than archived.
- **Apply prompt:** replace "do not edit the spec files under specs/" with:
  - if your change alters behavior described by another spec, update that
    spec's text in the same change so it stays accurate
  - don't change other specs' status, ids, or types
  - for accepted ADRs, don't edit the original sections; add a dated entry
    under `## Corrections` instead (see ADR-003's Corrections)
  - set this spec's status to `implemented` when done (unchanged)
- **Prompt spec list** (`repoContext`): mark archived specs, e.g.
  `FEAT-002 (feature, deprecated, archived — history, may be outdated)`.
- **Docs:** the help page, README, config templates, SKILL.md, and the
  seeded AGENTS.md describe implemented specs as living documentation and
  archive as being for invalid specs.
- **This repo:** update FEAT-006 and FEAT-007, whose text describes the old
  behavior.

## Acceptance Criteria

- [x] The default `done_statuses` is `[deprecated, rejected]`
- [x] Implemented specs are visible by default (in `/system`, FEAT-011); deprecated and rejected ones are hidden
- [x] `spec status` lists implemented as active, with only deprecated and rejected under "finished"
- [x] `spec archive` on an implemented spec fails without `--force`, with an explanation; deprecated and rejected specs archive without `--force`
- [x] The apply prompt tells agents to update other specs they affect, and not to change their status
- [x] The prompt's spec list marks archived specs
- [x] Docs updated (help, README, config templates, SKILL.md, AGENTS template)
- [x] FEAT-006 and FEAT-007 describe the new behavior
- [x] Tests updated and passing
