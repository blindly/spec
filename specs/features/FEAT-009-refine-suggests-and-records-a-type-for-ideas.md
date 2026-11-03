---
covers: [agent.go, commands.go, help.go, inbox.go, main.go]
created: 2026-11-01
depends_on: FEAT-008
id: FEAT-009
status: implemented
title: Refine suggests and records a type for ideas
type: feature
verified: 3309b8bba9d2844712d68ffbd7df96cae3fa5255
---

# Refine suggests and records a type for ideas

## Summary

Make `spec refine` the way an idea turns into a typed spec. Refining an idea
fleshes out its notes and always produces a type recommendation with a
reason. The recommendation is saved on the idea, shown in the inbox, used by
`spec promote` as its default, and can be acted on right away.

## Motivation

The inbox (FEAT-008) removes the need to choose a type up front, but you
still have to choose eventually. The refine prompt already asks the agent to
flag type mismatches for typed specs; for ideas, recommending a type is the
point of refining.

## Requirements

Addresses: REQ-013

## Design

- **Prompt for ideas:** refine an idea's notes for clarity and completeness
  without forcing a spec structure, then always end with
  `SPEC-TYPE-SUGGESTION: <project|feature|adr>: <reason>`. The prompt
  includes the "choosing a type" guidance from the help page.
- **Recording:** a valid suggestion is saved to the idea's frontmatter as
  `suggested_type` and `suggested_reason`, replacing any earlier one. The
  inbox row shows it as a badge, and the promote form preselects it.
- **Status:** a successful refine (body saved) moves a `new` idea to
  `refined`. A `refined` idea stays `refined`. Refining a `discarded` idea
  errors unless `--force` is given. Archived ideas (FEAT-007) can't be
  refined or promoted at all until they're unarchived.
- **No suggestion:** if the agent omits the line or names an invalid type,
  refine saves the body, prints "no type suggestion from agent", and leaves
  any earlier suggestion in place.
- **Next step:**
  - On a terminal, after saving: `Suggested: adr — <reason>. Promote now? [y/N]`.
    `y` runs `spec promote` with that type.
  - Not on a terminal: print the command instead (`spec promote IDEA-003 adr`).
- **Without an agent:** refine on an idea doesn't rewrite the body. Instead
  it runs a keyword heuristic and records the result with
  `suggested_reason: "heuristic: …"`:
  - decision words ("instead of", "chose", "alternatives", "vs") lean adr
  - checkbox lists and "should"/"must" lean feature
  - otherwise, no suggestion
- **Typed specs:** no change. Refine still only warns on a mismatch and
  never changes the type.

## Acceptance Criteria

- [x] Refining an idea with an agent that emits a suggestion saves `suggested_type` and `suggested_reason`
- [x] The suggestion line never appears in the saved body
- [x] Refining a `new` idea sets its status to `refined`; refining a `discarded` idea fails without `--force`
- [x] The inbox shows the suggested type as a badge and preselects it in the promote form
- [x] A missing or invalid suggestion prints "no type suggestion from agent" and keeps any previous suggestion
- [x] On a terminal, answering `y` to "Promote now?" promotes the idea with the suggested type
- [x] Not on a terminal, refine prints the promote command and doesn't prompt
- [x] With no agent, refine on an idea records a heuristic suggestion and leaves the body unchanged
- [x] Refining a typed spec behaves exactly as before
- [x] Tests cover each case with fake agents
