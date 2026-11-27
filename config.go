package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is loaded from .spec/config.yml (flat "key: value" lines, "#" comments).
type Config struct {
	Agent        string   // none | adapter name
	AgentCmd     string   // custom command; {prompt} is replaced by the prompt, {prompt-file} by a temp file holding it, else prompt goes to stdin
	AgentEnv     []string // pass-through names and NAME=secret_key grants
	AgentTimeout int      // seconds
	WebPort      int      // default port for `spec web`
	WebHost      string   // bind address for `spec web` (127.0.0.1 or 0.0.0.0)
	Statuses     []string // valid spec statuses
	DoneStatuses []string // statuses that count as finished (hidden on the board by default)
	IdeaStatuses []string // valid statuses for inbox ideas (type idea)
	Viewer       string   // external renderer for spec view; empty uses built-in output
	Editor       string   // editor command for `spec config edit`
	Verify       []string // commands run by spec verify
	PushCommand  string   // custom push command template ({branch}, {ref} placeholders); empty = git push origin <ref>
	PushOnCommit bool     // spec commit pushes after a successful commit (repo-only — push policy is clone state)
	PreCommit    []string // cheap checks run by spec commit / spec precheck
	Persona      string   // name of a ~/.config/spec/personas/<name>.md used as the role when .spec/role.md is unset
	VersionStamp bool     // spec commit refreshes VERSION (opt-in by the file's presence); spec release stamps + tags
	Release      bool     // spec release allowed — VERSION opts into stamping, this opts into tagging
	CommitAgent  bool     // spec commit asks a configured agent for the message by default (--no-agent or commit_agent: false disables)
	Skills       []string // library skills (~/.config/spec/skills/<name>/) enabled for this repo
	SkillTargets []string // agent skill dirs installed copies are generated into
	SkillAgents  []string // additional agents whose skill dirs join the targets
}

var (
	defaultStatuses = []string{"draft", "proposed", "accepted", "implemented", "deprecated", "rejected"}
	// implemented is deliberately not "done": an implemented spec is the
	// living description of the system (ADR-003)
	defaultDoneStatuses = []string{"deprecated", "rejected"}
	defaultIdeaStatuses = []string{"new", "refined", "discarded"}
)

func defaultConfig() Config {
	return Config{Agent: "none", AgentTimeout: 3600, WebPort: 8080, WebHost: "127.0.0.1",
		VersionStamp: true, CommitAgent: true,
		Statuses: defaultStatuses, DoneStatuses: defaultDoneStatuses, IdeaStatuses: defaultIdeaStatuses}
}

// statusesFor returns the status list that applies to a spec type: ideas
// have their own lifecycle, everything else uses statuses.
func (c Config) statusesFor(specType string) []string {
	if specType == "idea" {
		return c.IdeaStatuses
	}
	return c.Statuses
}

// validStatusFor reports whether status is valid for a spec of specType.
func (c Config) validStatusFor(specType, status string) bool {
	for _, v := range c.statusesFor(specType) {
		if v == status {
			return true
		}
	}
	return false
}

// isDone reports whether status counts as finished work.
func (c Config) isDone(status string) bool {
	for _, s := range c.DoneStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// globalConfigPath is the user-level config; repo .spec/config.yml overrides it.
func globalConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "spec", "config.yml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "spec", "config.yml")
}

