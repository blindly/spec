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
| `spec config` | Show effective configuration and resolved agent |
| `spec web [--port N]` | Local web UI: browse, create, edit body/status (default :8080) |

All operations are filesystem-only; no network access is required. LLM
agent commands (`draft`, `bootstrap`, `refine`) only run if you configure
or have an agent CLI installed; without one they fall back to offline
heuristics (`refine` has no fallback and reports an error).

## Agent configuration

`.spec/config.yml`:

```yaml
agent: claude          # none | claude | gemini | codex | aider | opencode | amp
agent_cmd: "mycli --flag {prompt}"   # custom command; {prompt} = prompt arg,
                                     # omitted = prompt on stdin
agent_timeout: 300     # seconds
```

Agent features are opt-in: the default is `agent: none` and no subprocess
is ever spawned unless you configure one. `agent_cmd` overrides `agent` —
useful for custom wrappers.

## Testing

```bash
go test ./...
```
