---
covers: [commands.go, skill.go, update.go, completion.go, spec_test.go, skill_test.go, completion_test.go, help.go, README.md]
created: 2026-11-20
id: FEAT-032
status: implemented
title: Self-maintaining generated files
type: feature
---

# Self-maintaining generated files

## Summary

Close the remaining "generated artifact only refreshes when someone
remembers" leaks: `AGENTS.md`/`CLAUDE.md` are seeded only by `spec init`
(repos inited earlier never get the harness pointer), installed
completion scripts age with every binary update, and a `skill_targets:`
dir outside the repo gets a fragile `../`-chain symlink.

## Motivation

The harness learns "this repo is spec-managed — use the specs/plan
workflow" from `AGENTS.md`/`CLAUDE.md` at session start. Repos without
those files walk in blind; repos with stale ones keep old guidance.
Same class of gap as `.gitignore` seeding, which moved into the
syncSkills funnel for exactly this reason. Completion scripts and
out-of-repo link targets are smaller instances of the same leak.

## Requirements

Addresses: REQ-016, REQ-017

## Design

- `ensureAgentPointers(root)` — extracted from `cmdInit` (same
  create-if-missing semantics), additionally called from `syncSkills`
  under the same `isDir(<root>/.spec)` guard as `ensureGitignore`.
  Every repo-level sync seeds the pointers; **never overwrites** —
  hand edits are preserved. Repo-free commands unaffected.
- `spec update` — after the skill resync, regenerates each completion
  script that is already installed (fish/bash/zsh user dirs); files
  that don't exist aren't created. Best-effort notes, not errors.
- `skillLinkTarget` — repo skills keep relative links only when the
  install target is inside the repo; targets outside root get an
  absolute link (a `..` chain to an external dir breaks if either
  side moves).

## Acceptance Criteria

- [x] `spec skill sync` creates missing AGENTS.md/CLAUDE.md in existing repos, preserves hand-edited ones, and repo-free commands create nothing
- [x] `spec update` regenerates already-installed completion scripts via the new binary
- [x] `skill_targets:` outside the repo gets an absolute symlink; in-repo targets stay relative
- [x] Tests cover pointer seeding/preservation and the external-target absolute link
