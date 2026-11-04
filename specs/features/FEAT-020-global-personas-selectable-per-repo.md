---
covers: [persona.go, role.go, config.go, agent.go, commands.go, main.go, help.go, role_test.go]
created: 2026-11-02
id: FEAT-020
status: implemented
title: Global personas selectable per repo
type: feature
verified: 425321963e1a1691e1d9a56b9eebe3c8e91bae46
---

# Global personas selectable per repo

## Summary

A user-level persona library lives at `~/.config/spec/personas/<name>.md`.
A repo opts in with `persona: <name>` in `.spec/config.yml`; the named
persona supplies the development role when `.spec/role.md` is absent or
empty. The project role always wins. `spec persona` manages the library,
`spec persona use <name>` selects one, and `spec role adopt` copies the
active persona into the project file so the repo can diverge.

## Motivation

FEAT-019 made the role strictly repo-local. Users who develop the same way
across many projects (e.g. "senior Go engineer") re-create the role in
every repo. A named, shared persona library removes the duplication while
keeping the rule that nothing global is used unless the repo opted in.

## Requirements

Addresses: REQ-004

## Design

- **Library:** one free-form markdown file per persona under
  `<global config dir>/personas/`. Names match
  `[A-Za-z0-9][A-Za-z0-9_.-]*` (no path traversal).
- **Selection:** a `persona:` config key, parsed like other flat keys and
  merged global→repo as usual. `spec persona use <name>` writes it into
  the repo's `.spec/config.yml`; `spec persona use none` removes it.
- **Resolution** (`roleFor`): `.spec/role.md` non-empty → project role;
  else `persona:` names a library file → that content; else none. The
  effective role is injected into prompts via `repoContext` unchanged —
  only the source differs.
- **`spec role`** prints the effective role plus a stderr note naming the
  source when it comes from a persona. **`spec role refine`** refines the
  effective role and writes `.spec/role.md` — with a persona active this
  forks it into the project. **`spec role adopt`** copies the active
  persona into `.spec/role.md` verbatim (errors when a project role or no
  persona is in use).
- **`spec persona`** lists personas (marking the active one);
  `new`/`edit` manage files via the configured editor; `refine <name>`
  improves a persona with the same verbatim/heading output contract —
  but is deliberately repo-blind (no `repoContext`) so repo specifics
  can't leak into a shared persona.
- **`spec status`** reports the effective role source.

## Acceptance Criteria

- [x] `.spec/role.md` takes precedence over any configured persona;
  persona used only when the file is absent/empty; nothing global is read
  unless `persona:` is set
- [x] `spec persona` lists the library and marks the active persona
- [x] `spec persona new|edit|refine <name>` manage library files; refine
  is repo-blind and enforces the `# ` heading contract
- [x] `spec persona use <name>` writes `persona:` to `.spec/config.yml`;
  `use none` removes it
- [x] `spec role` shows the effective role and its source; `spec role
  adopt` copies the active persona into the project file
- [x] `spec status` names the role source
- [x] Unit tests cover resolution precedence, `use`, `adopt`, and
  repo-blind persona refine
