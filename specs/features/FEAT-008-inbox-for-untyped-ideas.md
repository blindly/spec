---
covers: [agent.go, commands.go, config.go, help.go, inbox.go, main.go, store.go, templates.go, web.go, system.go]
created: 2026-11-01
depends_on: FEAT-006
id: FEAT-008
status: implemented
title: Inbox for untyped ideas
type: feature
verified: aeb547a901c588465f7629d37525ae76f8c9a61f
---

# Inbox for untyped ideas

## Summary

`spec new` forces you to pick a type (project, feature, adr) before you know
what the thing is, and since FEAT-003 that choice is permanent. This
feature adds an inbox of untyped ideas. Capturing an idea is the default way
to start, the inbox is the web UI's landing page, and `spec promote` turns
an idea into a typed spec once you know what it is.

## Motivation

Most specs start as a vague thought: "search is bad", "should we use
Postgres?". Whether that becomes a feature or an ADR only becomes clear
after thinking it through. Forcing the choice at creation leads to wrong
types, which are now expensive to fix.

## Requirements

Addresses: REQ-013, REQ-001

## Design

### The idea type

- New type `idea`: prefix `IDEA`, directory `specs/inbox/`, no required
  sections. Template:
  ```
  # <title>

  ## Notes

  TODO: what's the idea?
  ```
- `spec apply` never targets ideas, even with `--force`.
- Validate applies the usual type/prefix/directory rules (FEAT-003), with no
  section requirements.

### Idea statuses

- Ideas have their own lifecycle, separate from specs. New config key
  `idea_statuses`, default `[new, refined, discarded]`.
  - `new`: just captured (the initial status)
  - `refined`: refine has fleshed it out and suggested a type (FEAT-009)
  - `discarded`: decided against; the file is kept, so the idea isn't
    captured again
- `spec validate` checks ideas against `idea_statuses` and everything else
  against `statuses`. The web editor's status dropdown shows the list that
  matches the spec's type.
- `spec discard <IDEA-id>` sets an idea to `discarded`.

### Capturing

- `spec idea <title>` creates an idea with status `new`.
- In the web UI, the inbox has a capture box at the top (title, then
  Enter). The inbox is the one place to add ideas in the web UI; the "new
  spec" form is for typed specs only (FEAT-013).

### Inbox page (web landing page)

- `/` becomes the inbox, shown as a list, not a board. The board moves to
  `/board`. Nav: **inbox**, board, system (FEAT-011), new spec, readme, agents, help.
- One row per idea, newest first: id, title, status, the suggested-type
  badge (FEAT-009), and created date. Each row has:
  - a **promote** form: a type select (project/feature/adr) preselected to
    the suggested type, if any
  - a **discard** button
- Discarded ideas are hidden by default behind "N discarded — show".
- With an empty inbox: "Inbox empty — capture an idea above, or go to the
  board".
- New and discarded ideas never appear on the board; refined ideas appear in
  its "ready" column (FEAT-013).
- Promote and discard POSTs require the same form token as the other write
  forms.

### Promoting

- `spec promote <IDEA-id> [type]`, where `type` is `project`, `feature`, or
  `adr`. If omitted, the idea's `suggested_type` is used; if there's none,
  promote errors and asks for a type.
- It creates a new spec of that type with the next id and the type's
  template. The idea's notes replace the `TODO` text of the template's
  first section (Overview/Summary/Context). The new spec gets
  `promoted_from: IDEA-xxx` and status `draft`.
- **The idea file is deleted.** Git keeps its history, and `promoted_from`
  links the new spec back to it.
- Any `depends_on` references to the idea in other specs are rewritten to
  the new id.
- Discarded ideas can't be promoted without `--force`.
- Output: the new spec's path, plus a reminder to fill in its sections. The
  web promote redirects to the new spec's edit page.

### Docs

- Help page "Choosing a type": add "not sure yet? → `spec idea`".

## Acceptance Criteria

- [x] `spec idea "x"` creates `specs/inbox/IDEA-001-x.md` with status `new` and the idea template
- [x] `idea_statuses` defaults to `[new, refined, discarded]`, and validate checks ideas against it
- [x] The web editor's status dropdown shows idea statuses for ideas and spec statuses for everything else
- [x] `spec discard IDEA-001` sets status `discarded`
- [x] `/` shows the inbox as a list with capture box, promote, and discard; the board is at `/board`
- [x] Discarded ideas are hidden from the inbox by default, with a show toggle
- [x] Ideas don't appear on the board until refined (FEAT-013)
- [x] `spec promote IDEA-001 adr` creates an ADR with the idea's notes in Context and `promoted_from: IDEA-001`
- [x] The idea file is deleted after promoting
- [x] `spec promote IDEA-001` with no type uses `suggested_type`, or errors if there isn't one
- [x] Promoting a discarded idea fails without `--force`
- [x] `depends_on` references to the idea are rewritten to the new id
- [x] `spec apply` refuses ideas, even with `--force`
- [x] Web promote and discard reject a bad token
- [x] Tests cover capture, statuses, discard, promote with and without a type, deletion, reference rewriting, and the inbox page
- [x] Docs updated (help text, README, SKILL.md, web help)
