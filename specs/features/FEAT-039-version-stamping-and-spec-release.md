---
covers: [version.go, commit.go, config.go, main.go, help.go, completion.go]
created: 2026-11-24
depends_on: [FEAT-037, FEAT-036]
id: FEAT-039
status: implemented
title: VERSION stamping in spec commit and spec release
type: feature
verified: 70bac3c085dfdf70c10c20bf361fd51ed63dc7e1
---

# FEAT-039: VERSION stamping in spec commit and `spec release`

## Summary

Release versioning is owned by spec itself: `spec commit` refreshes a
build-stamp `VERSION` file on every commit, and `spec release` stamps a
VERSION-only commit, pushes, and tags `v<stamp>` for the release
workflow. This replaces external task tooling (`inv bump` in a personal
taskfile — unavailable where invoke isn't wired, e.g. Windows).

## Requirements

- `spec commit` stamps VERSION when the file exists and includes it in
  the commit
- `spec release` produces a VERSION-only `chore: bump <stamp>` commit,
  pushes the branch, and pushes `v<stamp>`
- Visible failure when `release: true` is unset, on a dirty tree, on
  missing VERSION, or on an existing tag

## Design

A `VERSION` file at the repo root **opts the repo in**. The stamp is
`<major>.<yy>.<mm>-build-<ddhhmm>` — a pure function of wall clock, so
it is always monotonic and needs no human input:

- The major is read from the existing file, so editing VERSION's major
  and committing is how a major bump rides a normal commit; `spec
  release --major N` forces it.
- The major auto-increments only when the fresh stamp would sort before
  the stored one (clock regression across machines/timezones).
- `version_stamp: false` in `.spec/config.yml` opts a repo back out —
  for repos that have a VERSION file under a different scheme.
- `release: true` in the repo's `.spec/config.yml` opts into releasing —
  `spec release` refuses without it. The key is repo-only by design: a
  global `release: true` would silently enable tagging everywhere.

`spec commit` stamps before staging so the refresh rides the same
commit; on an otherwise-clean tree this yields a stamp-only commit — a
bump without a release. `spec release` requires a clean tree (commit
first) so the VERSION commit stays bookkeeping-only and auditable; the
`chore:` subject self-attests spec-neutrality under FEAT-036 — no
trailer needed. When HEAD already carries the fresh stamp (a `spec
commit` in the same minute), no VERSION commit is made — release just
tags HEAD. An existing `v<stamp>` tag errors rather than retagging; a
failed tag push says how to retry.

## Acceptance Criteria

- [x] `spec commit` stamps VERSION when the file exists (config
  `version_stamp: false` opts out); the stamp rides the same commit
- [x] `spec release` produces a VERSION-only `chore: bump <stamp>`
  commit, pushes the branch, and pushes `v<stamp>`; `--major` forces
  the major
- [x] `release` fails visibly when `release: true` is unset
  (repo-only config key), on a dirty tree, a missing VERSION file, or
  an existing tag — no silent re-tag
- [x] Stamp format `<major>.<yy>.<mm>-build-<ddhhmm>` with the
  clock-regression major bump
