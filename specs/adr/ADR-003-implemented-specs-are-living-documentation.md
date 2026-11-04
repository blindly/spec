---
covers: [agent.go, archive.go, config.go, help.go, main.go, templates.go]
created: 2026-11-02
id: ADR-003
status: accepted
title: Implemented specs are living documentation
type: adr
verified: 70ccef5a8bf0436583d59a2cf4fa799a42e8e7c0
---

# Implemented specs are living documentation

## Context

FEAT-006 and FEAT-007 treated `implemented` as "finished": implemented specs
were hidden on the board by default and could be archived out of every view.
That's backwards. An implemented spec that still describes the system is its
documentation, the most useful kind of spec there is. Hiding it makes specs
"age out" after implementation, which is the main failure of spec-driven
development.

The real cause of aging is drift, not visibility. A spec stops being relevant
when later work changes the behavior it describes and nobody updates it. The
apply prompt caused this: it told agents not to edit any spec except their
own status, so an agent that changed described behavior couldn't fix the spec
describing it. (Example: FEAT-008 moved the board to `/board`, but FEAT-006
kept saying `/`.)

Alternatives considered:

- **Keep hiding implemented specs, and rely on archive and search to find
  them.** This keeps the board short, but valid documentation leaves people's
  view and goes stale unnoticed.
- **Remove archive entirely.** Simple, but deprecated and rejected specs and
  discarded ideas really are history and do clutter views.
- **Treat implemented specs as live, keep archive for invalid specs only, and
  make agents keep affected specs current.** Chosen.

## Decision

- `implemented` means **live**: the spec is valid and describes the current
  system. Implemented specs stay visible on the board and in `spec status`.
- `done_statuses` (hidden on the board by default) defaults to
  `[deprecated, rejected]`: specs that are no longer valid.
- `spec archive` accepts only invalid specs (done statuses and discarded
  ideas) without `--force`. Implemented specs need `--force`, and the error
  explains why.
- The apply prompt requires agents to update any other spec whose described
  behavior their change alters, in the same change, without changing that
  spec's status.

## Consequences

- The board gets longer as features ship. That's intended: it is the map of
  what the system does. If it becomes too long, the answer is better
  organization (filters, grouping), not hiding valid specs.
- Specs edited by agents need review like code, since agents now touch specs
  beyond the one being applied.
- Staleness can still happen through human edits or out-of-band changes.
  Detecting it automatically (linking specs to code or tests) is the next
  step, and isn't part of this decision.
- Partial replacement (one spec superseding part of another) has no explicit
  marker yet; agents update the older spec's text instead.

## Corrections

- **2026-11-02:** "Implemented specs stay visible on the board" — since
  FEAT-011, implemented specs (and accepted ADRs) are shown in the system
  view (`/system`), and the board holds only work in progress. The decision
  stands: implemented specs are never hidden as finished. Found by
  `spec drift` (FEAT-012).
