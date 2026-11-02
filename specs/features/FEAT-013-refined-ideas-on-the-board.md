---
created: 2026-11-02
depends_on: [FEAT-008, FEAT-009, FEAT-011]
id: FEAT-013
status: implemented
title: Refined ideas on the board
type: feature
---

# Refined ideas on the board

## Summary

Refined ideas (fleshed out, with a suggested type) appear on the board as a
first "ready" column, so the board shows the whole pipeline from "ready to
start" through accepted work. Raw ideas stay in the inbox. The web "new spec"
form becomes typed-only (project, feature, adr): ideas are captured from the
inbox, so there's one place to add them.

## Motivation

Two pages could add an idea (the inbox's capture box and the "new spec" form,
which defaulted to idea), which blurred what "new spec" means. Meanwhile the
board couldn't show what's ready to pick up next. A refined idea is exactly
that, and refining gives the `refined` status a visible purpose.

## Requirements

Addresses: REQ-013, REQ-011

## Design

- **Board:** a first column, **ready**, lists non-archived ideas with status
  `refined`, newest first. Each card shows the id, title, the suggested-type
  badge, and a promote form with a type select preselected to the suggested
  type. New and discarded ideas never appear on the board. The column is
  omitted when there are no refined ideas, and it counts as work for the
  board's "No work in progress" message.
- **Inbox:** unchanged. It still lists all ideas, refined ones included,
  since it's where ideas are captured and triaged.
- **New spec form:** offers project, feature, and adr only, with feature
  selected by default, plus a hint: "Not sure what it is yet? Add it to the
  inbox." Posting `type=idea` to `/new` is rejected.
- **CLI unchanged:** `spec idea` is the documented way to capture;
  `spec new idea` keeps working.

## Acceptance Criteria

- [x] The board shows a "ready" column first with refined ideas, each with a suggested-type badge and a promote form
- [x] New and discarded ideas don't appear on the board; the column is omitted when there are no refined ideas
- [x] Promoting from the board creates the spec and opens its edit page
- [x] The new spec form offers only project, feature, and adr, with feature selected, and links to the inbox
- [x] Posting an idea to `/new` is rejected
- [x] Tests cover the ready column, its exclusions, promote from the board, and the form
- [x] Docs and FEAT-008/FEAT-011 updated
