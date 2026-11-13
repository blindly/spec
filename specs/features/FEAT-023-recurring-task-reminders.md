---
covers: [reminders.go, reminders_test.go, commands.go, agent.go, facts.go, main.go, skills/spec/SKILL.md, README.md]
created: 2026-11-05
id: FEAT-023
status: implemented
title: Recurring task reminders
type: feature
---

# Recurring task reminders

## Summary

Recurring obligations (monthly blog post, quarterly dependency review,
…) are tracked in `.spec/reminders.yml` — a small committed state file
listing each task, its cadence, and when it was last done. `spec remind`
manages entries; `spec status`, `spec check`, and agent prompts surface
tasks that are due.

## Motivation

Specs describe the system, not the calendar — but some requirements are
periodic obligations rather than one-time work ("publish a blog post
every month"). The spec documents the obligation (the why); a state file
tracks the operational when (last done, due now), so neither humans nor
agents forget a due task. Splitting why/when keeps reminders out of the
spec status lifecycle, which has no notion of "resets every month".

## Requirements

Addresses: REQ-015

## Design

- `.spec/reminders.yml`, hand-parsed like `facts.yml`: a list of `- `
  items with `key: value` fields — `id`, `title`, `every`, `last_done`,
  and optional `spec` linking to the spec that mandates the cadence.
  Committed, so the state is shared team/agent truth.
- `every`: day intervals (`7d`, `30d`, `90d` — due when `last_done` is
  ≥ N days ago) or calendar periods `weekly`, `monthly`, `quarterly`,
  `yearly` (due when `last_done` falls in an earlier ISO week / month /
  quarter / year — a task done any time in October satisfies October and
  is next due in November). Missing or unparseable `last_done` → due; a
  future `last_done` → not due.
- Commands: `spec remind` lists all entries with DUE markers and
  last-done dates; `spec remind add <title> --every <period>
  [--spec <id>]` creates an entry (id slugified from the title,
  deduplicated); `spec remind done <id>` stamps `last_done` with the
  local date; `spec remind remove <id>` deletes an entry.
- Surfacing: `spec status` prints `Reminders: N due`; `spec check` warns
  on each due entry and on a `spec:` field referencing an unknown spec
  id (warn, never fail — like drift); `repoContext` gains a "Recurring
  tasks due" section so agents see obligations in prompts.
- Non-goals: web UI editing, push notifications, per-user local state,
  scheduled agent runs.

## Acceptance Criteria

- [x] `spec remind add/done/remove` round-trips entries in
  `.spec/reminders.yml`, preserving comments
- [x] `Nd` and calendar periods compute due-ness correctly, including
  never-done and future `last_done`
- [x] `spec status` and `spec check` surface due reminders
- [x] Due reminders appear in agent prompts via `repoContext`
- [x] A `spec:` field referencing an unknown spec id warns in
  `spec check`
