package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	specDir   = "specs"
	configDir = ".spec"
)

var prefixes = map[string]string{
	"project": "PROJ", "feature": "FEAT", "adr": "ADR", "imported": "DOC",
}

var validTypes = map[string]bool{"project": true, "feature": true, "adr": true}

type Spec struct {
	Path string
	Meta map[string][]string
	Body string
}

func (s Spec) ID() string {
	return metaGet(s.Meta, "id", strings.TrimSuffix(filepath.Base(s.Path), ".md"))
}
func (s Spec) Title() string  { return metaGet(s.Meta, "title", s.ID()) }
func (s Spec) Type() string   { return metaGet(s.Meta, "type", "") }
func (s Spec) Status() string { return metaGet(s.Meta, "status", "draft") }
func (s Spec) Field(k string) []string {
	if s.Meta[k] == nil {
		return nil
	}
	return s.Meta[k]
}

func (s Spec) Sections() []string {
	var out []string
	for _, line := range strings.Split(s.Body, "\n") {
		if strings.HasPrefix(line, "## ") {
			out = append(out, strings.TrimSpace(line[3:]))
		}
	}
	return out
}

var nonAlphanum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	slug := strings.Trim(nonAlphanum.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if slug == "" {
		return "untitled"
	}
	return slug
}

// findRoot walks upward from dir looking for a .spec directory.
func findRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, configDir)); err == nil && st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a spec-managed repository (run `spec init`)")
		}
		dir = parent
	}
}

type Store struct {
	Root string
}

func (st Store) specPath() string { return filepath.Join(st.Root, specDir) }

func (st Store) Specs() []Spec {
	var out []Spec
	root := st.specPath()
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		meta, body := parseFrontmatter(string(data))
		out = append(out, Spec{Path: path, Meta: meta, Body: body})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

func (st Store) Get(id string) *Spec {
	for _, s := range st.Specs() {
		if s.ID() == id {
			return &s
		}
	}
	return nil
}

func (st Store) nextID(specType string) string {
	prefix := prefixes[specType]
	highest := 0
	re := regexp.MustCompile(`^` + prefix + `-(\d+)`)
	for _, s := range st.Specs() {
		if m := re.FindStringSubmatch(s.ID()); m != nil {
			var n int
			fmt.Sscanf(m[1], "%d", &n)
			if n > highest {
				highest = n
			}
		}
	}
	return fmt.Sprintf("%s-%03d", prefix, highest+1)
}

func (st Store) Create(specType, title, body string, extra map[string][]string) (Spec, error) {
	id := st.nextID(specType)
	sub := map[string]string{"feature": "features", "adr": "adr", "imported": "imported"}[specType]
	if sub == "" {
		sub = specType
	}
	dir := filepath.Join(st.specPath(), sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Spec{}, err
	}
	meta := map[string][]string{
		"id":      {id},
		"title":   {title},
		"type":    {specType},
		"status":  {"draft"},
		"created": {time.Now().Format("2006-01-02")},
	}
	for k, v := range extra {
		meta[k] = v
	}
	path := filepath.Join(dir, id+"-"+slugify(title)+".md")
	if err := os.WriteFile(path, []byte(dumpFrontmatter(meta, body)), 0o644); err != nil {
		return Spec{}, err
	}
	return Spec{Path: path, Meta: meta, Body: body}, nil
}
