---
covers: store.go
created: 2026-11-01
id: FEAT-005
status: implemented
title: Atomic spec writes
type: feature
verified: 8bb4cbfd3650920472fae258012b1050c1712619
---

# Atomic spec writes

## Summary

The web editor saves specs by writing a temp file and renaming it over the
original, but other code paths rewrite existing files in place with
`os.WriteFile`. If the process crashes or the disk fills mid-write, the file
is left truncated. This feature routes every rewrite of an existing file
through one atomic write helper.

## Motivation

`spec refine` overwrites a spec after a long agent run, and facts are
rewritten on every `spec fact set`. A partial write there loses user
content with no backup.

## Requirements

Addresses: REQ-006

## Design

- Add `writeFileAtomic(path string, data []byte, perm os.FileMode) error`:
  write to a temp file in the same directory, sync it, then swap it in with
  the existing `replaceFile` (which already handles Windows). Remove the
  temp file on any error.
- Use it wherever an existing file is rewritten:
  - `spec refine` (commands.go)
  - `spec bootstrap` when it updates an existing spec (commands.go)
  - `setFact` and `unsetFact` (facts.go)
  - the web editor save (web.go), replacing its inline temp-file logic
- Creating brand-new files can keep using `os.WriteFile`, since there's no
  existing content to lose.

## Acceptance Criteria

- [x] `writeFileAtomic` exists and is used by refine, bootstrap updates, setFact, unsetFact, and web edit
- [x] If the write fails, the original file is unchanged and no temp file is left behind
- [x] The web editor's inline temp-file code is replaced by the helper
- [x] Unit test: a failed write leaves the original intact
- [x] Existing refine, facts, and web edit tests still pass
