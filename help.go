package main

import "strings"

// helpMarkdown is rendered on the web UI's /help page via mdBlock.
const helpMarkdown = `# Help

A spec is a markdown file under ` + "`specs/`" + ` with YAML frontmatter:

` + "```" + `
---
id: FEAT-001
type: feature          # project | feature | adr | imported | idea
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
- **idea**: none — just a title and notes

## The inbox

**Not sure what type something is yet? Capture it as an idea.** Ideas live
in the inbox (the web UI's front page, ` + "`specs/inbox/`" + `, ids ` + "`IDEA-xxx`" + `)
with their own lifecycle: **new** → **refined**, or **discarded**. When you
know what it is, promote it:

` + "```" + `
spec idea "better search"          # capture
spec inbox                         # list ideas (--all: discarded+archived too; --paths: paths only)
spec inbox cleanup                 # remove promotion debris (hand-set promoted_to:/bad status)
spec refine IDEA-003               # agent fleshes out the notes and suggests a type → refined
spec promote IDEA-003 feature      # becomes FEAT-xxx; the idea's notes go into its first section
spec discard IDEA-004              # decided against (kept, hidden in the inbox)
` + "```" + `

Refining an idea records the agent's ` + "`suggested_type`" + ` (shown as a badge in
the inbox and preselected for promote) and offers to promote right away.
Without an agent, refine only guesses the type from keywords, labelled
"heuristic". ` + "`spec promote`" + ` with no type uses the suggestion.

Promoting deletes the idea file — the new spec records ` + "`promoted_from`" + `,
and git keeps the history. Ideas are never applied. Refined ideas also show
on the board, in a first **ready** column with a promote button: they're
the candidates for what to work on next. New and discarded ideas stay in the
inbox, which is the one place to add ideas (the new spec form is for
project, feature, and adr specs).

## Inbox, board, system

The web UI has three views, one per stage:

| View | Shows | Answers |
|---|---|---|
| [inbox](/) | ideas | What might we do? |
| [board](/board) | refined ideas (ready), then work in progress: draft/proposed/accepted features, draft/proposed ADRs | What's next, and what are we working on? |
| [system](/system) | implemented features and projects, accepted ADRs, imported docs | What does the system do today? |

**Implemented specs are living documentation.** An implemented spec
describes how the system works today, so it stays in the system view for as
long as it's true. When later work changes behavior a spec describes, that
spec is updated in the same change (` + "`spec apply`" + ` tells agents to do this).
Specs that are no longer valid (` + "`done_statuses`" + `, default deprecated/rejected)
are hidden behind the board's "show" toggle.

## Drift

A spec in the system view **may be stale** when code it covers changed
after the spec was last updated. Each spec lists the files it covers in
` + "`covers:`" + ` frontmatter (exact paths, ` + "`dir/`" + `, or globs). ` + "`spec apply`" + ` records
the files the agent changed; ` + "`spec covers --infer`" + ` fills it in from git
history for older specs.

` + "```" + `
spec drift                 # list possibly-stale specs, with the commits involved
spec drift --review        # the agent checks each one: accurate → verified, stale → proposed update
spec verified FEAT-004     # checked, still accurate — clears the flag until the code changes again
` + "```" + `

Flagged specs get a "may be stale" badge on the [system](/system) view, a
warning in ` + "`spec check`" + `, and a mention in the next ` + "`spec apply`" + ` prompt.
Changing a spec in the same commit as its code keeps it in sync.
Drift detection needs git.

## Archive

When a spec stops describing the system, mark it **deprecated** (or update
it). Deprecated and rejected specs, and discarded ideas, can then be archived:

` + "```" + `
spec archive FEAT-002 FEAT-004     # → specs/archive/features/, stamped archived: <date>
spec unarchive FEAT-002            # moves it back
` + "```" + `

Implemented specs can't be archived without ` + "`--force`" + ` — they're still valid.
Archived specs keep their ids, stay searchable (` + "`spec search`" + ` marks them),
still count as REQ coverage in ` + "`spec trace`" + `, and are listed on the
[archive](/archive) page — but they're off the board, the inbox, ` + "`status`" + `
counts, ` + "`roadmap`" + `, and ` + "`apply`" + `.

## Choosing a type

Pick by the question the document answers:

| Type | Answers | Use when |
|---|---|---|
| **project** | What is this system, and what must it do? | Describing the whole system and its ` + "`REQ-xxx`" + ` requirements — usually one per repo |
| **feature** | What should we build? | Adding or changing behavior; ` + "`spec apply`" + ` implements it |
| **adr** | Why X instead of Y? | Recording a decision and the alternatives rejected; nothing to implement |
| **imported** | — | Bringing in existing docs as reference (` + "`spec import`" + `) |
| **idea** | Not sure yet | Capturing a thought before you know which of the above it is |

Quick tests:

- **Not sure yet?** → idea (promote it later)
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

A spec's type is fixed once created. It sets the id prefix (PROJ/FEAT/ADR/DOC/IDEA),
the directory, the required sections, and whether ` + "`spec apply`" + ` picks it up —
changing it later means a new spec, not an edit. ` + "`spec refine`" + ` warns
when a spec's content looks like a different type, but never changes it.
Accepted ADRs keep their original text, so the record of what was decided
stays intact:

- **A detail is out of date** (a name, a page, where something lives) but
  the decision stands → add a dated entry under a ` + "`## Corrections`" + `
  section at the end. Don't edit the original sections.
- **The decision itself changes** → write a new ADR that supersedes the old
  one, and mark the old one deprecated.

## Facts

` + "`.spec/facts.yml`" + ` holds definite project values (domain, phone,
company name, …) as flat ` + "`key: value`" + ` pairs. Reference them in spec
bodies as ` + "`{fact:key}`" + ` instead of inventing values — they are
substituted at spec creation and injected into agent prompts.
` + "`spec check`" + ` checks both placeholder types and warns about possible credential leaks.

Store credentials in the gitignored ` + "`.spec/secrets.yml`" + ` with
` + "`spec fact set --secret <key> <value>`" + `. Use ` + "`{secret:key}`" + ` in specs.
The placeholder stays literal and agents see the key in their project context.
` + "`spec fact list`" + ` masks secret values. Retrieve a value when needed with
` + "`spec fact get <key>`" + `.

A nonempty ` + "`agent_env:`" + ` list limits agents launched by spec to the listed
variables plus essentials. Use ` + "`NAME`" + ` to pass a variable through or
` + "`NAME=key`" + ` to inject a secret fact. An empty or absent list inherits the
whole environment.

Manage facts from the CLI: ` + "`spec fact list|get|set|unset|discover [--yes]`" + `.

` + "`spec fact discover`" + ` seeds missing placeholders and proposes repository values
for confirmation; ` + "`--yes`" + ` accepts all proposals. Token-shaped values are proposed as secrets.
Existing keys are preserved.

The project name in the web UI's badge comes from the public ` + "`project_name`" + ` fact
if set (` + "`spec fact set project_name \"My Project\"`" + `), else the project spec's
title, else the git remote's repo name, else the folder name.

## Roles and personas

The **role** tells the configured agent who it is and how it develops in
this repo — a persona and conventions injected into every agent prompt
(` + "`apply`" + `, ` + "`draft`" + `, ` + "`refine`" + `, ` + "`bootstrap`" + `, ` + "`fact discover`" + `, drift review).

The effective role resolves in order:

1. ` + "`.spec/role.md`" + ` — the project role, always wins when present
2. ` + "`persona: <name>`" + ` in ` + "`.spec/config.yml`" + ` — a shared persona from
   ` + "`~/.config/spec/personas/<name>.md`" + ` (a live reference: edit it once,
   every repo using it sees the change)
3. nothing — no role, prompts are unchanged

` + "```" + `
spec role                    # show the effective role (and its source)
spec role edit               # write the project role by hand
spec role refine             # agent drafts/improves it (forks a persona into the repo)
spec role adopt              # copy the active persona into .spec/role.md verbatim

spec persona                 # list the shared library (* = active)
spec persona view [name]     # cat a persona to stdout (default: the active one)
spec persona new|edit <name> # manage personas in $EDITOR
spec persona refine <name>   # agent improves it — repo-blind, stays generic
spec persona use <name>      # set persona: in .spec/config.yml (use none to clear)
` + "```" + `

Refined roles and personas are markdown documents that open with a
` + "`#`" + ` heading and a one-sentence "You are a …" persona statement; agent
narration around the document is stripped before it's saved.

Setting a role also appends a ` + "`<!-- spec:role -->`" + ` block to AGENTS.md
pointing agents at the role sources — so agent CLIs run directly in the
repo (outside ` + "`spec apply`" + `) adopt the same persona.

## CLI

` + "```" + `
spec status     # inbox + spec counts, REQ coverage
spec ls         # list specs: id, type, status, title, path (--all: archived too; --paths: paths only)
spec role       # show the effective development role (edit|refine|adopt to change it)
spec persona    # shared persona library (~/.config/spec/personas/): new|edit|refine <name>, use <name|none>, view [name]
spec idea <title>                 # capture an idea (type decided later)
spec promote <IDEA-id> [type]     # idea → project|feature|adr (default: suggested type)
spec discard <IDEA-id>
spec archive <id>...              # deprecated/rejected specs, discarded ideas → specs/archive/
spec unarchive <id>...
spec new feature <title>
spec search <query>
spec validate   # frontmatter, sections, type/id/dir match, REQ refs, cycles
spec check      # audit {fact:key} references; warn about drift
spec drift [--review]             # specs whose covered code changed since they were updated
spec covers [--infer]             # files each spec covers (infer from git history)
spec verified <id>                # mark a spec checked and accurate
spec skill      # enabled skills (source + status + description) then the
                # library's not-enabled entries — 'spec skill use <name>' enables
spec skill which <name>           # show a skill's resolved source (repo/library/built-in dir)
spec skill diff <name>            # diff installed copies vs source (--library: repo vs library)
spec skill use|unuse <name>       # enable/disable a library skill here (--global: global baseline)
spec skill adopt|push <name>      # managed → repo-owned fork · repo skill → library
spec skill rm <name>              # delete a repo skill + its installs (--library: library entry)
spec skill new|edit|view|refine <name>   # manage skills (--library: library file, --local: repo copy,
                                # new --no-edit: scaffold without opening $EDITOR)
spec skill import [<name|path|url> [<name>]]  # pull a skill into .spec/skills/ (bare: all
                                # foreign dirs; repo root or git URL picks a .spec/skills
                                # entry; --library: into the library; in-repo sources are moved)
spec skill check|sync [--force]   # verify / regenerate installs — links to the source (--global: user dirs)
spec apply <id> # run the configured agent to implement a spec
spec apply      # all accepted specs, after confirmation (--yes to skip)
                # succeeds only if status reaches implemented and verify: passes
                # --print outputs prompts · --no-verify skips verify
spec verify     # run verify: commands from .spec/config.yml
spec graph      # dependency graph (--mermaid)
spec trace      # REQ → feature coverage
spec <cmd> --help / spec help <cmd>   # one command's usage block
spec completion <bash|zsh|fish> [--install]  # print a completion script (or install it to your shell dir)
` + "```" + `
`

