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
	Agent        string // auto | none | adapter name
	AgentCmd     string // custom command; %s is replaced by the prompt, else prompt goes to stdin
	AgentTimeout int    // seconds
}

func defaultConfig() Config {
	return Config{Agent: "auto", AgentTimeout: 300}
}

func loadConfig(root string) Config {
	cfg := defaultConfig()
	data, err := os.ReadFile(filepath.Join(root, configDir, "config.yml"))
	if err != nil {
		return cfg
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
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		switch k {
		case "agent":
			cfg.Agent = v
		case "agent_cmd":
			cfg.AgentCmd = v
		case "agent_timeout":
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cfg.AgentTimeout = n
			}
		}
	}
	return cfg
}

func (c Config) String() string {
	return fmt.Sprintf("agent: %s\nagent_cmd: %s\nagent_timeout: %d",
		c.Agent, c.AgentCmd, c.AgentTimeout)
}
