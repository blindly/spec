---
covers: [update.go, facts.go, skill.go, spec_test.go, skill_test.go, help.go, README.md]
created: 2026-11-20
id: FEAT-031
status: implemented
title: Post-update resync and skill install audit
type: feature
---

# Post-update resync and skill install audit

## Summary

Two holes remain where a harness reads stale skill guidance with no
warning: `spec update` swaps the binary and leaves every embedded-skill
install behind (the old process can't write the new skill), and
`spec check` audits facts/reminders/drift but never looks at skill
installs, so a stale or missing install passes `spec verify` silently.
This spec closes both: update re-syncs installs through the new binary,
and check audits install status like `spec skill` does.

## Motivation

FEAT-030 made repo/library installs live links — the embedded built-in
`spec` skill is the only copy left, and it carries the specs/plan
workflow guidance harnesses keep forgetting. A binary update silently
ages that copy in every repo and every user-level dir until someone
happens to run `spec skill sync`. Meanwhile `spec check` — the gate
`verify:` runs — reports nothing wrong.

## Requirements

Addresses: REQ-016, REQ-017

## Design

- `spec update` — after the binary is replaced, re-exec **the new
  binary** (the running process still holds the old embedded skill):
  - `spec skill sync` when cwd is inside a spec-managed repo
    (`mustStore` succeeds)
  - `spec skill sync --global` always, covering the user-level dirs
  - Best-effort: the update already succeeded, so a sync that exits
    non-zero (e.g. clobber-protected stale copies) prints a note, not
    an error.
- `spec check` — new audit over the resolved skill set, same model as
  `spec skill`:
  - unresolvable `skills:` entry (orphaned) → FAIL
  - `missing`, `stale`, `relink` install → FAIL, with the same
    `sync`/`sync --force` hint `spec skill` prints
  - `copied` (equal-content copy awaiting link upgrade) → WARN —
    content is right, just not linked
  - `current` → silent

## Acceptance Criteria

- [x] `spec update` runs `spec skill sync` (repo, when in one) and `spec skill sync --global` via the new binary; failures are notes, not errors
- [x] `spec check` fails on missing/stale/orphaned installs with a sync hint, warns on copied, passes when all current
- [x] Tests cover the check audit statuses (FAIL and WARN cases)
