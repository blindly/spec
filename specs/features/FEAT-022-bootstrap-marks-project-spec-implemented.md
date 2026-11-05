---
covers: [commands.go, spec_test.go]
created: 2026-11-02
id: FEAT-022
status: implemented
title: Bootstrap marks project spec implemented
type: feature
verified: b4e420a69259da3f82ca5556a41d034f7d8a8757
---

# Bootstrap marks project spec implemented

## Summary

`spec bootstrap` writes the generated project spec with status
`implemented` instead of `draft` — both on first creation and when
regenerating a draft/proposed one.

## Motivation

Bootstrap describes the system as it exists today, so the spec it
produces is living documentation from the start (REQ-011/ADR-003
semantics), not proposed work. A `draft` project spec hid the repo's own
description from the system view until someone promoted it by hand.

## Requirements

Addresses: REQ-011

## Design

- `writeResult` in `cmdBootstrap` sets `status: implemented` on both
  paths: `meta["status"]` before rewriting an existing spec, and
  `setSpecMeta` after `Store.Create`.
- Consequence: bootstrap is truly one-shot — the rerun guard
  (accepted/implemented → "Already bootstrapped") now triggers on the
  second run. Regeneration still works for specs the user set back to
  draft/proposed.

## Acceptance Criteria

- [x] A bootstrapped project spec has status `implemented`
- [x] A second `spec bootstrap` is a no-op ("Already bootstrapped")
- [x] Unit test asserts the implemented status
