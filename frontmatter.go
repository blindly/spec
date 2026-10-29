package main

import (
	"fmt"
	"sort"
	"strings"
)

// parseFrontmatter splits a document into YAML-subset frontmatter and body.
// Supports "key: value" scalars, "key: [a, b]" inline lists, and dash lists.
func parseFrontmatter(text string) (map[string][]string, string) {
	meta := map[string][]string{}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta, text
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return map[string][]string{}, text
	}
	var key string
	for _, line := range lines[1:end] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && key != "" {
			meta[key] = append(meta[key], strings.TrimSpace(trimmed[2:]))
			continue
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			for _, item := range strings.Split(v[1:len(v)-1], ",") {
				if item = strings.TrimSpace(item); item != "" {
					meta[key] = append(meta[key], item)
				}
			}
		} else if v != "" {
			meta[key] = []string{v}
		} else {
			meta[key] = nil
		}
	}
	return meta, strings.Join(lines[end+1:], "\n")
}

func metaGet(meta map[string][]string, key, fallback string) string {
	if v := meta[key]; len(v) > 0 {
		return v[0]
	}
	return fallback
}

func dumpFrontmatter(meta map[string][]string, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := meta[k]
		if len(v) == 1 {
			fmt.Fprintf(&b, "%s: %s\n", k, v[0])
		} else {
			fmt.Fprintf(&b, "%s: [%s]\n", k, strings.Join(v, ", "))
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimRight(body, "\n"))
	b.WriteString("\n")
	return b.String()
}