// commandDoc pairs a command name with its usage block — the same lines
// the main help's Usage list shows. `spec <cmd> --help` prints a
// command's block; the main help is assembled from all blocks so the
// two can't diverge.
type commandDoc struct {
	name  string
	block string
}

var commandDocs = []commandDoc{
	{"init", `  spec init                          Initialize repository`},
	{"idea", `  spec idea <title>                  Capture an untyped idea in the inbox (not sure of the type yet)`},
	{"promote", `  spec promote <id> [type] [--force] Turn an idea into a project|feature|adr spec (deletes the idea)`},
	{"discard", `  spec discard <id>...               Mark ideas as discarded`},
	{"archive", `  spec archive <id>... [--force]     Move no-longer-valid specs (deprecated/rejected) and discarded ideas to specs/archive/`},
	{"unarchive", `  spec unarchive <id>...             Move archived specs back`},
	{"new", `  spec new <project|feature|adr|idea> <title>   Create a specification`},
	{"bootstrap", `  spec bootstrap                     Bootstrap specs from existing project`},
	{"import", `  spec import <file>                 Import existing documentation`},
	{"analyze", `  spec analyze                       Analyze repository`},
	{"ls", `  spec ls [--all] [--paths]          List specs (id, type, status, title, path) — alias: list`},
	{"inbox", `  spec inbox [--all] [--paths]       List inbox ideas (id, status, suggested type, title, path)`},
	{"search", `  spec search <query>                Search specifications`},
	{"validate", `  spec validate                      Validate specifications`},
	{"export", `  spec export [file.json]            Export metadata as JSON`},
	{"roadmap", `  spec roadmap [file.md]             Generate roadmap`},
	{"graph", `  spec graph [--mermaid]             Dependency graph`},
	{"trace", `  spec trace                         Requirements traceability`},
	{"draft", `  spec draft feature <title>         Generate feature draft (LLM agent if configured)`},
	{"view", `  spec view [--cmd CMD] <id>...       Print specs (or use an external viewer)`},
	{"edit", `  spec edit <id>                     Open a spec in your editor`},
	{"refine", `  spec refine <id> [--force]         LLM agent improves an existing spec; for ideas, also
                                       suggests a type and offers to promote (--force: discarded ideas)`},
	{"apply", `  spec apply [flags] [id...]         Run agent to implement specs (no ids: all accepted, after confirmation)
                                       --print outputs prompts · --yes skips confirmation
                                       --no-verify skips verify · --force allows any status`},
	{"role", `  spec role [edit|refine|adopt]      Show/edit/refine the effective role (.spec/role.md or persona);
                                       adopt copies the active persona into .spec/role.md`},
	{"persona", `  spec persona [new|edit|refine <name>|use <name|none>|view [name]]   Manage the shared
                                       persona library (~/.config/spec/personas/); use selects
                                       one for this repo; view cats one (default: active) to stdout`},
	{"status", `  spec status                        Summarize repository spec state`},
	{"ps", `  spec ps                            List specs in flight (not yet done) — the queue, not the dashboard`},
	{"commit", `  spec commit [-m msg] [--exempt R] [--agent]   Stage all and commit, conventional-commit
                                       style; an uncovered commit needs a non-feat: subject or a
                                       Spec-Exempt trailer (--exempt) or the push will fail;
                                       --agent asks the configured agent to write the message
                                       and pick the attestation from the staged diff`},
	{"fact", `  spec fact [list|get <key>|set [--secret] <key> <value>|unset <key>|discover [--yes]]     Manage public and secret project facts`},
	{"remind", `  spec remind [list|add <title> --every <7d|30d|weekly|monthly|quarterly|yearly> [--spec <id>]|done <id>|remove <id>]
                                       Manage recurring task reminders (.spec/reminders.yml)`},
	{"check", `  spec check                         Audit facts, secrets, skill installs, and drift`},
	{"drift", `  spec drift [--review [--yes]] [id...]   Specs whose covered code changed since they were last updated;
                                       --review asks the agent if each is stale and proposes an update`},
	{"covers", `  spec covers [--infer] [id...]      Show (or infer from git history) the files each spec covers`},
	{"verified", `  spec verified <id>...              Mark specs as checked and accurate (clears drift until code changes)`},
	{"skill", `  spec skill                           List enabled skills (source + status + description),
                                       then the library's not-enabled entries
  spec skill which <name>               Show a skill's resolved source (repo/library/built-in dir)
  spec skill diff <name>                Diff installed copies vs source (--library: repo vs library)
  spec skill use|unuse <name>           Enable/disable a library skill here (--global: global baseline)
  spec skill adopt|push <name>          managed → repo-owned fork · repo skill → library
  spec skill rm <name>                  Delete a repo skill + its installs (--library: library entry)
  spec skill new|edit|view|refine <name>   Manage skills (--library: library file, --local: repo copy,
                                       new --no-edit: scaffold without opening $EDITOR)
  spec skill import [<name|path|url> [<name>]]   Pull a skill into .spec/skills/ (bare: all foreign
                                       dirs; repo root or git URL picks a .spec/skills entry;
                                       --library: into the library; in-repo sources are moved)
  spec skill check|sync [--force]       Verify / regenerate installs — links to the source (--global: user dirs)`},
	{"verify", `  spec verify                        Run configured verification commands`},
	{"config", `  spec config                        Show effective configuration
  spec config init [global|repo] [--force]   Write config template (--force regenerates)
  spec config edit [global|repo]     Open config file in $EDITOR (or editor: key)`},
	{"web", `  spec web [--port N] [--host ADDR]   Web interface (defaults: web_port/web_host from config)`},
	{"update", `  spec update [tag]                  Self-update to a release (default: latest)`},
	{"version", `  spec version                       Print version`},
	{"help", `  spec help [command]                 Full help, or one command's help block`},
	{"completion", `  spec completion <bash|zsh|fish> [--install]   Print a completion script (or install it to your shell dir)`},
}

var commandDocByName = func() map[string]string {
	m := map[string]string{}
	for _, d := range commandDocs {
		m[d.name] = d.block
	}
	m["list"] = m["ls"]
	return m
}()

var usage = func() string {
	var b strings.Builder
	b.WriteString("spec — offline specification management\n\nUsage:\n")
	for _, d := range commandDocs {
		b.WriteString(d.block)
		b.WriteString("\n")
	}
	b.WriteString(`
Agent configuration lives in .spec/config.yml:
  agent: none|claude|devin|gemini|codex|aider|opencode|amp|cline|vibe   (default: none)
  agent_cmd: "mycli --flag {prompt}" # {prompt} = prompt arg · {prompt-file} = temp file
  agent_env: [LANG, API_TOKEN=deploy_token] # empty inherits the environment
                                       holding the prompt · absent = prompt via stdin
  agent_timeout: 3600                 # seconds
`)
	return b.String()
}()
