---
covers: [commands.go, main.go, help.go, spec_test.go]
created: 2026-11-02
id: FEAT-018
status: implemented
title: spec ls lists specs with paths for editing
type: feature
verified: 70ccef5a8bf0436583d59a2cf4fa799a42e8e7c0
---

# spec ls lists specs with paths for editing

## Summary

`spec ls` (alias `spec list`) prints one aligned line per spec — id, type,
status, title, and file path — so users can quickly find the id for
`spec edit <id>` or the path for `$EDITOR`. `--paths` prints paths only,
for piping to `xargs` or scripts; `--all` also lists archived specs.

## Motivation

`spec status` shows only aggregate counts and `spec view`/`spec edit`
require an id. There was no way to answer "which specs exist and where do
they live?" without listing files on disk.

## Requirements

Addresses: REQ-011

## Design

- `cmdList` iterates `Store.Specs()` (id-sorted) and writes tab-aligned
  columns via `text/tabwriter`: ID, TYPE, STATUS, TITLE, PATH (relative to
  the repo root). Ideas appear with type `idea`.
- Consistent with REQ-011, archived specs are hidden by default; `--all`
  includes them, marked `(archived)` after the status.
- `--paths` suppresses the columns and prints one path per line for
  scripting (`spec ls --paths | xargs $EDITOR`).
- Registered as both `ls` and `list`; documented in usage and `help.go`.

## Acceptance Criteria

- [x] `spec ls` lists every non-archived spec with id, type, status, title,
  and repo-relative path
- [x] `spec ls --all` also lists archived specs, marked `(archived)`
- [x] `spec ls --paths` prints only paths, one per line
- [x] `spec list` is an alias
- [x] Unit test covers listing, `--all`, and `--paths`
