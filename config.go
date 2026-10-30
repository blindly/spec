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
	AgentCmd     string   // custom command; {prompt} is replaced by the prompt, else prompt goes to stdin
	AgentTimeout int      // seconds
	WebPort      int      // default port for `spec web`
	WebHost      string   // bind address for `spec web` (127.0.0.1 or 0.0.0.0)
	Statuses     []string // valid spec statuses
	Editor       string   // editor command for `spec config edit`
	Verify       []string // commands run by spec verify
}

var defaultStatuses = []string{"draft", "proposed", "accepted", "implemented", "deprecated", "rejected"}

func defaultConfig() Config {
	return Config{Agent: "none", AgentTimeout: 900, WebPort: 8080, WebHost: "127.0.0.1", Statuses: defaultStatuses}
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

// loadConfig merges the global config (~/.config/spec/config.yml) with the
// repo's .spec/config.yml; repo keys win.
func loadConfig(root string) Config {
	cfg := defaultConfig()
	raw := parseConfigFile(globalConfigPath())
	for k, v := range parseConfigFile(filepath.Join(root, configDir, "config.yml")) {
		raw[k] = v
	}
	for k, v := range raw {
		switch k {
		case "agent":
			cfg.Agent = strings.Trim(v, `"'`)
		case "agent_cmd":
			cfg.AgentCmd = strings.Trim(v, `"'`)
		case "agent_timeout":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.AgentTimeout = n
			}
		case "editor":
			cfg.Editor = strings.Trim(v, `"'`)
		case "web_host":
			cfg.WebHost = strings.Trim(v, `"'`)
		case "web_port":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.WebPort = n
			}
		case "statuses", "verify":
			if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
				var list []string
				for _, item := range strings.Split(v[1:len(v)-1], ",") {
					if item = strings.TrimSpace(item); item != "" {
						list = append(list, item)
					}
				}
				if len(list) > 0 {
					if k == "statuses" {
						cfg.Statuses = list
					} else {
						cfg.Verify = list
					}
				}
			}
		}
	}
	return cfg
}

func (c Config) String() string {
	return fmt.Sprintf("agent: %s\nagent_cmd: %s\nagent_timeout: %d\nweb_host: %s\nweb_port: %d\neditor: %s\nstatuses: [%s]\nverify: [%s]",
		c.Agent, c.AgentCmd, c.AgentTimeout, c.WebHost, c.WebPort, c.editor(),
		strings.Join(c.Statuses, ", "), strings.Join(c.Verify, ", "))
}

const globalConfigTemplate = `# Global defaults for all spec-managed repositories.
# Repo-level .spec/config.yml overrides these per-key.

# Agent (opt-in; none = no subprocess ever runs)
# agent: claude        # claude | devin | gemini | codex | aider | opencode | amp | cline | vibe
# agent_cmd: mycli --flag {prompt}   # {prompt} = prompt arg; absent = stdin
# agent_timeout: 900   # seconds

# Web
# web_host: 127.0.0.1  # use 0.0.0.0 to expose on all interfaces
# web_port: 8080

# Lifecycle
# statuses: [draft, proposed, accepted, implemented, deprecated, rejected]

# Verification — commands run by spec verify (no shell; plain argv)
# verify: [go test ./..., spec validate, spec check]

# Editor for spec edit / spec config edit (falls back to $EDITOR, then vi)
# editor: vim
`

const repoConfigTemplate = `version: 1

# Repo-level overrides. Global defaults live in ~/.config/spec/config.yml —
# any key here wins over the same key there.

# Agent (opt-in; none = no subprocess ever runs)
# agent: claude        # claude | devin | gemini | codex | aider | opencode | amp | cline | vibe
# agent_cmd: mycli --flag {prompt}   # {prompt} = prompt arg; absent = stdin
# agent_timeout: 900   # seconds

# Web
# web_host: 127.0.0.1   # use 0.0.0.0 to expose on all interfaces
# web_port: 8080

# Lifecycle
# statuses: [draft, proposed, accepted, implemented, deprecated, rejected]

# Verification — commands run by spec verify (no shell; plain argv)
# verify: [go test ./..., spec validate, spec check]

# Editor for spec edit / spec config edit (falls back to $EDITOR, then vi)
# editor: vim
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
