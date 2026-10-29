package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Facts live in .spec/facts.yml — flat "key: value" lines, "#" comments.
// They hold definite project facts (domain, phone, legal name, …) that
// specs and agents may reference via {fact:key} placeholders.

type Fact struct {
	Key   string
	Value string
}

func factsPath(root string) string {
	return filepath.Join(root, configDir, "facts.yml")
}

// loadFacts parses .spec/facts.yml preserving declaration order.
func loadFacts(root string) []Fact {
	data, err := os.ReadFile(factsPath(root))
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
		v = strings.Trim(v, `"'`)
		if k == "" {
			continue
		}
		facts = append(facts, Fact{Key: k, Value: v})
	}
	return facts
}

func factsMap(facts []Fact) map[string]string {
	m := make(map[string]string, len(facts))
	for _, f := range facts {
		m[f.Key] = f.Value
	}
	return m
}

// setFact adds or updates a key, preserving existing lines and comments.
func setFact(root, key, value string) error {
	path := factsPath(root)
	data, _ := os.ReadFile(path)
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
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// unsetFact removes a key line; reports whether anything changed.
func unsetFact(root, key string) (bool, error) {
	path := factsPath(root)
	data, err := os.ReadFile(path)
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
	return true, os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
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

func cmdFact(out, errOut io.Writer, args []string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "list" {
		facts := loadFacts(st.Root)
		if len(facts) == 0 {
			fmt.Fprintln(out, "No facts defined (.spec/facts.yml).")
			return nil
		}
		for _, f := range facts {
			fmt.Fprintf(out, "%-20s %s\n", f.Key, f.Value)
		}
		return nil
	}
	switch args[0] {
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: spec fact get <key>")
		}
		v, ok := factsMap(loadFacts(st.Root))[args[1]]
		if !ok {
			return fmt.Errorf("fact %q not defined", args[1])
		}
		fmt.Fprintln(out, v)
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: spec fact set <key> <value>")
		}
		if err := setFact(st.Root, args[1], strings.Join(args[2:], " ")); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s = %s\n", args[1], strings.Join(args[2:], " "))
	case "unset":
		if len(args) < 2 {
			return fmt.Errorf("usage: spec fact unset <key>")
		}
		removed, err := unsetFact(st.Root, args[1])
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("fact %q not defined", args[1])
		}
		fmt.Fprintf(out, "Removed %s\n", args[1])
	default:
		return fmt.Errorf("usage: spec fact [list|get <key>|set <key> <value>|unset <key>]")
	}
	return nil
}

// cmdCheck audits facts usage: {fact:key} references in spec bodies must
// resolve; defined-but-never-referenced keys are reported as warnings.
func cmdCheck(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	facts := loadFacts(st.Root)
	defined := factsMap(facts)
	referenced := map[string]bool{}
	var undefined []string
	for _, s := range st.Specs() {
		for _, m := range factRefPattern.FindAllStringSubmatch(s.Body, -1) {
			key := m[1]
			if _, ok := defined[key]; !ok {
				undefined = append(undefined, fmt.Sprintf("%s: {fact:%s}", s.ID(), key))
			}
			referenced[key] = true
		}
	}
	problems := 0
	for _, u := range undefined {
		fmt.Fprintln(out, "FAIL  undefined fact reference", u)
		problems++
	}
	seen := map[string]bool{}
	for _, f := range facts {
		if f.Value == "" {
			fmt.Fprintf(out, "WARN  fact %q has an empty value\n", f.Key)
		}
		if seen[f.Key] {
			fmt.Fprintf(out, "WARN  fact %q is defined more than once\n", f.Key)
		}
		seen[f.Key] = true
		if !referenced[f.Key] {
			fmt.Fprintf(out, "WARN  fact %q is never referenced ({fact:%s})\n", f.Key, f.Key)
		}
	}
	if problems > 0 {
		return fmt.Errorf("%d undefined fact reference(s)", problems)
	}
	fmt.Fprintln(out, "OK  facts check passed")
	return nil
}