// parseConfigFile returns the raw key→value pairs in a config file.
func parseConfigFile(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		// strip inline comments ("key: value  # note")
		if i := strings.Index(v, " #"); i >= 0 {
			v = v[:i]
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

// unquote strips one matching pair of outer quotes, keeping inner quotes
// (e.g. agent_cmd: "mycli --model 'gpt 5' {prompt}") intact.
func unquote(v string) string {
	if n := len(v); n >= 2 && (v[0] == '"' || v[0] == '\'') && v[n-1] == v[0] {
		return v[1 : n-1]
	}
	return v
}

// parseList parses a "[a, b, c]" config list value; nil when not a list.
func parseList(v string) []string {
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil
	}
	var list []string
	for _, item := range strings.Split(v[1:len(v)-1], ",") {
		if item = strings.TrimSpace(item); item != "" {
			list = append(list, item)
		}
	}
	return list
}

// loadConfig merges the global config (~/.config/spec/config.yml) with the
// repo's .spec/config.yml; repo keys win — except skills:, which unions
// (a repo's entries extend the global baseline rather than replacing it).
func loadConfig(root string) Config {
	cfg := defaultConfig()
	globalRaw := parseConfigFile(globalConfigPath())
	repoRaw := parseConfigFile(repoConfigPath(root))
	raw := map[string]string{}
	for k, v := range globalRaw {
		raw[k] = v
	}
	for k, v := range repoRaw {
		raw[k] = v
	}
	for _, k := range configKeys {
		if k.set == nil {
			continue // applied elsewhere (skills union, skill_global_targets)
		}
		src := raw
		if k.repoOnly {
			src = repoRaw
		}
		if v, ok := src[k.name]; ok {
			k.set(&cfg, v)
		}
	}
	cfg.Skills = unionStrings(parseList(globalRaw["skills"]), parseList(repoRaw["skills"]))
	return cfg
}

// configKey is one supported config.yml key: its merge scope, a one-line
// doc for `spec config keys`, and the setter loadConfig applies when the
// key is present. The registry drives both parsing and documentation so
// a key can't exist in one place without the other (FEAT-043).
type configKey struct {
	name     string
	scope    string // global+repo | repo | global | union — shown by spec config keys
	repoOnly bool   // read only from the repo file — a global value is ignored
	doc      string
	set      func(*Config, string) // nil = documented here, applied elsewhere
}

func truthy(v string) bool { return v == "true" || v == "yes" || v == "1" || v == "on" }
func notFalsey(v string) bool {
	return v != "false" && v != "no" && v != "0" && v != "off"
}

func listSetter(get func(*Config) *[]string) func(*Config, string) {
	return func(c *Config, v string) {
		if list := parseList(v); len(list) > 0 {
			*get(c) = list
		}
	}
}

var configKeys = []configKey{
	{"agent", "global+repo", false, "agent adapter — none|claude|devin|gemini|codex|aider|opencode|amp|cline|vibe",
		func(c *Config, v string) { c.Agent = strings.Trim(v, `"'`) }},
	{"agent_cmd", "global+repo", false, "custom agent argv — {prompt}=arg, {prompt-file}=temp file, absent=stdin",
		func(c *Config, v string) { c.AgentCmd = unquote(v) }},
	{"agent_env", "global+repo", false, "env for the agent — bare NAME passes through, NAME=secret_key grants a secret; empty list inherits all",
		func(c *Config, v string) {
			c.AgentEnv = parseList(v)
			if v != "" && (!strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]")) {
				c.AgentEnv = []string{""} // reject malformed lists before launching an agent
			}
		}},
	{"agent_timeout", "global+repo", false, "agent subprocess timeout, seconds",
		func(c *Config, v string) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				c.AgentTimeout = n
			}
		}},
	{"statuses", "global+repo", false, "valid spec statuses",
		listSetter(func(c *Config) *[]string { return &c.Statuses })},
	{"done_statuses", "global+repo", false, "statuses counting as finished (hidden on the web board)",
		listSetter(func(c *Config) *[]string { return &c.DoneStatuses })},
	{"idea_statuses", "global+repo", false, "inbox idea lifecycle statuses",
		listSetter(func(c *Config) *[]string { return &c.IdeaStatuses })},
	{"verify", "global+repo", false, "checks run by spec verify, after spec apply, spec commit --verify",
		listSetter(func(c *Config) *[]string { return &c.Verify })},
	{"pre_commit", "global+repo", false, "fast checks run by spec commit, spec precheck, the pre-commit hook",
		listSetter(func(c *Config) *[]string { return &c.PreCommit })},
	{"skill_targets", "global+repo", false, "extra repo dirs skills install into — the agent's dir and skill_agents dirs are always targets",
		listSetter(func(c *Config) *[]string { return &c.SkillTargets })},
	{"skill_agents", "global+repo", false, "agents whose skill dirs are install targets (claude, devin, codex, ...) — repo list replaces global",
		listSetter(func(c *Config) *[]string { return &c.SkillAgents })},
	{"skills", "union", false, "library skills enabled — global baseline ∪ repo list", nil},
	{"persona", "global+repo", false, "persona from ~/.config/spec/personas/ used as this repo's role",
		func(c *Config, v string) { c.Persona = strings.Trim(v, `"'`) }},
	{"commit_agent", "global+repo", false, "agent writes spec commit messages by default",
		func(c *Config, v string) { c.CommitAgent = notFalsey(v) }},
	{"version_stamp", "global+repo", false, "spec commit refreshes VERSION when the repo has one",
		func(c *Config, v string) { c.VersionStamp = notFalsey(v) }},
	{"web_host", "global+repo", false, "spec web bind address",
		func(c *Config, v string) { c.WebHost = strings.Trim(v, `"'`) }},
	{"web_port", "global+repo", false, "spec web port",
		func(c *Config, v string) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				c.WebPort = n
			}
		}},
	{"editor", "global+repo", false, "editor for spec edit / spec config edit ($EDITOR fallback)",
		func(c *Config, v string) { c.Editor = strings.Trim(v, `"'`) }},
	{"viewer", "global+repo", false, "spec view renderer — {file} placeholder; built-in plain output default",
		func(c *Config, v string) { c.Viewer = unquote(v) }},
	{"release", "repo", true, "allow spec release to tag+push (repo only — global would tag every repo)",
		func(c *Config, v string) { c.Release = truthy(v) }},
	{"push_command", "repo", true, "push argv template for spec push/spec release — {ref}/{branch} expand (repo only)",
		func(c *Config, v string) { c.PushCommand = unquote(v) }},
	{"push_on_commit", "repo", true, "spec commit pushes (via spec push) after each commit (repo only)",
		func(c *Config, v string) { c.PushOnCommit = truthy(v) }},
	{"skill_global_targets", "global", false, "user-level skill install dirs for --global (default: agent's dir, else both) — parsed in skill.go", nil},
}

