---
covers: agent.go
created: 2026-11-01
id: ADR-001
status: accepted
title: Bypass codex sandbox in spec apply
type: adr
verified: 3309b8bba9d2844712d68ffbd7df96cae3fa5255
---

# Bypass codex sandbox in spec apply

## Context

`spec apply` and `spec refine` run codex non-interactively with
`codex exec`. Applying a spec needs write access to the repo. Two problems
came up:

1. Codex refuses to run outside a trusted git repository, and spec-managed
   repos aren't necessarily git repos (`spec init` doesn't create one).
2. Codex's sandbox (bubblewrap) can't start in container environments that
   lack CAP_SYS_ADMIN. It fails with
   `bwrap: loopback: Failed RTM_NEWADDR: Operation not permitted`, and every
   sandbox mode (`read-only`, `workspace-write`) fails the same way.
   `codex exec` also can't answer approval prompts.

Alternatives considered:

- **`-s workspace-write`:** the right permissions, but bwrap still can't
  start in these environments.
- **Granting the container CAP_SYS_ADMIN:** weakens the outer sandbox in
  order to enable the inner one.
- **Leaving the default and documenting `agent_cmd`:** works, but every
  codex user in a container hits the error first.

## Decision

The built-in codex adapter runs:

```
codex exec --skip-git-repo-check --dangerously-bypass-approvals-and-sandbox {prompt}
```

Users running on a bare host who want codex's sandbox can set
`agent_cmd: "codex exec -s workspace-write {prompt}"` in
`.spec/config.yml`.

Shipped in v1.0.32 (`--skip-git-repo-check`) and v1.0.33 (sandbox bypass).

## Consequences

- `spec apply` and `spec refine` work with codex out of the box, including
  in containers and non-git repos.
- Codex commands run unsandboxed with the user's permissions. That's
  acceptable only when the outer environment is trusted or isolated.
- On bare hosts the default is less protected unless the user sets
  `agent_cmd`.
- If codex gets a sandbox that works without CAP_SYS_ADMIN, revisit this
  and supersede it with a new ADR.
