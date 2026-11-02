---
created: 2026-11-01
id: PROJ-001
status: implemented
title: spec
type: project
---

# spec

## Overview

`spec` is an offline-first specification management CLI written in Go
(stdlib only). It manages project specs, feature specs, and ADRs as markdown
files with YAML frontmatter under `specs/`, validates them, traces
requirements to features, and can drive an LLM agent CLI to draft, refine,
and implement specs. A local web UI provides browsing and editing.

## Goals

- Specs live in the repo as plain markdown, reviewable like code.
- Every core operation works offline with no network access.
- Agent integration is opt-in and works with many agent CLIs.
- Spec invariants (ids, types, sections, requirement references) are
  enforced by tooling rather than convention.

## Non-Goals

- Hosting specs anywhere other than the local filesystem.
- A multi-user collaboration server.
- Drag-and-drop or rich project-management features in the web UI.

## Requirements

- REQ-001: Specs are markdown files with frontmatter under `specs/`, typed
  `project`, `feature`, `adr`, `imported`, or `idea`. The type determines the
  id prefix (PROJ/FEAT/ADR/DOC/IDEA), the directory, and the required
  sections, and these must stay consistent.
- REQ-002: `spec validate` enforces required sections, configured statuses,
  references to defined `REQ-xxx` ids, and the absence of dependency cycles.
- REQ-003: All operations other than agent commands work offline. Agents
  run only when configured.
- REQ-004: Agent commands (`draft`, `refine`, `apply`) run the configured
  agent CLI with a timeout and report the actual outcome to the user.
- REQ-005: `spec apply` runs agents only on a scope the user has explicitly
  approved; it never starts unexpected or unreviewed agent runs.
- REQ-006: Rewriting an existing spec or project file never leaves it
  truncated or partially written.
- REQ-007: The web UI lets users browse, create, and edit specs without
  breaking spec invariants (REQ-001, REQ-002).
- REQ-008: The web UI does not allow unauthorized users to read or modify
  specs.
- REQ-009: Definite project values live in `.spec/facts.yml` and specs
  reference them with fact placeholders; `spec check` audits the references.
- REQ-010: `spec trace` and `spec status` report REQ → feature coverage.
- REQ-011: Default views (the web board, `spec status`) show every spec that
  is still valid — including implemented specs, which are the living
  description of the system. Only specs that are no longer valid
  (deprecated, rejected) are kept out of the way.
- REQ-012: Specs that are no longer valid can be archived out of all active
  views while remaining in the repo, searchable, and referenceable. Valid
  (implemented) specs are not archived.
- REQ-013: Users can capture an idea without choosing a spec type, and turn
  it into a typed spec later, with the tool suggesting the type.
- REQ-014: Implemented specs stay accurate. A change that alters behavior
  described by an existing spec updates that spec as part of the same
  change, so specs don't silently go stale after implementation.

## Architecture

Single Go binary. `store.go` loads and creates specs; `frontmatter.go`
parses and writes frontmatter; `commands.go` implements CLI commands;
`agent.go` resolves and runs agent CLIs; `web.go` serves the web UI with
embedded assets; `facts.go` manages facts. Configuration merges
`~/.config/spec/config.yml` with the repo's `.spec/config.yml`.
