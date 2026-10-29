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
| `spec init` | Initialize spec management in a repository |
| `spec new project\|feature\|adr <title>` | Create a specification |
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
| `spec refine <id>` | LLM agent rewrites an existing spec's body in place |
| `spec apply [--run\|--force] [id…]` | Print apply prompt(s) for accepted/proposed features; `--run` executes via agent |
| `spec status` | Summarize repo spec state (counts, coverage, agent) |
| `spec fact [list\|get\|set\|unset]` | Manage `.spec/facts.yml` project facts |
| `spec check` | Audit `{fact:key}` references against facts.yml |
| `spec config` | Show effective configuration and resolved agent |
| `spec web [--port N]` | Local web UI: browse, create, edit body/status (default :8080) |

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
agent: claude          # none | claude | devin | gemini | codex | aider | opencode | amp
agent_cmd: "mycli --flag {prompt}"   # custom command; {prompt} = prompt arg,
                                     # omitted = prompt on stdin
agent_timeout: 900     # seconds
web_host: 127.0.0.1    # bind address for `spec web`; 0.0.0.0 exposes it (--host overrides)
web_port: 8080         # default port for `spec web` (--port overrides)
statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
```

Agent features are opt-in: the default is `agent: none` and no subprocess
is ever spawned unless you configure one. `agent_cmd` overrides `agent` —
useful for custom wrappers. `statuses` controls the valid status list used
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