// repoConfigPath is the repo's .spec/config.yml.
func repoConfigPath(root string) string {
	return filepath.Join(root, configDir, "config.yml")
}

// configFileSkills returns the skills: list of one config file.
func configFileSkills(path string) []string {
	return parseList(parseConfigFile(path)["skills"])
}

func unionStrings(a, b []string) []string {
	out := append([]string{}, a...)
	for _, s := range b {
		if !contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func (c Config) String() string {
	return fmt.Sprintf("agent: %s\nagent_cmd: %s\nagent_env: [%s]\nagent_timeout: %d\nweb_host: %s\nweb_port: %d\neditor: %s\nviewer: %s\npersona: %s\nskills: [%s]\nskill_agents: [%s]\nskill_targets: [%s]\nstatuses: [%s]\ndone_statuses: [%s]\nidea_statuses: [%s]\nverify: [%s]\npre_commit: [%s]\npush_command: %s\npush_on_commit: %t\nversion_stamp: %t\nrelease: %t\ncommit_agent: %t",
		c.Agent, c.AgentCmd, strings.Join(c.AgentEnv, ", "), c.AgentTimeout, c.WebHost, c.WebPort, c.editor(), c.Viewer, c.Persona,
		strings.Join(c.Skills, ", "), strings.Join(c.SkillAgents, ", "), strings.Join(c.SkillTargets, ", "),
		strings.Join(c.Statuses, ", "), strings.Join(c.DoneStatuses, ", "),
		strings.Join(c.IdeaStatuses, ", "), strings.Join(c.Verify, ", "), strings.Join(c.PreCommit, ", "), c.PushCommand, c.PushOnCommit, c.VersionStamp, c.Release, c.CommitAgent)
}

const globalConfigTemplate = `# Global defaults for all spec-managed repositories.
# Repo-level .spec/config.yml overrides these per-key.

# Agent (opt-in; none = no subprocess ever runs)
# agent: claude        # claude | devin | gemini | codex | aider | opencode | amp | cline | vibe
# agent_cmd: mycli --flag {prompt}   # {prompt} = prompt arg; {prompt-file} = temp file; absent = stdin
# agent_env: [LANG, API_TOKEN=deploy_token]  # empty inherits the environment
# agent_timeout: 3600   # seconds
# commit_agent: false   # opt out of agent-written commit messages (default: on when an agent is configured)

# Web
# web_host: 127.0.0.1  # use 0.0.0.0 to expose on all interfaces
# web_port: 8080

# Lifecycle
# statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
# done_statuses: [deprecated, rejected]   # no longer valid: hidden on the board by default (implemented specs stay visible)
# idea_statuses: [new, refined, discarded]   # lifecycle for inbox ideas

# Verification — run by spec verify and after spec apply (no shell; quotes group args)
# verify: [go test ./..., spec validate, spec check]

# Pre-commit — fast checks run by spec commit and the pre-commit hook
# (keep them cheap; the full suite stays in verify:)
# pre_commit: [ruff check, biome check, tsc --noEmit]

# Editor for spec edit / spec config edit (falls back to $EDITOR, then vi)
# editor: vim

# Persona — a shared development role from ~/.config/spec/personas/<name>.md,
# used in agent prompts when the repo has no .spec/role.md (which always wins)
# persona: senior-go   # see: spec persona

# Skills — a global baseline of library skills (~/.config/spec/skills/<name>/)
# enabled for every repo; repos can replace the list or add repo-owned
# skills under .spec/skills/
# skills: [token-efficiency]   # see: spec skill
# skill_agents: [devin]        # extra agents whose skill dirs are targets (agent: always counts)
# skill_targets: [.other/skills]   # extra install dirs beyond the agents' own
# skill_global_targets: [~/.claude/skills, ~/.config/devin/skills]   # extra user-level dirs for --global

# Releases — a VERSION file opts into version stamping: spec commit
# refreshes it and spec release tags v<stamp>; set false to manage it by hand
# version_stamp: true

# Viewer for spec view (default: built-in plain output; no $PAGER)
# viewer: "glow --style dark {file}"   # {file} = path; absent = appended
`

const repoConfigTemplate = `version: 1

# Repo-level overrides. Global defaults live in ~/.config/spec/config.yml —
# any key here wins over the same key there.

# Agent (opt-in; none = no subprocess ever runs)
# agent: claude        # claude | devin | gemini | codex | aider | opencode | amp | cline | vibe
# agent_cmd: mycli --flag {prompt}   # {prompt} = prompt arg; {prompt-file} = temp file; absent = stdin
# agent_env: [LANG, API_TOKEN=deploy_token]  # empty inherits the environment
# agent_timeout: 3600   # seconds
# commit_agent: false   # opt out of agent-written commit messages (default: on when an agent is configured)

# Web
# web_host: 127.0.0.1   # use 0.0.0.0 to expose on all interfaces
# web_port: 8080

# Lifecycle
# statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
# done_statuses: [deprecated, rejected]   # no longer valid: hidden on the board by default (implemented specs stay visible)
# idea_statuses: [new, refined, discarded]   # lifecycle for inbox ideas

# Verification — run by spec verify and after spec apply (no shell; quotes group args)
# verify: [go test ./..., spec validate, spec check]

# Pre-commit — fast checks run by spec commit and the pre-commit hook
# (keep them cheap; the full suite stays in verify:)
# pre_commit: [ruff check, biome check, tsc --noEmit]

# Push — spec push and spec release run this instead of git push origin
# <ref>; {branch} = current branch, {ref} = the ref being pushed.
# Unparseable commands make the coverage gate check commits not on any remote.
# push_command: git push gitea {branch}
# push_on_commit: true   # spec commit pushes (via spec push) after committing

# Editor for spec edit / spec config edit (falls back to $EDITOR, then vi)
# editor: vim

# Persona — a shared development role from ~/.config/spec/personas/<name>.md,
# used in agent prompts when the repo has no .spec/role.md (which always wins)
# persona: senior-go   # see: spec persona

# Skills — library skills (~/.config/spec/skills/<name>/) enabled for this
# repo; repo-owned skills under .spec/skills/ need no entry here
# skills: [review]             # see: spec skill
# skill_agents: [devin]        # extra agents whose skill dirs are targets (agent: always counts)
# skill_targets: [.other/skills]   # extra install dirs beyond the agents' own

# Releases — a VERSION file opts into version stamping: spec commit
# refreshes it; set false to manage it by hand
# version_stamp: true
# release: false            # spec release requires this — tags v<stamp> on release

# Viewer for spec view (default: built-in plain output; no $PAGER)
# viewer: "glow --style dark {file}"   # {file} = path; absent = appended
`

// initGlobalConfig writes the annotated template; refuses to overwrite
// unless force is set.
func initGlobalConfig(force bool) (string, error) {
	path := globalConfigPath()
	if path == "" {
		return "", fmt.Errorf("cannot determine home directory")
	}
	if _, err := os.Stat(path); err == nil && !force {
		return "", fmt.Errorf("%s already exists (use --force to regenerate)", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(globalConfigTemplate), 0o644)
}

// initRepoConfig writes .spec/config.yml from the annotated template;
// refuses to overwrite unless force is set.
func initRepoConfig(root string, force bool) (string, error) {
	path := filepath.Join(root, configDir, "config.yml")
	if _, err := os.Stat(path); err == nil && !force {
		return "", fmt.Errorf("%s already exists (use --force to regenerate)", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(repoConfigTemplate), 0o644)
}

// setConfigKey writes key: value into the repo's .spec/config.yml.
func setConfigKey(root, key, value string) error {
	return setConfigKeyIn(repoConfigPath(root), key, value)
}

// setConfigKeyIn writes key: value into the config file at path, replacing
// the existing uncommented line for that key or dropping it when value is
// "". Commented-out lines mentioning the key are untouched.
func setConfigKeyIn(path, key, value string) error {
	data, _ := os.ReadFile(path)
	var lines []string
	replaced := false
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		k, _, found := strings.Cut(line, ":")
		if found && strings.TrimSpace(k) == key && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			if value != "" {
				lines = append(lines, key+": "+value)
			}
			replaced = true
			continue
		}
		if line != "" || len(lines) > 0 {
			lines = append(lines, line)
		}
	}
	if !replaced && value != "" {
		lines = append(lines, key+": "+value)
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// editor resolves the configured editor: config key, then $EDITOR, then vi.
func (c Config) editor() string {
	if c.Editor != "" {
		return c.Editor
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// validStatus reports whether s is in the configured status list.
func (c Config) validStatus(s string) bool {
	for _, v := range c.Statuses {
		if v == s {
			return true
		}
	}
	return false
}
