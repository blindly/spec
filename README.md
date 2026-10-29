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
| `spec draft feature <title>` | Generate a feature draft, auto-linking matching requirements |
| `spec web [--port N]` | Optional local web interface (default :8080) |

All operations are filesystem-only; no network access is required.

## Testing

```bash
go test ./...
```
