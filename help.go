package main

// helpMarkdown is rendered on the web UI's /help page via mdBlock.
const helpMarkdown = `# Help

A spec is a markdown file under ` + "`specs/`" + ` with YAML frontmatter:

` + "```" + `
---
id: FEAT-001
type: feature          # project | feature | adr | imported | idea
status: proposed       # valid values come from .spec/config.yml (statuses:)
depends_on: [FEAT-000] # optional; spec graph shows these, cycles fail validate
---
` + "```" + `

Each type has required sections (enforced by ` + "`spec validate`" + ` and the
editor's save):

- **project**: Overview, Requirements — owns the ` + "`REQ-xxx`" + ` ids
- **feature**: Summary, Design, Acceptance Criteria — must list
  ` + "`Addresses: REQ-xxx`" + ` for existing REQ ids
- **adr**: Context, Decision, Consequences
- **idea**: none — just a title and notes

## The inbox

**Not sure what type something is yet? Capture it as an idea.** Ideas live
in the inbox (the web UI's front page, ` + "`specs/inbox/`" + `, ids ` + "`IDEA-xxx`" + `)
with their own lifecycle: **new** → **refined**, or **discarded**. When you
know what it is, promote it:

` + "```" + `
spec idea "better search"          # capture
spec refine IDEA-003               # agent fleshes out the notes and suggests a type → refined
spec promote IDEA-003 feature      # becomes FEAT-xxx; the idea's notes go into its first section
spec discard IDEA-004              # decided against (kept, hidden in the inbox)
` + "```" + `

Refining an idea records the agent's ` + "`suggested_type`" + ` (shown as a badge in
the inbox and preselected for promote) and offers to promote right away.
Without an agent, refine only guesses the type from keywords, labelled
"heuristic". ` + "`spec promote`" + ` with no type uses the suggestion.

Promoting deletes the idea file — the new spec records ` + "`promoted_from`" + `,
and git keeps the history. Ideas never appear on the board and are never
applied.

## Inbox, board, system

The web UI has three views, one per stage:

| View | Shows | Answers |
|---|---|---|
| [inbox](/) | ideas | What might we do? |
| [board](/board) | work in progress: draft/proposed/accepted features, draft/proposed ADRs | What are we working on? |
| [system](/system) | implemented features and projects, accepted ADRs, imported docs | What does the system do today? |

**Implemented specs are living documentation.** An implemented spec
describes how the system works today, so it stays in the system view for as
long as it's true. When later work changes behavior a spec describes, that
spec is updated in the same change (` + "`spec apply`" + ` tells agents to do this).
Specs that are no longer valid (` + "`done_statuses`" + `, default deprecated/rejected)
are hidden behind the board's "show" toggle.

## Archive

When a spec stops describing the system, mark it **deprecated** (or update
it). Deprecated and rejected specs, and discarded ideas, can then be archived:

` + "```" + `
spec archive FEAT-002 FEAT-004     # → specs/archive/features/, stamped archived: <date>
spec unarchive FEAT-002            # moves it back
` + "```" + `

Implemented specs can't be archived without ` + "`--force`" + ` — they're still valid.
Archived specs keep their ids, stay searchable (` + "`spec search`" + ` marks them),
still count as REQ coverage in ` + "`spec trace`" + `, and are listed on the
[archive](/archive) page — but they're off the board, the inbox, ` + "`status`" + `
counts, ` + "`roadmap`" + `, and ` + "`apply`" + `.

## Choosing a type

Pick by the question the document answers:

| Type | Answers | Use when |
|---|---|---|
| **project** | What is this system, and what must it do? | Describing the whole system and its ` + "`REQ-xxx`" + ` requirements — usually one per repo |
| **feature** | What should we build? | Adding or changing behavior; ` + "`spec apply`" + ` implements it |
| **adr** | Why X instead of Y? | Recording a decision and the alternatives rejected; nothing to implement |
| **imported** | — | Bringing in existing docs as reference (` + "`spec import`" + `) |
| **idea** | Not sure yet | Capturing a thought before you know which of the above it is |

Quick tests:

- **Not sure yet?** → idea (promote it later)
- **Can you write a checkbox that proves it's done?** → feature
- **Did you reject an alternative someone might later ask about?** → adr
- **Is the decision hard to reverse or costly to get wrong?** → adr

Features and ADRs often come in pairs: the feature is the work, the ADR
records why it was done that way. Don't write an ADR for every choice —
only ones a future reader would question.

There is no bug type:

- code violates a spec → fix the code
- spec is wrong or ambiguous → edit the spec (or write an adr if it's a real decision)
- bug reveals a missing requirement → add the REQ to the project spec, then a feature

A spec's type is fixed once created. It sets the id prefix (PROJ/FEAT/ADR/DOC/IDEA),
the directory, the required sections, and whether ` + "`spec apply`" + ` picks it up —
changing it later means a new spec, not an edit. ` + "`spec refine`" + ` warns
when a spec's content looks like a different type, but never changes it.
ADRs are never rewritten
once accepted: write a new ADR that supersedes the old one and mark the old
one deprecated.

## Facts

` + "`.spec/facts.yml`" + ` holds definite project values (domain, phone,
company name, …) as flat ` + "`key: value`" + ` pairs. Reference them in spec
bodies as ` + "`{fact:key}`" + ` instead of inventing values — they are
substituted at spec creation and injected into agent prompts.
` + "`spec check`" + ` fails on references to undefined keys.

Manage facts from the CLI: ` + "`spec fact list|get|set|unset`" + `.

## CLI

` + "```" + `
spec status     # inbox + spec counts, REQ coverage
spec idea <title>                 # capture an idea (type decided later)
spec promote <IDEA-id> [type]     # idea → project|feature|adr (default: suggested type)
spec discard <IDEA-id>
spec archive <id>...              # deprecated/rejected specs, discarded ideas → specs/archive/
spec unarchive <id>...
spec new feature <title>
spec search <query>
spec validate   # frontmatter, sections, type/id/dir match, REQ refs, cycles
spec check      # audit {fact:key} references
spec apply <id> # run the configured agent to implement a spec
spec apply      # all accepted specs, after confirmation (--yes to skip)
                # succeeds only if status reaches implemented and verify: passes
                # --print outputs prompts · --no-verify skips verify
spec verify     # run verify: commands from .spec/config.yml
spec graph      # dependency graph (--mermaid)
spec trace      # REQ → feature coverage
` + "```" + `
`
