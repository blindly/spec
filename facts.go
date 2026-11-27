package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Fact struct {
	Key   string
	Value string
}

func factsPath(root string) string {
	return filepath.Join(root, configDir, "facts.yml")
}

const secretsFile = ".spec/secrets.yml"

func secretsPath(root string) string {
	return filepath.Join(root, secretsFile)
}

func loadSecretFacts(root string) []Fact {
	return readFacts(secretsPath(root))
}

// loadFacts returns only public facts that are not shadowed by a secret.
func loadFacts(root string) []Fact {
	return publicFacts(readFacts(factsPath(root)), factsMap(loadSecretFacts(root)))
}

func publicFacts(facts []Fact, secrets map[string]string) []Fact {
	var public []Fact
	for _, f := range facts {
		if _, secret := secrets[f.Key]; !secret {
			public = append(public, f)
		}
	}
	return public
}

var quotedFactValue = regexp.MustCompile(`^("(?:[^"\\]|\\.)*")(?:[ \t]+#.*)?$`)

func readFacts(path string) []Fact {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var facts []Fact
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" {
			facts = append(facts, Fact{Key: k, Value: parseFactValue(v)})
		}
	}
	return facts
}

func parseFactValue(value string) string {
	if quoted := quotedFactValue.FindStringSubmatch(value); quoted != nil {
		if decoded, err := strconv.Unquote(quoted[1]); err == nil {
			return decoded
		}
	}
	if i := strings.Index(value, " #"); i >= 0 {
		value = strings.TrimSpace(value[:i])
	}
	return strings.Trim(value, `"'`)
}

func factsMap(facts []Fact) map[string]string {
	m := make(map[string]string, len(facts))
	for _, f := range facts {
		m[f.Key] = f.Value
	}
	return m
}

func setFact(root, key, value string) error {
	return setFactIn(factsPath(root), 0o644, key, value)
}

// setFactIn adds or updates a key, preserving unrelated lines and comments.
func setFactIn(path string, perm os.FileMode, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if perm == 0o600 {
		value = encodeSecret(value)
	}
	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		k, _, found := strings.Cut(trimmed, ":")
		if found && strings.TrimSpace(k) == key {
			lines[i] = fmt.Sprintf("%s: %s", key, value)
			replaced = true
		}
	}
	if !replaced {
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines[n-1] = fmt.Sprintf("%s: %s", key, value)
			lines = append(lines, "")
		} else {
			lines = append(lines, fmt.Sprintf("%s: %s", key, value), "")
		}
	}
	return writeFileAtomic(path, []byte(strings.Join(lines, "\n")), perm)
}

// unsetFactIn removes a key line and reports whether anything changed.
func unsetFactIn(path string, perm os.FileMode, key string) (bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(data), "\n")
	var out []string
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if k, _, found := strings.Cut(trimmed, ":"); found && strings.TrimSpace(k) == key {
				removed = true
				continue
			}
		}
		out = append(out, line)
	}
	if !removed {
		return false, nil
	}
	return true, writeFileAtomic(path, []byte(strings.Join(out, "\n")), perm)
}

var factRefPattern = regexp.MustCompile(`\{fact:([a-zA-Z0-9_.-]+)\}`)

// substituteFacts resolves {fact:key} placeholders using .spec/facts.yml.
// Unknown keys are left untouched.
func substituteFacts(text, root string) string {
	facts := factsMap(loadFacts(root))
	if len(facts) == 0 {
		return text
	}
	return factRefPattern.ReplaceAllStringFunc(text, func(m string) string {
		key := factRefPattern.FindStringSubmatch(m)[1]
		if v, ok := facts[key]; ok {
			return v
		}
		return m
	})
}

func encodeSecret(value string) string {
	if strings.TrimSpace(value) != value || strings.Trim(value, `"'`) != value ||
		strings.Contains(value, " #") || strings.ContainsAny(value, "\r\n\x00") {
		return strconv.Quote(value)
	}
	return value
}

