---
covers: [commands.go, config.go, help.go, main.go, spec_test.go, view.go]
created: 2026-11-02
id: FEAT-016
status: implemented
title: spec view
type: feature
verified: 3309b8bba9d2844712d68ffbd7df96cae3fa5255
---

# spec view

## Summary

`spec view <id>...` prints specs to stdout — a one-line header with the
spec's id, type, status, and title, followed by its body. It is the
read-only counterpart of `spec edit`: inspect a spec without launching an
editor, and pipe it into `grep`, `less`, or another tool. The display
command is overridable: a `viewer:` config key or `--cmd` flag can hand
the file to an external renderer (`glow` for markdown, `cat` for plain
text), while the default stays spec's own plain output.

## Motivation

The web UI and `spec edit` are the only ways to read a spec; `spec search`
shows matching lines but never the whole document. A terminal user (or an
agent collecting context) should be able to print a spec directly.

## Requirements

Addresses: REQ-011

## Design

- `spec view [--cmd CMD] <id>...` takes one or more spec ids, resolved the
  same way as `spec edit` (`st.Get`) — any spec works, including archived
  ones.
- **Default output** (no viewer configured): each spec prints as a header
  line, e.g. `FEAT-016 · feature · proposed — spec view`, a separator,
  then the body. Multiple specs are separated by a blank line.
- **Viewer override:** resolution order is `--cmd` flag, then the new
  `viewer:` config key, then the built-in default. No environment
  variable is consulted — the default stays scriptable even when the
  user has `$PAGER` set.
- The viewer command is exec'd with the spec's file path, using the same
  contract as `agent_cmd`: a `{file}` placeholder gets the path; if
  absent, the path is appended — so `viewer: "cat"` and
  `viewer: "glow --style dark {file}"` both work. With multiple ids the
  viewer runs once per spec, in order.
- When a viewer is used it owns the display — spec prints no header of
  its own. Viewers are launched per invocation, unlike `spec edit`'s
  one-shot `$EDITOR`; a missing viewer binary is a clear error.
- An unknown id is an error naming the id; earlier ids in the same
  invocation still print.
- The command appears in the usage list and the README command table, and
  `viewer:` is documented in the config template like `editor:`.

## Acceptance Criteria

- [x] `spec view FEAT-016` prints the spec's header line and body
- [x] Multiple ids print in order, separated for readability
- [x] `spec view --cmd cat FEAT-016` pipes the file through cat; `--cmd glow` renders it
- [x] `viewer:` in config applies when no `--cmd` is passed; `--cmd` wins over config
- [x] `{file}` placeholder in the viewer command is substituted; without it the path is appended
- [x] A viewer that isn't on PATH produces a clear error
- [x] Archived and inbox specs are viewable by id
- [x] Unknown ids produce a clear error; valid ids in the same call still print
- [x] Default output never consults $PAGER or launches an editor
- [x] Usage and README list the command; config template documents `viewer:`
- [x] Tests cover default output, `--cmd`, config `viewer:`, precedence, unknown ids, and archived specs
