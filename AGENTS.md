# Agent instructions

This repository uses spec for spec-driven development.

- Specs live in specs/ (project/, features/, adr/, imported/); untyped
  ideas live in specs/inbox/ until promoted (spec promote).
- Each spec is markdown with YAML frontmatter (type, status, depends_on).
- Valid statuses are configured in .spec/config.yml (statuses:).
- .spec/facts.yml holds definite project facts, referenced as {fact:key}.
- Run spec check to validate specs and spec verify to run configured checks.
- When implementing a spec, update its frontmatter status to implemented.
