---
covers: [web.go, commands.go]
created: 2026-11-01
id: FEAT-003
status: implemented
title: Lock spec type in web editor
type: feature
verified: 782805b99eaa54dfcf313cc51252c43f60be7f9a
---

# Lock spec type in web editor

## Summary

The web edit page has a type dropdown. Changing it rewrites the `type:`
frontmatter but keeps the old id and file path, so a spec can end up as
type `adr` with id `FEAT-001` in `specs/features/`. This feature makes the
type read-only in the editor and has `spec validate` catch mismatches that
already exist or come from hand edits.

## Motivation

The type is fixed at creation: it sets the id prefix, directory, required
sections, and whether `spec apply` picks the spec up. Editing only the
frontmatter breaks those links silently.

## Requirements

Addresses: REQ-001, REQ-007, REQ-002

## Design

- **Editor:** replace the type `<select>` with read-only text, plus a hint:
  "to change the type, create a new spec".
- **Server:** the edit POST ignores any submitted `type` and keeps the
  spec's current type. Required-section checks use the current type.
- **Validate:** `spec validate` fails when a spec's `type` doesn't match its
  id prefix (`PROJ`/`FEAT`/`ADR`/`DOC`) or its directory, naming the spec
  and both values.
- The `imported` type keeps working as it does today.

## Acceptance Criteria

- [x] The edit page shows the type as text, not a dropdown
- [x] POSTing a different `type` to `/edit/<id>` leaves the saved type unchanged
- [x] Required sections are checked against the spec's current type
- [x] `spec validate` reports a spec whose type doesn't match its id prefix
- [x] `spec validate` reports a spec whose type doesn't match its directory
- [x] Existing tests updated; new tests cover the POST and both validate cases
