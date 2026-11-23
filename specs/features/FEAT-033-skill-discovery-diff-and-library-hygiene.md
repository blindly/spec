---
covers: [skill.go, skilldiff.go, frontmatter.go, config.go, help.go, completion.go, skill_test.go, README.md]
created: 2026-11-23
id: FEAT-033
status: implemented
title: Skill discovery, diff, and library hygiene
type: feature
---

# Skill discovery, diff, and library hygiene

## Summary

The skill tooling grew a real discovery problem: `spec skill` lists
library leftovers as a bare `available` label with no hint what the
skills are for or how to enable them, "stale" is a binary verdict with
no way to see the difference, nothing validates library entries
themselves, `rm --library` silently orphans other repos, and `import`
only understands local paths. This feature makes the listing
self-describing (groups, descriptions, a status legend), adds `which`
and `diff` inspection commands, lints library entries, keeps a registry
of known spec repos so cross-repo effects can be warned about, and
extends `import` to repo roots and git URLs.

## Motivation

With a dozen skills in the library, bare names tell the user nothing —
`spec skill view` per name is the only way to browse. `--force`
decisions (`sync`, `push`, `unuse`, `rm`) are currently blind: the user
knows a copy differs but cannot see how. Library entries can drift out
of shape (missing SKILL.md, mismatched frontmatter name) with no audit.
Removing a library entry leaves every enabled repo `orphaned` — a fair
model, but the removal should warn while the information is still known.
And the library is designed to be shared via git (FEAT-026) yet `import`
cannot read a remote repo.

## Requirements

Addresses: REQ-003, REQ-017

## Design

- **List output.** `spec skill` / `spec skill check` group the output
  into an `enabled` section (the resolved set, unchanged columns:
  name, source label, per-target status — plus a trailing description
  column from each skill's SKILL.md frontmatter) and a
  `library (not enabled — 'spec skill use <name>' to enable)` section
  listing each remaining library skill with its description. A compact
  legend line explains the status vocabulary
  (`current · copied · relink · stale · missing`). Descriptions come
  from `description:` frontmatter, truncated to one line; absent ones
  show nothing (lint covers the gap). The repo-free `spec skill`
  (library listing) gains the same description column.
- **`spec skill which <name>`** prints the name's resolution: source
  kind and directory (`(embedded)` for the built-in). A name that is in
  the library but not enabled reports `library (not enabled)`. Unknown
  names error.
- **`spec skill diff <name>`** prints a unified diff between each
  installed copy and the resolved source — the "what would `--force`
  discard" question. Installs that are links to the right source
  report `current`; missing ones report `missing`. With `--library` it
  diffs `.spec/skills/<name>` against the library entry instead — the
  pre-`push` question. The diff is computed in-process (line LCS with
  common prefix/suffix trimming, 3 lines of context) — no external
  `diff` binary, keeping the stdlib-only constraint and identical
  behavior on Windows. File-level add/remove is shown for
  non-comparable files (binary or absent on one side).
- **Library lint.** `spec skill check` warns on errOut for every
  library entry that is malformed: no SKILL.md, frontmatter `name:`
  present but not matching the directory name, or `description:` absent
  or empty. Warnings are notes — they do not affect the exit code or
  the `bad` count. `spec check` WARNs the same way, but only for the
  resolved (enabled) set — the un-enabled remainder of the library is
  not the repo's problem.
- **Repo registry.** Repo-level `syncSkills` (which `spec init`, every
  repo `spec skill sync`, and `spec update`'s resync all run) records
  the repo's absolute root in `~/.config/spec/repos`, one path per
  line. The file is advisory only: reads prune entries that no longer
  contain a `.spec/` dir, and every consumer treats it as a hint, not
  a guarantee. `spec skill rm --library <name>` warns naming the
  registered repos (and the global `skills:` list) that enable the
  skill and will report it orphaned. `spec skill push` notes how many
  registered repos enable the pushed name — a reminder the shared copy
  serves others.
- **`import` sources.** `spec skill import <source> [name]` gains two
  source shapes beyond the existing bare-name and skill-dir cases:
  - a repo root — a directory containing `.spec/skills/` — from which
    `name` selects a repo skill (required when the repo has several,
    defaults when it has one);
  - a git URL (contains `://`, starts with `git@`, or ends in `.git`),
    which is shallow-cloned to a temp dir and then treated as a repo
    root the same way.
  The clone is the only networked operation; it happens only when the
  user passes a URL. An optional `name` operand also renames a skill
  imported from a plain skill dir.
- **`skill_global_targets:`** global-config key: a list of dirs
  (`~/`-relative or absolute) used as the install targets of
  `spec skill ... --global`, replacing the built-in agent→dir table
  when set. This is the user-level counterpart of `skill_targets:` and
  the extension point for agents spec does not know about.
- **Deferred: per-skill target lists.** Enabling a skill into a subset
  of `skill_targets:` would need structured config (the flat
  `key: [list]` format cannot express per-skill target sets) for a
  rare need — installing a skill for one agent but not another in a
  multi-agent repo. The workaround is a second repo or a repo-owned
  copy plus a custom `skill_targets:`. Revisit if a real case appears.

## Acceptance Criteria

- [x] `spec skill` groups enabled vs available, shows SKILL.md
  descriptions for both, explains the `available` section, and prints
  a status legend; the repo-free library listing shows descriptions too
- [x] `spec skill which` prints the resolved source (kind + dir),
  distinguishes `library (not enabled)`, and errors on unknown names
- [x] `spec skill diff` shows a unified diff of installed vs source per
  target and `--library` diffs repo vs library — all in-process, no
  external diff tool
- [x] `spec skill check` lints every library entry (SKILL.md present,
  frontmatter name matches dir, description non-empty) as non-fatal
  notes; `spec check` applies the same lint to the resolved set as WARN
- [x] `spec init` / repo `spec skill sync` register the repo in
  `~/.config/spec/repos`; `rm --library` names registered repos and the
  global list that will go orphaned; `push` notes the enabled-repo count
- [x] `spec skill import` accepts a repo root (`.spec/skills/`
  present), an optional rename operand, and git URLs (shallow clone);
  `--library` works for all source shapes
- [x] `skill_global_targets:` overrides the `--global` install dirs
- [x] Help text, completion table, config templates, README, and the
  spec SKILL.md document the new commands and keys; PROJ-001's REQ-003
  records the URL-import network exception
- [x] Tests cover the new commands, lint, registry warnings, repo-root
  import, and `skill_global_targets:`

