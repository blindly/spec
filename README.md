# spec

Offline-first specification management CLI written in Go (stdlib only, no
dependencies). Manages project specs, feature specs, and ADRs as Markdown
files with frontmatter under `specs/`.

## Install

```bash
go build -o spec .
```

## Commands

| Command | Description |
| --- | --- |
| `spec init` | Initialize spec management in a repository (seeds `AGENTS.md`, `CLAUDE.md`, and a `spec` skill under `.devin/skills/` + `.claude/skills/`) |
| `spec idea <title>` | Capture an untyped idea in the inbox (`specs/inbox/`) — decide the type later |
| `spec promote <IDEA-id> [project\|feature\|adr] [--force]` | Turn an idea into a spec (defaults to its suggested type); the idea's notes go into the first section, `depends_on` references are updated, and the idea file is deleted |
| `spec discard <IDEA-id>…` | Mark ideas discarded (kept, hidden in the inbox) |
| `spec archive <id>… [--force]` | Move specs that are no longer valid (`done_statuses`: deprecated, rejected) and discarded ideas to `specs/archive/<type dir>/`, stamped `archived:`. Implemented specs are living documentation and need `--force`. Archived specs keep their ids and stay searchable and traceable, but leave the board, inbox, `status` counts, `roadmap`, and `apply` |
| `spec unarchive <id>…` | Move archived specs back |
| `spec new project\|feature\|adr\|idea <title>` | Create a specification |
| `spec bootstrap` | Generate a project spec from an existing repo |
| `spec import <file>` | Import existing documentation |
| `spec analyze` | Analyze the repository |
| `spec search <query>` | Search specifications |
| `spec validate` | Validate specs (sections, ids, links, requirements) |
| `spec export [file.json]` | Export metadata as JSON |
| `spec roadmap [file.md]` | Generate a roadmap grouped by status |
| `spec graph [--mermaid]` | Dependency graph from `depends_on` frontmatter |
| `spec trace` | Requirements traceability matrix (REQ → features) |
| `spec draft feature <title>` | Generate a feature draft (LLM agent if configured, else heuristic) |
| `spec refine <id> [--force]` | LLM agent rewrites an existing spec's body in place. For ideas: fleshes out the notes, records a `suggested_type` (new → refined), and offers to promote; without an agent, records a keyword-heuristic suggestion only. `--force` refines discarded ideas |
| `spec apply [--print\|--yes\|--no-verify\|--force] [id…]` | Run the configured agent to implement specs. Named ids may be accepted or proposed; with no ids, all accepted specs after a confirmation (`--yes` skips it). Success means the spec reached `implemented` and `verify:` passed (`--no-verify` skips it). `--print` outputs the prompt(s) |
| `spec status` | Summarize repo spec state (counts, coverage, agent) |
| `spec fact [list\|get\|set\|unset]` | Manage `.spec/facts.yml` project facts |
| `spec check` | Audit `{fact:key}` references against facts.yml |
| `spec verify` | Run configured `verify: [...]` commands (tests, validate, check) |
| `spec config` | Show effective configuration and resolved agent |
| `spec config init [global\|repo] [--force]` | Write config template with all options; `--force` regenerates |
| `spec web [--port N]` | Local web UI with three views: **inbox** (front page) to capture/promote/discard ideas, **board** of work in progress by status, and **system** — implemented specs and accepted ADRs, the living documentation. Plus an archive list, create, edit body/status, README/agent docs (default :8080) |

All operations are filesystem-only; no network access is required. LLM
agent commands (`draft`, `bootstrap`, `refine`) only run if you configure
or have an agent CLI installed; without one they fall back to offline
heuristics (`refine` has no fallback and reports an error).

## Agent configuration

Config is layered: `~/.config/spec/config.yml` (global, per-user) is read
first, then `.spec/config.yml` in the repo overrides it key-by-key. Set
your agent once globally and only override per repo when needed.

`.spec/config.yml` (repo) / `~/.config/spec/config.yml` (global):

```yaml
agent: claude          # none | claude | devin | gemini | codex | aider | opencode | amp | cline | vibe
agent_cmd: "mycli --flag {prompt}"   # custom command; {prompt} = prompt arg,
                                     # omitted = prompt on stdin
agent_timeout: 3600     # seconds
web_host: 127.0.0.1    # bind address for `spec web`; 0.0.0.0 exposes it (--host overrides)
web_port: 8080         # default port for `spec web` (--port overrides)
statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
done_statuses: [deprecated, rejected]   # no longer valid: hidden on the web board (?all=1 shows them); implemented specs stay visible
idea_statuses: [new, refined, discarded]   # lifecycle for inbox ideas (separate from statuses)
verify: [go test ./..., spec validate, spec check]   # run by `spec verify` and after `spec apply`
editor: vim            # falls back to $EDITOR, then vi
```

`spec config init [global|repo]` writes the annotated template listing
every supported key; add `--force` to regenerate an existing file.

Agent features are opt-in: the default is `agent: none` and no subprocess
is ever spawned unless you configure one. `agent_cmd` overrides `agent` —
useful for custom wrappers. `agent_cmd` and `verify:` commands are split
into arguments with shell-style quoting (`'a b'`, `"a b"`, `a\ b`) but never
run through a shell. `statuses` controls the valid status list used
by `spec validate`, the web editor dropdown, and `spec roadmap` ordering.

## Facts

`.spec/facts.yml` holds definite project facts as flat `key: value` pairs
(domain, phone, company name, …). Specs and templates can reference them
as `{fact:key}` — substituted at spec creation. Facts are also injected
into LLM agent prompts so drafts use real values instead of inventing
them. `spec check` reports references to undefined facts and defined
facts that nothing references.

## Testing

```bash
go test ./...
```
