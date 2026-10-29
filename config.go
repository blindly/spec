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
}

var defaultStatuses = []string{"draft", "proposed", "accepted", "implemented", "deprecated", "rejected"}

func defaultConfig() Config {
	return Config{Agent: "none", AgentTimeout: 300, WebPort: 8080, WebHost: "127.0.0.1", Statuses: defaultStatuses}
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
		case "web_host":
			cfg.WebHost = strings.Trim(v, `"'`)
		case "web_port":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.WebPort = n
			}
		case "statuses":
			if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
				var list []string
				for _, item := range strings.Split(v[1:len(v)-1], ",") {
					if item = strings.TrimSpace(item); item != "" {
						list = append(list, item)
					}
				}
				if len(list) > 0 {
					cfg.Statuses = list
				}
			}
		}
	}
	return cfg
}

func (c Config) String() string {
	return fmt.Sprintf("agent: %s\nagent_cmd: %s\nagent_timeout: %d\nweb_host: %s\nweb_port: %d\nstatuses: [%s]",
		c.Agent, c.AgentCmd, c.AgentTimeout, c.WebHost, c.WebPort, strings.Join(c.Statuses, ", "))
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
