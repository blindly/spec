---
covers: [skill.go, commands.go, main.go, help.go, spec_test.go]
created: 2026-11-16
id: FEAT-025
status: implemented
title: spec skill checks and updates installed skill copies
type: feature
verified: 297bf4b440f5344b6ed41f7bdfba1485c6870ece
---

# spec skill checks and updates installed skill copies

## Summary

`spec skill` reports whether the skill files `spec init` seeds into a repo
(`.devin/skills/spec/SKILL.md`, `.claude/skills/spec/SKILL.md`) still match
the version embedded in the running binary. `spec skill update` rewrites
stale or missing copies. `--global` applies the same check/update to the
user-level skill dirs (`~/.config/devin/skills/`, `~/.claude/skills/`).

## Motivation

`spec init` seeds the skill copies only when they don't exist — they are
snapshots of whatever binary version ran init, and neither `spec init` nor
`spec update` ever refreshes them. When the bundled skill changes (e.g. a
new workflow rule), every repo inited earlier keeps the stale copy with no
way to notice or upgrade. A freshness check plus an update path closes the
loop (REQ-016).

## Requirements

Addresses: REQ-016

## Design

- The canonical skill is `skills/spec/SKILL.md`, embedded as `specSkill`
  via `//go:embed` — every binary carries its version, so the check is
  offline and needs no version metadata.
- `spec skill` (or `spec skill check`) compares each installed location's
  bytes to the embedded copy and prints one line per location —
  `current`, `stale`, or `missing`. It exits non-zero when any checked
  location is not current, so it can gate scripts or `verify:` entries.
- `spec skill update` writes the embedded copy to every location that is
  stale or missing (creating parent dirs), and reports `current` for
  those already up to date.
- Repo mode checks `<root>/.devin/skills/spec/SKILL.md` and
  `<root>/.claude/skills/spec/SKILL.md` under `Store.Root`, not cwd.
  `--global` instead checks the user-level dirs for the two agent CLIs.
- Like `spec init`, update writes files verbatim — it does not merge
  local edits; a customized copy is "stale" and gets overwritten, which
  the command reports.

FEAT-026 generalizes this: `check`/`sync` cover the whole resolved skill
set (repo `.spec/skills/`, library `skills:` entries, built-in `spec`),
installed copies are generated into the resolved `skill_targets:` dirs,
`update` remains as a `sync --force` alias, and `--global` covers the
user-level dirs as before.

## Acceptance Criteria

- [x] `spec skill` prints per-location status (current/stale/missing)
- [x] `spec skill` exits non-zero when any repo copy is stale or missing,
  zero when all are current
- [x] `spec skill update` rewrites stale/missing copies to match the
  embedded skill and leaves current copies untouched
- [x] `--global` checks/updates the user-level skill dirs
- [x] Unit tests cover the three statuses, the exit code, and update
