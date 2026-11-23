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
| `spec init` | Initialize spec management in a repository (seeds `AGENTS.md`, `CLAUDE.md`, installs the resolved skills — built-in `spec` plus any global `skills:` baseline — into the agent skill dirs, and installs a `pre-push` hook requiring a `specs/` change in any push that touches other files — a `Spec-Exempt: <reason>` commit trailer covers defect fixes, `SPEC_SKIP=1 git push` is the bypass) |
| `spec idea <title>` | Capture an untyped idea in the inbox (`specs/inbox/`) — decide the type later |
| `spec inbox [--all] [--paths]` | List inbox ideas (id, status, suggested type, title, path); `--all` includes discarded and archived |
| `spec promote <IDEA-id> [project\|feature\|adr] [--force]` | Turn an idea into a spec (defaults to its suggested type); the idea's notes go into the first section, `depends_on` references are updated, and the idea file is deleted |
| `spec discard <IDEA-id>…` | Mark ideas discarded (kept, hidden in the inbox) |
| `spec archive <id>… [--force]` | Move specs that are no longer valid (`done_statuses`: deprecated, rejected) and discarded ideas to `specs/archive/<type dir>/`, stamped `archived:`. Implemented specs are living documentation and need `--force`. Archived specs keep their ids and stay searchable and traceable, but leave the board, inbox, `status` counts, `roadmap`, and `apply` |
| `spec unarchive <id>…` | Move archived specs back |
| `spec new project\|feature\|adr\|idea <title>` | Create a specification |
| `spec bootstrap` | Generate a project spec from an existing repo |
| `spec import <file>` | Import existing documentation |
| `spec analyze` | Analyze the repository |
| `spec view [--cmd CMD] <id>…` | Print specs; optionally use an external viewer |
| `spec search <query>` | Search specifications |
| `spec validate` | Validate specs (sections, ids, links, requirements) |
| `spec export [file.json]` | Export metadata as JSON |
| `spec roadmap [file.md]` | Generate a roadmap grouped by status |
| `spec graph [--mermaid]` | Dependency graph from `depends_on` frontmatter |
| `spec trace` | Requirements traceability matrix (REQ → features) |
| `spec draft feature <title>` | Generate a feature draft (LLM agent if configured, else heuristic) |
| `spec refine <id> [--force]` | LLM agent rewrites an existing spec's body in place. For ideas: fleshes out the notes, records a `suggested_type` (new → refined), and offers to promote; without an agent, records a keyword-heuristic suggestion only. `--force` refines discarded ideas |
| `spec apply [--print\|--yes\|--no-verify\|--force] [id…]` | Run the configured agent to implement specs. Named ids may be accepted or proposed; with no ids, all accepted specs after a confirmation (`--yes` skips it). Success means the spec reached `implemented` and `verify:` passed (`--no-verify` skips it). `--print` outputs the prompt(s) |
| `spec role [edit\|refine]` | Print, edit, or improve the repo-local `.spec/role.md` development persona (refine requires an agent) |
| `spec status` | Summarize repo spec state (counts, coverage, agent, pre-push hook state); `spec ps` adds an `In flight:` list of specs not yet done |
| `spec commit [-m msg] [--exempt R]` | Stage all and commit; appends the `Spec-Exempt:` trailer the pre-push hook needs when the commit has no `specs/` change (`--exempt` reason, or auto when the message is generated) |
| `spec fact [list\|get\|set\|unset\|discover [--yes]]` | Manage public and secret project facts (`set --secret <key> <value>`) |
| `spec remind [list\|add <title> --every <period> [--spec <id>]\|done <id>\|remove <id>]` | Track recurring obligations in `.spec/reminders.yml` (`--every`: `7d`, `30d`, … or `weekly`/`monthly`/`quarterly`/`yearly`); `spec status`/`spec check` and agent prompts surface due items |
| `spec check` | Audit fact and secret references, enforce the secrets file ignore rule, fail on stale/missing skill installs, and warn about possible leaks and drift |
| `spec drift [--review [--yes]] [id…]` | List system specs whose covered code (`covers:` frontmatter) changed since they were last updated or verified. `--review` asks the agent whether each is really stale: accurate → `verified:` stamp, stale → proposed update applied on confirmation. Needs git |
| `spec covers [--infer] [id…]` | Show the files each spec covers; `--infer` fills empty `covers:` from the commits that touched each spec. `spec apply` records covers automatically |
| `spec skill` | List enabled skills with source, install status, and description (repo-owned `.spec/skills/<name>/`, managed library skills (`skills:` in config → `~/.config/spec/skills/<name>/`), the built-in `spec` skill), then the library's not-enabled entries |
| `spec skill which <name>` | Show the resolved source kind and directory (`library (not enabled)` for library entries not in `skills:`), plus `enabled in:` — this repo, `global skills:`, and registered repos enabling it |
| `spec skill diff <name>` | Unified diff of each installed copy vs its resolved source (`--library`: repo skill vs library entry) |
| `spec skill use\|unuse <name>` | Enable/disable a library skill in this repo (writes `skills:` and installs/removes generated links) |
| `spec skill adopt\|push <name>` | `adopt` forks a managed skill into `.spec/skills/`; `push` publishes a repo skill to the library (`--force` overwrites a differing shared copy) |
| `spec skill new\|edit\|view\|refine <name>` | Manage skills — `--library` targets the library file, `--local` forks to `.spec/skills/` first (refine is repo-blind for shared skills, repo-aware for repo skills); `new --no-edit` scaffolds without opening `$EDITOR` (also skipped when stdin isn't a terminal) |
| `spec skill import <name\|path\|url> [name] [--library]` | Copy a skill dir from the usual agent skill dirs, an explicit path, a spec repo root (`.spec/skills/`), or a git URL into `.spec/skills/` (or the library) |
| `spec skill check\|sync [--force] [--global]` | Check exits non-zero on missing/stale installs and notes foreign dirs in the targets (advisory); `sync` materializes them — symlinks to the resolved source (`skill_targets:`, else the configured agent's dir; the embedded built-in and link-failure fallback stay file copies); repo syncs also ensure `.gitignore` entries, `AGENTS.md`/`CLAUDE.md` pointers, and the spec-coverage `pre-push` hook exist. `--global` covers the user-level skill dirs |
| `SPEC_TRACE=1 spec …` | Print every git command spec runs, with its duration, to stderr — for diagnosing slow drift checks or `/system` loads on a given machine |
| `spec verified <id>…` | Stamp `verified: <HEAD>` — the spec was checked and is accurate; clears drift until covered code changes again |
| `spec verify` | Run configured `verify: [...]` commands (tests, validate, check) |
| `spec config` | Show effective configuration and resolved agent |
| `spec config init [global\|repo] [--force]` | Write config template with all options; `--force` regenerates |
| `spec web [--port N]` | Local web UI with three views: **inbox** (front page) to capture/promote/discard ideas, **board** of refined ideas ("ready") and work in progress by status, and **system** — implemented specs and accepted ADRs, the living documentation. Plus an archive list, create, edit body/status, README/agent docs (default :8080) |
| `spec <command> --help` / `spec help <command>` | Print one command's usage block (same text as the main help) |
| `spec completion <bash\|zsh\|fish>` | Print a shell completion script — commands, subcommands, flags, and dynamic args (spec ids, skill/persona names, fact keys). Install: `source <(spec completion bash)`, `spec completion zsh > ~/.zfunc/_spec`, `spec completion fish > ~/.config/fish/completions/spec.fish` |

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
agent_env: []          # e.g. [LANG, API_TOKEN=deploy_token]
agent_timeout: 3600     # seconds
web_host: 127.0.0.1    # bind address for `spec web`; 0.0.0.0 exposes it (--host overrides)
web_port: 8080         # default port for `spec web` (--port overrides)
statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
done_statuses: [deprecated, rejected]   # no longer valid: hidden on the web board (?all=1 shows them); implemented specs stay visible
idea_statuses: [new, refined, discarded]   # lifecycle for inbox ideas (separate from statuses)
verify: [go test ./..., spec validate, spec check]   # run by `spec verify` and after `spec apply`
editor: vim            # falls back to $EDITOR, then vi
viewer: "glow --style dark {file}" # optional; default is built-in plain output
```

`spec config init [global|repo]` writes the annotated template listing
every supported key; add `--force` to regenerate an existing file.

Agent features are opt-in: the default is `agent: none` and no subprocess
is ever spawned unless you configure one. `agent_cmd` overrides `agent` —
useful for custom wrappers. `agent_cmd` and `verify:` commands are split
into arguments with shell-style quoting (`'a b'`, `"a b"`, `a\ b`) but never
run through a shell. `statuses` controls the valid status list used
by `spec validate`, the web editor dropdown, and `spec roadmap` ordering.

An unset or empty `agent_env:` list lets agents inherit the whole environment.
A nonempty list grants the essentials (`PATH`, `HOME`, `TMPDIR`, and platform
equivalents), plus any variable named in the list. An entry like
`API_TOKEN=deploy_token` injects the secret fact `deploy_token`; a missing
secret prevents the agent from starting.

## Facts

`.spec/facts.yml` holds definite project facts as flat `key: value` pairs
(domain, phone, company name, …). Specs and templates can reference them
as `{fact:key}` — substituted at spec creation. Facts are also injected
into LLM agent prompts so drafts use real values instead of inventing
them. `spec check` reports references to undefined facts and defined
facts that nothing references.

Store credentials with `spec fact set --secret <key> <value>`. This writes
`.spec/secrets.yml` with mode `0600`. `spec init` adds the file to
`.gitignore`, and `spec check` rejects a tracked or unignored secrets file
and warns about possible leaks. Use `{secret:key}` to document a credential
in a spec. This placeholder stays literal and agent context shows secret
keys with a retrieval hint. `spec fact list` masks secret values. Use
`spec fact get <key>` to retrieve a credential at the point of use.

`spec fact discover` seeds missing placeholder keys with empty values and
proposes values from structured repository files, or from the configured
agent. Token-shaped candidates are proposed as secrets. Proposals require
confirmation (`--yes` accepts all), and existing keys are preserved.

The web UI's project badge uses the public `project_name` fact if set
(`spec fact set project_name "My Project"`), else the project spec's title,
else the git remote's repo name, else the folder name.

## Testing

```bash
go test ./...
```

`spec view` accepts active, archived, and inbox ids. `--cmd` overrides
`viewer:`; commands use quote-aware arguments without a shell. `{file}`
is replaced with the path, or the path is appended when absent. Each spec
runs its own viewer. The default prints a header and body and ignores `$PAGER`.
