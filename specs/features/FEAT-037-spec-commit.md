---
covers: [commit.go, commit_test.go, main.go, help.go, completion.go, README.md, skills/spec/SKILL.md]
created: 2026-11-23
id: FEAT-037
status: implemented
title: spec commit — staging, message, and spec attestation
type: feature
---

# spec commit — staging, message, and spec attestation

## Summary

`spec commit [-m msg] [--exempt reason] [--agent]` stages everything
and commits — a thin wrapper that owns the spec-coverage contract the
pre-push hook enforces, so callers (humans, `tasks.py`, agents) don't
re-implement the gate. Messages use conventional-commit style.

## Motivation

FEAT-036's hook made "behavior ships with its spec" mechanical, which
pushed the *attestation* question upstream: the attestation has to land
in the commit message. Putting that logic in each caller (tasks.py,
shell aliases, agents) forks the rule — the first attempt did exactly
that. `spec commit` keeps one implementation in the binary that
generates the hook.

## Requirements

Addresses: REQ-014

## Design

- `git add -A`, then commit with `-m`, an agent-written message
  (`--agent`, FEAT-038), or a `chore:`-prefixed diff summary
  (`chore: update N files (+a -d): f, g, +k more`).
- **Gate** (same rule as the hook): spec repo, at least one staged
  file outside `specs/`/`.spec/`, and no `specs/` file staged → the
  commit needs attestation, which is any of:
  - a `Spec-Exempt: <reason>` trailer — explicit, wins over anything
    else (`--exempt` appends it);
  - a non-`feat` conventional-commit subject (`fix:`, `docs:`,
    `test:`, `chore:`, `refactor:`, `perf:`, `style:`, `ci:`,
    `build:`, `revert:`) — the routine spec-neutral categories;
  - anything else (`feat:`, `type!:`, bare text) → commits anyway with
    a stderr note that the push will fail the hook — the hook, not
    the commit step, is the enforcement boundary, and spec does not
    rubber-stamp a message it didn't attest.
- Nothing to commit prints `nothing to commit` and exits 0.
- Requires a spec repo (`mustStore`); callers outside spec repos use
  plain `git commit` — matching how tasks.py delegates only in
  spec-managed dirs.

## Acceptance Criteria

- [x] `spec commit` stages all changes and commits with `-m`, an
  agent message (`--agent`), or a `chore:`-prefixed summary
- [x] Uncovered commits pass silently when the message already
  attests — `Spec-Exempt:` trailer or non-`feat:` prefix; `--exempt`
  appends the trailer
- [x] An uncovered commit whose message attests nothing (bare or
  `feat:`) warns that the push will fail — and still commits
- [x] Tasks-style callers can delegate wholesale; documented in help,
  completion, README, and the spec skill
- [x] Tests cover the trailer, prefix, warning, and nothing-to-commit
  paths against a real git repo
