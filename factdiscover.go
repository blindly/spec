package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type factCandidate struct{ Key, Value, Source string }

// appendDiscoveredFacts preserves the original bytes and commits additions together.
func appendDiscoveredFacts(path string, perm os.FileMode, additions []Fact) error {
	if len(additions) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteByte('\n')
	}
	for _, f := range additions {
		value := f.Value
		if perm == 0o600 {
			value = encodeSecret(value)
		}
		fmt.Fprintf(&b, "%s: %s\n", f.Key, value)
	}
	return writeFileAtomic(path, []byte(b.String()), perm)
}

func discoverFacts(st Store, in io.Reader, out, errOut io.Writer, yes bool) error {
	defined := factsMap(readFacts(factsPath(st.Root)))
	secrets := factsMap(loadSecretFacts(st.Root))
	for key, value := range secrets {
		defined[key] = value
	}
	var seeds []Fact
	for _, s := range st.Specs() {
		for _, m := range factRefPattern.FindAllStringSubmatch(s.Body, -1) {
			if _, ok := defined[m[1]]; !ok {
				seeds = append(seeds, Fact{Key: m[1]})
				defined[m[1]] = ""
			}
		}
	}
	if err := appendDiscoveredFacts(factsPath(st.Root), 0o644, seeds); err != nil {
		return err
	}
	fmt.Fprintf(out, "Seeded %d keys.\n", len(seeds))
	cfg := loadConfig(st.Root)
	ag, err := resolveAgent(cfg)
	var candidates []factCandidate
	fallback := ag == nil || err != nil
	if err != nil {
		fmt.Fprintln(errOut, "warning:", err, "— falling back to heuristic fact discovery")
	}
	if ag != nil && err == nil {
		prompt := "Scan the repository for definite project values: domains, phone numbers, emails, project names and other stable values. Read files only; do not modify files or read .spec/secrets.yml. Never invent values. Return only lines in this exact format: key: value — source (file path). Keys must match [a-zA-Z0-9_.-]+. Omit existing fact keys. Return no lines if none are found.\n" + repoContext(st, 50)
		output, runErr := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if runErr != nil {
			fmt.Fprintln(errOut, "warning:", runErr, "— falling back to heuristic fact discovery")
			fallback = true
		} else {
			candidates = parseFactCandidates(output)
			if strings.TrimSpace(output) != "" && len(candidates) == 0 {
				fmt.Fprintln(errOut, "warning: agent returned no valid fact candidates — falling back to heuristic fact discovery")
				fallback = true
			}
		}
	}
	if fallback {
		candidates = structuredFactCandidates(st.Root)
	}
	var pending, pendingSecrets []Fact
	seen := map[string]string{}
	for _, c := range candidates {
		_, secretKey := secrets[c.Key]
		secret := tokenPattern.MatchString(c.Value)
		for _, value := range secrets {
			if value != "" && c.Value == value {
				secret = true
				break
			}
		}
		display := c.Value
		if secret || secretKey {
			display = "<secret>"
		}
		if value, ok := defined[c.Key]; ok {
			if value != c.Value {
				fmt.Fprintf(out, "Conflict %s: %s — %s (already defined; skipped)\n", c.Key, display, c.Source)
			}
			continue
		}
		if value, ok := seen[c.Key]; ok {
			if value != c.Value {
				fmt.Fprintf(out, "Conflict %s: %s — %s (duplicate proposal; skipped)\n", c.Key, display, c.Source)
			}
			continue
		}
		seen[c.Key] = c.Value
		fmt.Fprintf(out, "%s: %s — %s\n", c.Key, display, c.Source)
		if secret {
			pendingSecrets = append(pendingSecrets, Fact{c.Key, c.Value})
		} else {
			pending = append(pending, Fact{c.Key, c.Value})
		}
	}
	written := 0
	proposed := len(pending) + len(pendingSecrets)
	if proposed > 0 {
		accept := yes
		if !yes {
			fmt.Fprint(out, "Write proposed facts? [y/N] ")
			answer, _ := bufio.NewReader(in).ReadString('\n')
			answer = strings.ToLower(strings.TrimSpace(answer))
			accept = answer == "y" || answer == "yes"
		}
		if accept {
			if err := appendDiscoveredFacts(secretsPath(st.Root), 0o600, pendingSecrets); err != nil {
				return err
			}
			if err := appendDiscoveredFacts(factsPath(st.Root), 0o644, pending); err != nil {
				return err
			}
			written = proposed
		}
	}
	fmt.Fprintf(out, "Proposed %d candidates; wrote %d values (%d keys seeded).\n", proposed, written, len(seeds))
	return nil
}

var candidateLinePattern = regexp.MustCompile(`^([a-zA-Z0-9_.-]+):\s*(.+?)\s+—\s+(.+)$`)

func parseFactCandidates(output string) []factCandidate {
	var result []factCandidate
	for _, line := range strings.Split(output, "\n") {
		m := candidateLinePattern.FindStringSubmatch(strings.TrimSpace(line))
		if m != nil {
			result = append(result, factCandidate{m[1], strings.TrimSpace(m[2]), strings.TrimSpace(m[3])})
		}
	}
	return result
}

func structuredFactCandidates(root string) []factCandidate {
	var result []factCandidate
	add := func(k, v, s string) {
		if v != "" && !strings.ContainsAny(v, "\r\n") {
			result = append(result, factCandidate{k, v, s})
		}
	}
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = root
	if data, err := cmd.Output(); err == nil {
		remote := strings.TrimSpace(string(data))
		host := ""
		if strings.Contains(remote, "://") {
			if u, e := url.Parse(remote); e == nil {
				host = u.Hostname()
			}
		} else if before, _, ok := strings.Cut(remote, ":"); ok && !strings.Contains(before, "/") {
			_, host, _ = strings.Cut(before, "@")
			if host == "" {
				host = before
			}
		}
		add("git_remote_host", host, "git remote origin")
	}
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "module" {
				add("go_module", strings.Trim(fields[1], `"`), "go.mod")
				break
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			Name     string `json:"name"`
			Homepage string `json:"homepage"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			add("package_name", pkg.Name, "package.json")
			add("homepage", pkg.Homepage, "package.json")
		}
	}
	if data, err := os.ReadFile(filepath.Join(root, "pyproject.toml")); err == nil {
		section := ""
		name := regexp.MustCompile(`^name\s*=\s*(?:"([^"\r\n]*)"|'([^'\r\n]*)')\s*(?:#.*)?$`)
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") {
				section = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
			}
			if section == "[project]" || section == "[tool.poetry]" {
				if m := name.FindStringSubmatch(line); m != nil {
					add("python_project_name", m[1]+m[2], "pyproject.toml")
					break
				}
			}
		}
	}
	return result
}
