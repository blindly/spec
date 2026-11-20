---
covers: [skill.go, help.go, spec_test.go, skill_test.go]
created: 2026-11-18
id: FEAT-030
status: implemented
title: Linked skill installs
type: feature
---

# Linked skill installs

## Summary

`spec skill sync` installs skills as symlinks to their resolved source
instead of copying files: repo skills (`repo` kind) get relative links
inside the repo, library skills get absolute links to
`~/.config/spec/skills/`. Only the embedded built-in `spec` skill — which
has no source directory — is still materialized as a file copy. Skill
content consumers (agent harnesses reading `.claude/skills/`,
`.devin/skills/`) therefore always see the current source without a
manual `sync`.

## Motivation

Copied installs are snapshots: every library edit silently ages every
installed copy until someone runs `spec skill sync`, and the harness
keeps acting on stale guidance. Links make "installed" mean "resolved
live" — for a global library skill, `~/.claude/skills/<name>` →
`~/.config/spec/skills/<name>` stays current forever; for a repo skill,
`.claude/skills/<name>` → `../../.spec/skills/<name>` survives repo
moves and is still per-machine (targets are gitignored).

## Requirements

Addresses: REQ-016, REQ-017

## Design

- Install per resolved `skillSource.kind`:
  - `repo` → symlink `<target>/<name>` → relative path to
    `.spec/skills/<name>` (e.g. `../../.spec/skills/x` from
    `.claude/skills/`), so a repo move doesn't break it.
  - `library` → symlink `<target>/<name>` → absolute
    `librarySkillDir(name)` (repo and `--global` installs alike).
  - `built-in` (`dir == ""`) → unchanged file copy; the FEAT-025
    freshness check still applies to it. This only covers the embedded
    fallback — when `spec` resolves via `.spec/skills/spec` or a library
    `spec/` dir it links like any repo/library skill.
- `check`/`skillStatusLine` inspect `os.Lstat`: a correct link reports
  `current`; a missing entry `missing`; a real directory where a link
  is expected reports `copied` — sync upgrades it in place (contents
  equal → replace with link; differing → `stale`, needs `--force`,
  same clobber rule as today).
- A link failure (Windows privileges, cross-device targets) falls back
  to the copy path with a note — sync never fails over linking.
- `rm`/`unuse`/stale-cleanup remove the link itself (`os.RemoveAll` on
  a symlink removes the link, not the target); foreign dirs are still
  reported, not touched.
- When resolution changes (e.g. `adopt` forks library → repo), sync
  re-points the link — an existing link to a different source is
  replaced, not reported stale.

## Acceptance Criteria

- [x] Repo skills install as relative symlinks into the repo target dirs; content reads through the link live
- [x] Library skills install as absolute symlinks to the library, repo and --global alike
- [x] Built-in spec skill still installs as a copy and keeps content-staleness checks
- [x] Editing the library source changes what `.claude/skills/` serves without running sync
- [x] check reports missing/link/copied/stale correctly; sync upgrades equal-content copies to links, protects differing ones
- [x] Link failure falls back to copy, not an error
- [x] adopt/use/unuse/rm move links correctly; RemoveAll never touches the source dir
- [x] Tests cover link creation, live-read, legacy-copy upgrade, and clobber protection (link-failure fallback has no portable test — symlink can't be made to fail on a normal local fs)