var secretRefPattern = regexp.MustCompile(`\{secret:([a-zA-Z0-9_.-]+)\}`)
var factKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func cmdFact(out, errOut io.Writer, args []string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		secrets := loadSecretFacts(st.Root)
		facts := publicFacts(readFacts(factsPath(st.Root)), factsMap(secrets))
		if len(facts) == 0 && len(secrets) == 0 {
			fmt.Fprintln(out, "No facts defined (.spec/facts.yml or .spec/secrets.yml).")
			return nil
		}
		for _, f := range facts {
			fmt.Fprintf(out, "%-20s %s\n", f.Key, f.Value)
		}
		seen := map[string]bool{}
		for _, f := range secrets {
			if !seen[f.Key] {
				fmt.Fprintf(out, "%-20s <secret>\n", f.Key)
				seen[f.Key] = true
			}
		}
		return nil
	}
	switch args[0] {
	case "discover":
		if len(args) > 2 || (len(args) == 2 && args[1] != "--yes") {
			return fmt.Errorf("usage: spec fact discover [--yes]")
		}
		return discoverFacts(st, os.Stdin, out, errOut, len(args) == 2)
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: spec fact get <key>")
		}
		v, ok := factsMap(loadSecretFacts(st.Root))[args[1]]
		if !ok {
			v, ok = factsMap(readFacts(factsPath(st.Root)))[args[1]]
		}
		if !ok {
			return fmt.Errorf("fact %q not defined", args[1])
		}
		fmt.Fprintln(out, v)
	case "set":
		args = args[1:]
		forceSecret := len(args) > 0 && args[0] == "--secret"
		if forceSecret {
			args = args[1:]
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: spec fact set [--secret] <key> <value>")
		}
		key, value := args[0], strings.Join(args[1:], " ")
		if !factKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid fact key %q", key)
		}
		_, alreadySecret := factsMap(loadSecretFacts(st.Root))[key]
		if forceSecret || alreadySecret {
			if err := setFactIn(secretsPath(st.Root), 0o600, key, value); err != nil {
				return err
			}
			if _, err := unsetFactIn(factsPath(st.Root), 0o644, key); err != nil {
				return err
			}
			value = "<secret>"
		} else if err := setFact(st.Root, key, value); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s = %s\n", key, value)
	case "unset":
		if len(args) < 2 {
			return fmt.Errorf("usage: spec fact unset <key>")
		}
		// Remove the public entry first so a shadowed value cannot become visible.
		removed, err := unsetFactIn(factsPath(st.Root), 0o644, args[1])
		if err != nil {
			return err
		}
		removedSecret, err := unsetFactIn(secretsPath(st.Root), 0o600, args[1])
		if err != nil {
			return err
		}
		if !removed && !removedSecret {
			return fmt.Errorf("fact %q not defined", args[1])
		}
		fmt.Fprintf(out, "Removed %s\n", args[1])
	default:
		return fmt.Errorf("usage: spec fact [list|get <key>|set [--secret] <key> <value>|unset <key>|discover [--yes]]")
	}
	return nil
}

// cmdCheck audits fact references, possible secret leaks, reminders, and drift.
func cmdCheck(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	problems := checkFacts(st, out)
	// reminders (FEAT-023): warn on due entries and misconfig, never fail
	now := time.Now()
	for _, r := range loadReminders(st.Root) {
		if !validEvery(r.Every) {
			fmt.Fprintf(out, "WARN  reminder %q has an invalid cadence %q\n", r.ID, r.Every)
		}
		if r.Spec != "" && st.Get(r.Spec) == nil {
			fmt.Fprintf(out, "WARN  reminder %q references unknown spec %s\n", r.ID, r.Spec)
		}
		if r.Due(now) {
			fmt.Fprintf(out, "WARN  reminder %q is due (%s)\n", r.ID, r.Title)
		}
	}
	// skills (FEAT-031): a stale or missing install means the harness
	// reads outdated guidance — fail like `spec skill` does
	problems += checkSkillInstalls(st, out)
	// spec hooks (FEAT-036): missing/outdated/unmanaged all mean the
	// spec-coverage rule isn't enforced — warn, never fail
	if dir, ok := gitHooksDir(st.Root); ok {
		for _, h := range specHookScripts {
			if s := hookFileState(dir, h.name, h.script); s != "installed" {
				fmt.Fprintf(out, "WARN  %s hook %s — 'spec skill sync' installs/refreshes\n", h.name, s)
			}
		}
	}
	// drift (FEAT-012): warn, never fail
	if g := loadGit(st.Root); g != nil {
		stale, _ := g.systemDrift(st, loadConfig(st.Root))
		for _, d := range stale {
			fmt.Fprintf(out, "WARN  %s may be stale: %s changed since it was last updated (spec drift)\n", d.Spec.ID(), strings.Join(d.Files, ", "))
		}
	}
	if problems > 0 {
		return fmt.Errorf("%d check failure(s)", problems)
	}
	fmt.Fprintln(out, "OK  check passed")
	return nil
}
