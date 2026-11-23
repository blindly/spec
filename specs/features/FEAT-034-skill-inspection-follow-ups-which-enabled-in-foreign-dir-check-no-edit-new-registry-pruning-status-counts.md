---
covers: [skill.go, commands.go, help.go, completion.go, skill_test.go, README.md]
created: 2026-11-23
id: FEAT-034
status: implemented
title: Skill inspection follow-ups
type: feature
---

# Skill inspection follow-ups

## Summary

Five small follow-ups to FEAT-033: `spec skill which` now reports which
registered repos (and config level) enable a skill; `spec skill check`
reports foreign dirs sitting in the install targets; `spec skill new`
skips the editor with `--no-edit` or when stdin isn't a terminal; the
repo registry prunes dead entries when it is next written; and
`spec status` reports the not-enabled library count alongside the
managed/local counts.

## Motivation

FEAT-033 added the registry and lint machinery; these close the gaps it
exposed: `which` answers "where from" but not "who uses it", `check`
can't see leftover dirs that resolution forgot, `new` can't be scripted
(it always execs `$EDITOR`), the registry file can only grow, and
`status` under-reports the library.

## Requirements

Addresses: REQ-017

## Design

- **`which` "enabled in".** After the resolution line, `spec skill
  which` appends `enabled in: …` listing `this repo` (repo `skills:`),
  `global skills:` (global list), and the registered repos enabling the
  name — shown for every resolved or readable skill, so an available
  entry can show where it is already in use elsewhere.
- **Foreign-dir check.** `spec skill` / `spec skill check` scan each
  install target for entries not in the resolved set and note them on
  stderr (`note: foreign skill dir <dst>`) — the same vocabulary `sync`
  uses. Advisory only: it never affects the `bad` count or exit code,
  matching how `sync` treats foreign dirs (removes generated leftovers,
  reports the rest).
- **`--no-edit` on `new`.** `spec skill new <name> [--no-edit]` writes
  the scaffold and skips the editor; the editor is also skipped
  automatically when stdin is not a terminal. `--library` and the
  post-create sync behave as before. The flag is listed in the skill
  command's flags and completion.
- **Registry pruning.** `registerRepo` filters dead entries (no `.spec/`
  dir) out of `~/.config/spec/repos` when it writes, so churn from
  deleted repos can't accumulate. `registeredRepos` keeps its
  read-time filter as a second line of defense.
- **Status count.** `spec status` prints
  `Skills: N managed, M local, K available` — `available` being the
  library entries not enabled or present in the repo
  (`availableSkills`).

## Acceptance Criteria

- [x] `spec skill which` lists `this repo`, `global skills:`, and
  registered repos enabling the skill under `enabled in:`
- [x] `spec skill`/`check` notes foreign dirs in install targets on
  stderr without failing; `sync`'s behavior is unchanged
- [x] `spec skill new --no-edit` (and non-terminal stdin) writes the
  scaffold without exec'ing an editor; `new` still syncs and
  `--library` still works
- [x] `registerRepo` writes a pruned registry (dead entries dropped)
- [x] `spec status` shows the available-skill count
- [x] Help, completion, and README cover the new flag and behavior
- [x] Tests cover each item
