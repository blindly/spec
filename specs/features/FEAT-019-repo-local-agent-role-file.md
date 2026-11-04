---
covers: [role.go, agent.go, main.go, help.go, commands.go, driftcmd.go]
created: 2026-11-02
id: FEAT-019
status: implemented
title: Repo-local agent role file
type: feature
verified: a33dc388d4e1ed03f312baf415d45b586dafc305
---

# Repo-local agent role file

## Summary

An optional `.spec/role.md` file describes the persona and conventions the
configured agent should adopt — e.g. "senior Go engineer, stdlib only,
minimal diffs". When the file exists and is non-empty, its content is
injected into every agent prompt. A `spec role` command family manages it:
`spec role` prints it, `spec role edit` opens it in the editor, and
`spec role refine` has the agent improve it (or draft one from the repo
when it's missing/empty).

## Motivation

Agent prompts currently hardcode their framing with no project-specific
voice. A repo-local role lets a project tell every agent invocation who it
is and how it works — distinct from AGENTS.md, which some agent CLIs read
natively but spec does not control and cannot inject for agents that
ignore it.

## Requirements

Addresses: REQ-004

## Design

- `.spec/role.md` is a free-form markdown file next to `facts.yml`. It is
  project context, not a spec: no id, status, lifecycle, or required
  sections. It is **repo-local** — nothing global supplies a role unless
  the repo opts into a shared persona via `persona:` (FEAT-020); a missing
  or empty file and no persona is simply "no role".
- `loadRole(root)` reads and trims the file; "" means unset.
- `repoContext` appends a `Development role (follow this persona and these
  conventions):` section when a role is set. Because every prompt builder
  (apply, draft, refine, refine-idea, bootstrap, fact discover, role refine, drift review) already
  goes through `repoContext`, that is the single injection point. Drift review also appends
  `repoContext` so the role applies there too.
- `spec role` prints the role, or a hint on stderr that none is set.
- `spec role edit` creates the file if absent and opens it via the
  configured editor (same resolution as `spec edit` / `spec config edit`).
- `spec role refine` runs the configured agent: given the current role
  (possibly empty) plus `repoContext`, it returns an improved or
  newly-drafted role; the result is written atomically to `.spec/role.md`.
  Without an agent it fails like `spec refine` does. A role is often
  defined before development begins, so the prompt does not require an
  established codebase — the repository context is optional input.
- The refine prompt states the reply is written verbatim to the file and
  must be only the file contents — a markdown document starting with a
  `#` heading and a one-sentence "You are a …" persona statement, with no
  re-documentation of tools the agent already knows. The output is
  cleaned through `bodyFromAgentOutput` (narration, frontmatter, and
  fences stripped); output that doesn't start with a `# ` heading fails
  like `spec refine` and leaves the role unchanged.
- `spec status` shows `Role: .spec/role.md` when set, so users can see the
  agent has a persona.

## Acceptance Criteria

- [x] `.spec/role.md` content appears in every agent prompt; absent or
  empty file changes nothing
- [x] Nothing global supplies a role unless the repo opts into a shared
  persona via `persona:` (FEAT-020)
- [x] `spec role` prints the role or a "no role set" hint
- [x] `spec role edit` opens the file in the editor, creating it first
- [x] `spec role refine` drafts a role from the repo when the file is
  missing/empty and improves it when present; errors clearly without an
  agent
- [x] `spec status` reports the role when set
- [x] Unit tests cover loading, prompt injection, and the refine path
