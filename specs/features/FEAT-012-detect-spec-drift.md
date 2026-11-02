---
covers: [commands.go, drift.go, driftcmd.go, facts.go, help.go, main.go, system.go, templates.go, web.go]
created: 2026-11-02
depends_on: [FEAT-010, FEAT-011]
id: FEAT-012
status: implemented
title: Detect spec drift
type: feature
verified: 79c7ddfbcf19372fec6714881a1f9d24ce29cff0
---

# Detect spec drift

## Summary

Detect when a spec in the system view may no longer describe the system:
code it covers changed after the spec was last updated. Each spec records
the files it covers; `spec drift` lists specs whose covered files changed
since their checkpoint, and `spec drift --review` asks the agent whether
each one is actually stale and proposes an update. The same warnings appear
in `spec check`, the system view, and around `spec apply`.

## Motivation

Implemented specs are living documentation (ADR-003), and the apply prompt
tells agents to keep them current (FEAT-010). That instruction alone isn't
enough: in the session that introduced it, FEAT-010, FEAT-009, and ADR-003
all went stale while the rule was being followed. A probe of this repo's git
history flagged four specs, three of them really stale. Drift has to be
detected by the tool, not left to discipline.

## Requirements

Addresses: REQ-014

## Design

### Coverage

- Frontmatter `covers: [path, ...]`, relative to the repo root. An entry
  ending in `/` matches a directory; entries with `*`, `?`, or `[` are globs;
  anything else is an exact path.
- **Recorded by `spec apply`:** spec snapshots the working tree (HEAD plus a
  hash of each dirty file) before the agent runs and diffs it afterwards. Code
  files the agent changed are merged into the applied spec's `covers` when
  the outcome is implemented.
- **`spec covers --infer [id...]`** fills `covers` for system specs that have
  none, from the code files changed in the commits that touched each spec.
- **`spec covers [id...]`** lists coverage.
- "Code file" excludes `specs/`, `.spec/`, dot-directories, docs (`.md`,
  `.markdown`, `.rst`, `.txt`), lockfiles, `VERSION`/`LICENSE`, and tests
  (`_test.`, `.test.`, `.spec.`, `test_*`, `test/`, `tests/`, `__tests__/`).

### Detection

- Only specs in the system (FEAT-011) are checked; work in progress can't
  drift yet.
- A spec's **checkpoint** is the newest of: the last commit touching the spec
  file, and the commit in its `verified:` stamp. A spec whose file has
  uncommitted changes is being edited and counts as in sync.
- Changes to `covers:` alone are bookkeeping, not an update to what the
  spec says: they don't move the checkpoint, whether uncommitted or
  committed. (Found by running `spec covers --infer` on this repo, which
  otherwise hid every flag.)
- A spec **may be stale** when any covered file changed in a commit newer
  than its checkpoint, or has uncommitted changes.
- Specs with no `covers` are reported separately ("no coverage — run
  `spec covers --infer`").
- Requires git; without it, drift reports that it's unavailable and other
  commands stay silent.
- **Cost:** a fixed handful of git commands per check, independent of the
  number of specs or commits. History is read only for `specs/` and covered
  paths, and one `git log -p -- specs/` decides which spec commits were
  covers-only. `spec web` caches the result until HEAD or the working tree
  changes. `SPEC_TRACE=1` prints each git call with its duration.

### Commands

- **`spec drift [id...]`** lists possibly-stale specs with the changed files
  and the commits involved, newest first. It never fails the build.
- **`spec drift --review [id...] [--yes]`** sends each flagged spec, plus
  `git diff <checkpoint> -- <covered files>`, to the agent. The agent
  replies `VERDICT: accurate|stale`, a `REASON:` line, and, if stale, the
  full updated body.
  - **accurate:** stamp `verified:`.
  - **stale:** show the reason and a diff of the proposed body. Apply it on
    `y` (or with `--yes`); otherwise leave the spec unchanged. Applying also
    stamps `verified:`.
- **`spec verified <id>...`** stamps `verified: <HEAD commit>` (the spec has
  been checked and is accurate).
- **`spec check`** prints a `WARN` line for each possibly-stale spec. It
  doesn't fail.

### Around apply

- The apply prompt lists specs that already may be stale, with their covered
  files, so the agent can bring them up to date if its work touches them.
- After the agent runs, spec warns about other system specs that cover files
  this run changed, if their spec files weren't updated in the same run.

### Web

- `/system` rows show a "may be stale" badge, with the changed files in its
  tooltip.
- A spec's page shows the changed files and commits since its checkpoint.

## Acceptance Criteria

- [x] `covers` entries match exact paths, directories (trailing `/`), and globs
- [x] `spec apply` records the code files the agent changed in the applied spec's `covers`
- [x] `spec covers --infer` fills `covers` from the commits that touched each spec, excluding docs, specs, and tests
- [x] A system spec is flagged when a covered file changes in a later commit, or has uncommitted changes
- [x] A spec isn't flagged when it was updated in the same or a later commit, or has uncommitted edits
- [x] `spec verified` clears the flag until covered files change again
- [x] Only system specs are checked; specs without `covers` are reported as having no coverage
- [x] `spec drift --review`: an accurate verdict stamps `verified`; a stale verdict shows a diff and applies it with confirmation or `--yes`
- [x] `spec check` warns about possibly-stale specs without failing
- [x] The apply prompt lists possibly-stale specs, and after applying, spec warns about other specs covering files the run changed
- [x] `/system` shows a "may be stale" badge; the spec page lists changed files and commits
- [x] Without git, drift reports that it's unavailable and nothing else breaks
- [x] Tests use real git repos with fixed commit dates, and fake agents for apply and review
- [x] Docs updated (help, README, SKILL.md, AGENTS template)
