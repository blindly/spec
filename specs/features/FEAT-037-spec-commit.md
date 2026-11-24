---
covers: [commit.go, commit_test.go, main.go, help.go, completion.go, README.md, skills/spec/SKILL.md]
created: 2026-11-23
id: FEAT-037
status: implemented
title: spec commit — staging, message, and Spec-Exempt trailer
type: feature
verified: 4c35d667ab4067bad1fa73526ec12cb495938362
---

# spec commit — staging, message, and Spec-Exempt trailer

## Summary

`spec commit [-m msg] [--exempt reason]` stages everything and commits
— a thin wrapper that owns the spec-coverage contract the pre-push
hook enforces, so callers (humans, `tasks.py`, agents) don't
re-implement the gate.

## Motivation

FEAT-036's hook made "behavior ships with its spec" mechanical, which
pushed the *attestation* question upstream: a `Spec-Exempt:` trailer
has to land in the commit message. Putting that logic in each caller
(tasks.py, shell aliases, agents) forks the rule — the first attempt
did exactly that. `spec commit` keeps one implementation in the binary
that generates the hook.

## Requirements

Addresses: REQ-014

## Design

- `git add -A`, then commit with `-m` or an auto diff-summary
  (`update N files (+a -d): f, g, +k more`).
- **Gate** (same rule as the hook): spec repo, at least one staged
  file outside `specs/`/`.spec/`, and no `specs/` file staged → the
  commit needs attestation:
  - `--exempt <reason>` → appends `Spec-Exempt: <reason>`;
  - no `-m` (the harness wrote the message) → appends
    `Spec-Exempt: spec auto-commit — no spec-covered files`;
  - `-m` without `--exempt` → commits anyway with a stderr note that
    the push will fail the hook — the hook, not the commit step, is
    the enforcement boundary.
- Nothing to commit prints `nothing to commit` and exits 0.
- Requires a spec repo (`mustStore`); callers outside spec repos use
  plain `git commit` — matching how tasks.py delegates only in
  spec-managed dirs.

## Acceptance Criteria

- [x] `spec commit` stages all changes and commits with `-m` or an
  auto summary
- [x] Spec-uncovered commits get a `Spec-Exempt:` trailer from
  `--exempt` or from auto-message attestation; covered commits get
  none
- [x] A hand-written `-m` without `--exempt` on an uncovered commit
  warns that the push will fail (and still commits)
- [x] Tasks-style callers can delegate wholesale; documented in help,
  completion, README, and the spec skill
- [x] Tests cover the trailer paths against a real git repo
