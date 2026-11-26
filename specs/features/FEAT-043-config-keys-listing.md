---
covers: [config.go, commands.go, help.go, spec_test.go]
id: FEAT-043
type: feature
status: implemented
depends_on: [FEAT-041, FEAT-042]
title: spec config keys — discoverable configuration options
verified: "2026-01-26"
---

## Summary

The set of supported config keys lived in three places that could drift:
the `loadConfig` parse switch, the `Config.String()` dump, and the
init templates. `spec config keys` lists every supported key with its
merge scope and a one-line description, driven by the same registry
`loadConfig` parses through — an undocumented key can't be parsed, and
an unparsed key can't be documented.

## Design

- `configKeys` registry in config.go: `{name, scope, doc, set}`. The
  merge loop applies `set` per key from the merged map; `repoOnly` keys
  read only the repo file. `skills` (global∪repo union) and
  `skill_global_targets` (global only, parsed in skill.go) are
  documented entries whose values are applied outside the loop.
- `spec config keys` prints `key / scope / description`.
- `spec config` continues to print every key's effective value.

## Acceptance Criteria

- [x] `spec config keys` lists every supported key with scope and doc
- [x] Parsing is registry-driven — the list can't drift from `loadConfig`
- [x] Repo-only keys (release, push_command, push_on_commit) are labeled
