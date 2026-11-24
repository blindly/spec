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
	Persona      string   // name of a ~/.config/spec/personas/<name>.md used as the role when .spec/role.md is unset
	VersionStamp bool     // spec commit refreshes VERSION (opt-in by the file's presence); spec release stamps + tags
	Release      bool     // spec release allowed — VERSION opts into stamping, this opts into tagging
	CommitAgent  bool     // spec commit asks the agent for the message by default (--no-agent overrides)
	Skills       []string // library skills (~/.config/spec/skills/<name>/) enabled for this repo
	SkillTargets []string // agent skill dirs installed copies are generated into
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
		VersionStamp: true,
		Statuses:     defaultStatuses, DoneStatuses: defaultDoneStatuses, IdeaStatuses: defaultIdeaStatuses}
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
	for k, v := range raw {
		switch k {
		case "agent":
			cfg.Agent = strings.Trim(v, `"'`)
		case "agent_cmd":
			cfg.AgentCmd = unquote(v)
		case "agent_env":
			cfg.AgentEnv = parseList(v)
			if v != "" && (!strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]")) {
				cfg.AgentEnv = []string{""} // reject malformed lists before launching an agent
			}
		case "agent_timeout":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.AgentTimeout = n
			}
		case "viewer":
			cfg.Viewer = unquote(v)
		case "editor":
			cfg.Editor = strings.Trim(v, `"'`)
		case "persona":
			cfg.Persona = strings.Trim(v, `"'`)
		case "version_stamp":
			cfg.VersionStamp = v != "false" && v != "no" && v != "0" && v != "off"
		case "commit_agent":
			cfg.CommitAgent = v == "true" || v == "yes" || v == "1" || v == "on"
		case "web_host":
			cfg.WebHost = strings.Trim(v, `"'`)
		case "web_port":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.WebPort = n
			}
		case "statuses", "verify", "done_statuses", "idea_statuses", "skill_targets":
			if list := parseList(v); len(list) > 0 {
				switch k {
				case "statuses":
					cfg.Statuses = list
				case "done_statuses":
					cfg.DoneStatuses = list
				case "idea_statuses":
					cfg.IdeaStatuses = list
				case "skill_targets":
					cfg.SkillTargets = list
				default:
					cfg.Verify = list
				}
			}
		}
	}
	cfg.Skills = unionStrings(parseList(globalRaw["skills"]), parseList(repoRaw["skills"]))
	// release is deliberately repo-only — a global release: true would
	// silently enable tagging in every repo.
	v := repoRaw["release"]
	cfg.Release = v == "true" || v == "yes" || v == "1" || v == "on"
	return cfg
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
	return fmt.Sprintf("agent: %s\nagent_cmd: %s\nagent_env: [%s]\nagent_timeout: %d\nweb_host: %s\nweb_port: %d\neditor: %s\nviewer: %s\npersona: %s\nskills: [%s]\nskill_targets: [%s]\nstatuses: [%s]\ndone_statuses: [%s]\nidea_statuses: [%s]\nverify: [%s]\nversion_stamp: %t\nrelease: %t\ncommit_agent: %t",
		c.Agent, c.AgentCmd, strings.Join(c.AgentEnv, ", "), c.AgentTimeout, c.WebHost, c.WebPort, c.editor(), c.Viewer, c.Persona,
		strings.Join(c.Skills, ", "), strings.Join(c.SkillTargets, ", "),
		strings.Join(c.Statuses, ", "), strings.Join(c.DoneStatuses, ", "),
		strings.Join(c.IdeaStatuses, ", "), strings.Join(c.Verify, ", "), c.VersionStamp, c.Release, c.CommitAgent)
}

const globalConfigTemplate = `# Global defaults for all spec-managed repositories.
# Repo-level .spec/config.yml overrides these per-key.

# Agent (opt-in; none = no subprocess ever runs)
# agent: claude        # claude | devin | gemini | codex | aider | opencode | amp | cline | vibe
# agent_cmd: mycli --flag {prompt}   # {prompt} = prompt arg; {prompt-file} = temp file; absent = stdin
# agent_env: [LANG, API_TOKEN=deploy_token]  # empty inherits the environment
# agent_timeout: 3600   # seconds
# commit_agent: true    # spec commit asks the agent for the message by default

# Web
# web_host: 127.0.0.1  # use 0.0.0.0 to expose on all interfaces
# web_port: 8080

# Lifecycle
# statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
# done_statuses: [deprecated, rejected]   # no longer valid: hidden on the board by default (implemented specs stay visible)
# idea_statuses: [new, refined, discarded]   # lifecycle for inbox ideas

# Verification — run by spec verify and after spec apply (no shell; quotes group args)
# verify: [go test ./..., spec validate, spec check]

# Editor for spec edit / spec config edit (falls back to $EDITOR, then vi)
# editor: vim

# Persona — a shared development role from ~/.config/spec/personas/<name>.md,
# used in agent prompts when the repo has no .spec/role.md (which always wins)
# persona: senior-go   # see: spec persona

# Skills — a global baseline of library skills (~/.config/spec/skills/<name>/)
# enabled for every repo; repos can replace the list or add repo-owned
# skills under .spec/skills/
# skills: [token-efficiency]   # see: spec skill
# skill_targets: [.devin/skills, .claude/skills]   # install dirs (default: the configured agent's dir, else both)
# skill_global_targets: [~/.claude/skills, ~/.config/devin/skills]   # user-level dirs for --global (default: agent's dir, else both)

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
# commit_agent: true    # spec commit asks the agent for the message by default

# Web
# web_host: 127.0.0.1   # use 0.0.0.0 to expose on all interfaces
# web_port: 8080

# Lifecycle
# statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
# done_statuses: [deprecated, rejected]   # no longer valid: hidden on the board by default (implemented specs stay visible)
# idea_statuses: [new, refined, discarded]   # lifecycle for inbox ideas

# Verification — run by spec verify and after spec apply (no shell; quotes group args)
# verify: [go test ./..., spec validate, spec check]

# Editor for spec edit / spec config edit (falls back to $EDITOR, then vi)
# editor: vim

# Persona — a shared development role from ~/.config/spec/personas/<name>.md,
# used in agent prompts when the repo has no .spec/role.md (which always wins)
# persona: senior-go   # see: spec persona

# Skills — library skills (~/.config/spec/skills/<name>/) enabled for this
# repo; repo-owned skills under .spec/skills/ need no entry here
# skills: [review]             # see: spec skill
# skill_targets: [.devin/skills, .claude/skills]   # install dirs (default: the configured agent's dir, else both)

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
