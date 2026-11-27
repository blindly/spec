---
covers: [skill.go, skill_test.go, spec_test.go, help.go, skills/spec/SKILL.md, README.md]
id: FEAT-044
type: feature
status: implemented
depends_on: [FEAT-026, FEAT-033]
title: local skills are first-class — global sync prunes like repo sync
verified: "2026-01-26"
---

## Summary

The enablement model is a three-tier vault: the **library**
(`~/.config/spec/skills/`) is invisible to harnesses until `use`
installs an entry into agent dirs; **repo** skills version with the
repo in `.spec/skills/`; and anything sitting in an agent dir that
spec didn't install is a **local skill** — legitimate, installed by
other means, never spec's to delete or scold.

Two gaps broke the model: the global sync path (`cmdSkillGlobal`)
never pruned un-resolved installs at all, so `unuse` couldn't hide a
skill from the harness at user level; and un-resolved dirs were
labeled "foreign" — debris to be removed — when they're local skills.

## Design

- **Prune only provable installs.** A shared `pruneSkillTargets`
  removes installs not in the resolved set *only* when they are a
  symlink into `.spec/skills/` or the library (`generatedLink`), or a
  real dir byte-identical to a library entry — content recoverable
  from the vault. This identical-copy rule is what makes `unuse` hold
  on copy-install systems (Windows, cross-device) where links can't
  be created. Everything else is left untouched.
- **Global sync gets the same pass** for `sync`/`update`, resolved =
  `{spec} ∪ global skills:` — closing the asymmetry. `check` stays
  read-only.
- **`spec skill` shows what's enabled here** — the resolved set with
  status plus a `local` group: dirs present in target dirs but not in
  the resolved set, naming where each sits (`installed by other means —
  spec doesn't manage; 'spec skill import' adopts`). The trailing
  "library (not enabled)" section leaves the main listing.
- **`spec skill library` shows the vault** — every library entry with
  its enablement (`enabled (repo)`, `enabled (global)`, or not
  enabled) and description. Works outside a repo too.
- **`use`/`unuse` flips enablement** — the only path between the vault
  and the harness's view.
- **Local ≠ problem**: quiet syncs don't narrate them, `spec check`
  doesn't fail them, and `--force` never removes them. Force affects
  only managed installs (stale copies of *enabled* skills).
- **Collision semantics unchanged**: `use` of a library skill whose
  name already sits in a target dir as a differing local reports
  stale; `--force` overwrites. `unuse` of an install that was
  hand-edited leaves it behind as a local — spec can't eat user edits.

## Acceptance Criteria

- [x] Global `sync`/`update` prune generated links and
  library-identical dirs not in the resolved set — same rule as repo
- [x] Differing real dirs and non-library names are never removed;
  they appear under `local` in `spec skill` with their target dirs
- [x] `use`/`unuse` is the harness-visibility gate for managed
  installs on both repo and global targets
- [x] `spec skill` shows enabled + `local` groups; `spec skill
  library` lists every vault entry with its enablement scope
- [x] No "foreign" vocabulary remains in user-facing output
- [x] Tests cover global prune (link, identical dir, differing kept)
  and the `local`/`library` listing groups

## Deferred / known edges

- Installs that predate spec management are indistinguishable from
  hand-installed locals — the gate applies going forward; identical
  leftovers are pruned, differing ones stay local by definition.
- `unuse --global` of an install with local edits still refuses
  without `--force` (explicit consent), rather than silently leaving
  a local — stricter than the model requires.
- `local` is reported per repo-level target only; a global-level
  local listing (`spec skill --global`) is unimplemented.
