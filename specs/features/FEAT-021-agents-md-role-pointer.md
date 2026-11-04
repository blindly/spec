---
covers: [role.go, persona.go, templates.go, role_test.go, persona_test.go]
created: 2026-11-02
id: FEAT-021
status: implemented
title: AGENTS.md role pointer
type: feature
verified: a33dc388d4e1ed03f312baf415d45b586dafc305
---

# AGENTS.md role pointer

## Summary

When a development role or persona is in effect, spec ensures AGENTS.md
tells agents to adopt it — a marker-delimited
`<!-- spec:role -->` block appended on role-granting operations
(`role refine`, `role adopt`, `persona use`), plus a line in the
`agentsTemplate` that `spec init` seeds. Agents run directly in the repo
(outside `spec apply`) then pick up the same persona.

## Motivation

The role file and persona selection (FEAT-019, FEAT-020) only reach agent
prompts that spec builds. Agent CLIs invoked by hand read AGENTS.md
natively — a pointer there makes the role effective for ad-hoc sessions
without duplicating the role text into AGENTS.md.

## Requirements

Addresses: REQ-004

## Design

- `ensureRolePointer(root)` appends a marker-delimited block
  (`<!-- spec:role -->` … `<!-- /spec:role -->`) to AGENTS.md when absent;
  creates a minimal AGENTS.md when missing. The block references the role
  sources (`.spec/role.md`, `persona:` in `.spec/config.yml` with the
  `~/.config/spec/personas/` library) rather than copying role text, so it
  stays valid when the role changes.
- Called after a role becomes available: `spec role refine` and
  `spec role adopt` write `.spec/role.md`, `spec persona use <name>`
  selects a persona. It is idempotent — the marker prevents duplicates.
- `agentsTemplate` gains a bullet pointing at the role file and persona
  mechanism, so `spec init`-seeded repos document it from the start.
  (CLAUDE.md needs no change — it already `@AGENTS.md`-imports.)
- Only *granting* operations write the pointer: `persona use none` and
  removing `role.md` leave the block — it describes where a role would
  come from and is harmless when none is set.

## Acceptance Criteria

- [x] `role refine`, `role adopt`, and `persona use` leave a
  `<!-- spec:role -->` block in AGENTS.md; a second run adds nothing
- [x] The pointer is created even when AGENTS.md doesn't exist
- [x] `agentsTemplate` documents the role mechanism for `spec init`
- [x] Removing the role doesn't break anything (block is inert text)
- [x] Unit tests cover append-once and creation
