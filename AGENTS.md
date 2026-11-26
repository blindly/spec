# Agent instructions

This repository uses spec for spec-driven development.

- Specs live in specs/ (project/, features/, adr/, imported/); untyped
  ideas live in specs/inbox/ until promoted (spec promote).
- Each spec is markdown with YAML frontmatter (type, status, depends_on).
- Valid statuses are configured in .spec/config.yml (statuses:).
- .spec/facts.yml holds definite project facts, referenced as {fact:key}.
- Run spec check to validate specs and spec verify to run configured checks.
- When implementing a spec, update its frontmatter status to implemented.
- Implemented specs are living documentation: if a change alters behavior
  another spec describes, update that spec in the same change.
- Run spec drift to find specs whose covered code changed since they were
  last updated; fix them, or spec verified <id> if still accurate. Never
  remove covers: entries to silence a drift flag — covers list what the
  spec describes; an accurate spec gets verified, not decoupled.
- A spec-managed pre-push hook blocks pushes that change files outside
  specs/ without a spec update; an advisory commit-msg hook warns at
  commit time, and a pre-commit hook runs the repo's configured
  pre_commit checks (spec precheck tests them). spec push runs the
  same gate in-process before pushing. Commit through `spec commit` with conventional-commit
  subjects: a non-feat: prefix (fix:, docs:, chore:, ...) or a
  `Spec-Exempt: <reason>` trailer (--exempt) attests spec-neutral work;
  --agent asks the configured agent to write the message and pick the
  attestation (a configured agent is the default;
  commit_agent: false or --no-agent overrides); `SPEC_SKIP=1 git push` is the last-resort bypass.
- A VERSION file opts the repo into release versioning: `spec commit`
  refreshes the stamp, and `spec release` stamps a VERSION-only commit,
  pushes, and tags `v<stamp>` — only where repo config sets
  `release: true`.
