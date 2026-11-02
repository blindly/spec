package main

// helpMarkdown is rendered on the web UI's /help page via mdBlock.
const helpMarkdown = `# Help

A spec is a markdown file under ` + "`specs/`" + ` with YAML frontmatter:

` + "```" + `
---
id: FEAT-001
type: feature          # project | feature | adr | imported
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

## Choosing a type

Pick by the question the document answers:

| Type | Answers | Use when |
|---|---|---|
| **project** | What is this system, and what must it do? | Describing the whole system and its ` + "`REQ-xxx`" + ` requirements — usually one per repo |
| **feature** | What should we build? | Adding or changing behavior; ` + "`spec apply`" + ` implements it |
| **adr** | Why X instead of Y? | Recording a decision and the alternatives rejected; nothing to implement |
| **imported** | — | Bringing in existing docs as reference (` + "`spec import`" + `) |

Quick tests:

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

Choose the type at creation. It sets the id prefix (PROJ/FEAT/ADR/DOC),
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
spec status     # spec counts, REQ coverage
spec new feature <title>
spec search <query>
spec validate   # frontmatter, sections, REQ refs, dependency cycles
spec check      # audit {fact:key} references
spec apply      # run the configured agent to implement specs (--print to output prompts)
spec verify     # run verify: commands from .spec/config.yml
spec graph      # dependency graph (--mermaid)
spec trace      # REQ → feature coverage
` + "```" + `
`
