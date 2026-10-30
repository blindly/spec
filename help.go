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
spec verify     # run verify: commands from .spec/config.yml
spec graph      # dependency graph (--mermaid)
spec trace      # REQ → feature coverage
` + "```" + `
`
