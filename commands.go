package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func mustStore() Store {
	root, err := findRoot(".")
	if err != nil {
		fatal(err)
	}
	return Store{Root: root}
}

func cmdInit() error {
	for _, dir := range []string{
		configDir,
		filepath.Join(specDir, "project"),
		filepath.Join(specDir, "features"),
		filepath.Join(specDir, "adr"),
		filepath.Join(specDir, "imported"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	config := filepath.Join(configDir, "config.yml")
	if _, err := os.Stat(config); os.IsNotExist(err) {
		if err := os.WriteFile(config, []byte("version: 1\n# agent: claude\n# agent_cmd: mycli --flag {prompt}\n# agent_timeout: 300\n"), 0o644); err != nil {
			return err
		}
	}
	fmt.Println("Initialized spec repository.")
	return nil
}

func cmdNew(specType, title string) error {
	if !validTypes[specType] {
		return fmt.Errorf("unknown spec type %q (want project|feature|adr)", specType)
	}
	st := mustStore()
	s, err := st.Create(specType, title, templateFor(specType, title), nil)
	if err != nil {
		return err
	}
	fmt.Println("Created", s.Path)
	return nil
}

func cmdBootstrap() error {
	st := mustStore()
	var features, docs []string
	filepath.Walk(st.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && strings.HasPrefix(info.Name(), ".") && path != st.Root {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(st.Root, path)
		if strings.HasPrefix(rel, specDir+string(filepath.Separator)) {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(rel), ".md") {
			docs = append(docs, rel)
		}
		switch info.Name() {
		case "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "pom.xml":
			features = append(features, rel)
		}
		return nil
	})

	title := filepath.Base(st.Root)
	cfg := loadConfig(st.Root)
	ag, aerr := resolveAgent(cfg)
	if aerr != nil {
		fmt.Fprintln(os.Stderr, "warning:", aerr, "— falling back to heuristic bootstrap")
	}
	if ag != nil {
		var ctx strings.Builder
		fmt.Fprintf(&ctx, "Manifest files: %s\nDocumentation files: %s\n",
			strings.Join(features, ", "), strings.Join(docs, ", "))
		ctx.WriteString(repoContext(st, 50))
		prompt := fmt.Sprintf(bootstrapPromptFmt, title,
			fmt.Sprintf(projectTemplate, title), ctx.String())
		out, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err, "— falling back to heuristic bootstrap")
		} else {
			s, err := st.Create("project", title, bodyFromAgentOutput(out), nil)
			if err != nil {
				return err
			}
			fmt.Println("Created", s.Path, "via", ag.name)
			return nil
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Overview\n\nBootstrapped from existing repository.\n\n", title)
	b.WriteString("## Goals\n\n- TODO\n\n## Non-Goals\n\n- TODO\n\n## Requirements\n\n- REQ-001: TODO\n\n## Architecture\n\n")
	if len(features) > 0 {
		b.WriteString("Detected project files:\n\n")
		for _, f := range features {
			fmt.Fprintf(&b, "- `%s`\n", f)
		}
	}
	if len(docs) > 0 {
		b.WriteString("\nExisting documentation:\n\n")
		for _, d := range docs {
			fmt.Fprintf(&b, "- `%s`\n", d)
		}
	}
	s, err := st.Create("project", title, b.String(), nil)
	if err != nil {
		return err
	}
	fmt.Println("Created", s.Path)
	return nil
}

func cmdRefine(id string) error {
	st := mustStore()
	s := st.Get(id)
	if s == nil {
		return fmt.Errorf("no spec with id %s", id)
	}
	cfg := loadConfig(st.Root)
	ag, err := resolveAgent(cfg)
	if err != nil {
		return err
	}
	if ag == nil {
		return fmt.Errorf("refine requires an LLM agent (none configured; set agent/agent_cmd in .spec/config.yml)")
	}
	prompt := fmt.Sprintf(refinePromptFmt, s.Type(), s.ID(), s.Title(), s.Body, repoContext(st, 50))
	out, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
	if err != nil {
		return err
	}
	body := bodyFromAgentOutput(out)
	if err := os.WriteFile(s.Path, []byte(dumpFrontmatter(s.Meta, body)), 0o644); err != nil {
		return err
	}
	fmt.Println("Refined", s.Path, "via", ag.name)
	return nil
}

func cmdConfig() error {
	st := mustStore()
	cfg := loadConfig(st.Root)
	fmt.Println(cfg.String())
	ag, err := resolveAgent(cfg)
	if err != nil {
		fmt.Println("resolved agent: error:", err)
	} else if ag != nil {
		fmt.Println("resolved agent:", ag.name)
	} else {
		fmt.Println("resolved agent: none")
	}
	return nil
}

func cmdImport(file string) error {
	st := mustStore()
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	meta, body := parseFrontmatter(string(data))
	if t := metaGet(meta, "title", ""); t != "" {
		title = t
	} else {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "# ") {
				title = strings.TrimSpace(line[2:])
				break
			}
		}
	}
	s, err := st.Create("imported", title, body,
		map[string][]string{"source": {file}})
	if err != nil {
		return err
	}
	fmt.Println("Imported", file, "->", s.Path)
	return nil
}

var langByExt = map[string]string{
	".go": "Go", ".py": "Python", ".js": "JavaScript", ".ts": "TypeScript",
	".rs": "Rust", ".java": "Java", ".c": "C", ".h": "C/C++", ".cpp": "C++",
	".rb": "Ruby", ".sh": "Shell", ".md": "Markdown", ".yml": "YAML", ".yaml": "YAML",
	".json": "JSON", ".toml": "TOML",
}

func cmdAnalyze() error {
	st := mustStore()
	langs := map[string]int{}
	files := 0
	filepath.Walk(st.Root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != st.Root {
				return filepath.SkipDir
			}
			return nil
		}
		files++
		if lang, ok := langByExt[filepath.Ext(path)]; ok {
			langs[lang]++
		}
		return nil
	})
	specs := st.Specs()
	fmt.Printf("Repository: %s\n", st.Root)
	fmt.Printf("Files: %d\n", files)
	fmt.Println("Languages:")
	keys := make([]string, 0, len(langs))
	for k := range langs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return langs[keys[i]] > langs[keys[j]] })
	for _, k := range keys {
		fmt.Printf("  %-12s %d\n", k, langs[k])
	}
	byType := map[string]int{}
	byStatus := map[string]int{}
	for _, s := range specs {
		byType[s.Type()]++
		byStatus[s.Status()]++
	}
	fmt.Printf("Specs: %d\n", len(specs))
	for k, v := range byType {
		fmt.Printf("  %-10s %d\n", k, v)
	}
	for k, v := range byStatus {
		fmt.Printf("  status %-12s %d\n", k, v)
	}
	return nil
}

func cmdSearch(query string) error {
	st := mustStore()
	q := strings.ToLower(query)
	found := false
	for _, s := range st.Specs() {
		data, _ := os.ReadFile(s.Path)
		if strings.Contains(strings.ToLower(string(data)), q) {
			rel, _ := filepath.Rel(st.Root, s.Path)
			fmt.Printf("%s  %s  %s\n", s.ID(), s.Status(), rel)
			found = true
		}
	}
	if !found {
		fmt.Println("No matches.")
	}
	return nil
}

var reqPattern = regexp.MustCompile(`REQ-\d+`)

func cmdValidate() error {
	st := mustStore()
	specs := st.Specs()
	errs := 0
	fail := func(format string, args ...any) {
		fmt.Printf("FAIL  "+format+"\n", args...)
		errs++
	}
	ids := map[string]bool{}
	for _, s := range specs {
		rel, _ := filepath.Rel(st.Root, s.Path)
		if s.ID() == "" {
			fail("%s: missing id", rel)
		} else if ids[s.ID()] {
			fail("%s: duplicate id %s", rel, s.ID())
		}
		ids[s.ID()] = true
		if s.Type() == "" {
			fail("%s: missing type", rel)
		}
		for _, sec := range requiredSections[s.Type()] {
			found := false
			for _, have := range s.Sections() {
				if strings.EqualFold(have, sec) {
					found = true
				}
			}
			if !found {
				fail("%s (%s): missing section %q", rel, s.ID(), sec)
			}
		}
	}
	for _, s := range specs {
		for _, dep := range s.Field("depends_on") {
			if st.Get(dep) == nil {
				fail("%s: depends_on references unknown spec %s", s.ID(), dep)
			}
		}
	}
	// Requirement traceability: every REQ defined in project specs should be
	// referenced by at least one feature, and feature references must exist.
	defined := map[string]bool{}
	for _, s := range specs {
		if s.Type() == "project" {
			for _, m := range reqPattern.FindAllString(s.Body, -1) {
				defined[m] = true
			}
		}
	}
	for _, s := range specs {
		if s.Type() != "feature" {
			continue
		}
		for _, m := range reqPattern.FindAllString(s.Body, -1) {
			if !defined[m] {
				fail("%s: references undefined requirement %s", s.ID(), m)
			}
		}
	}
	if errs == 0 {
		fmt.Printf("OK  %d specs valid\n", len(specs))
		return nil
	}
	return fmt.Errorf("%d validation error(s)", errs)
}

func cmdExport(out string) error {
	st := mustStore()
	type entry struct {
		ID     string   `json:"id"`
		Title  string   `json:"title"`
		Type   string   `json:"type"`
		Status string   `json:"status"`
		Path   string   `json:"path"`
		Deps   []string `json:"depends_on,omitempty"`
	}
	var list []entry
	for _, s := range st.Specs() {
		rel, _ := filepath.Rel(st.Root, s.Path)
		list = append(list, entry{s.ID(), s.Title(), s.Type(), s.Status(), rel, s.Field("depends_on")})
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if out == "" || out == "-" {
		fmt.Println(string(data))
		return nil
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		return err
	}
	fmt.Println("Exported", len(list), "specs ->", out)
	return nil
}

func cmdRoadmap(out string) error {
	st := mustStore()
	order := []string{"proposed", "accepted", "implemented", "draft", "deprecated", "rejected"}
	groups := map[string][]Spec{}
	for _, s := range st.Specs() {
		groups[s.Status()] = append(groups[s.Status()], s)
	}
	var b strings.Builder
	b.WriteString("# Roadmap\n\nGenerated by `spec roadmap`.\n")
	for _, status := range order {
		items := groups[status]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s%s\n\n", strings.ToUpper(status[:1]), status[1:])
		for _, s := range items {
			rel, _ := filepath.Rel(st.Root, s.Path)
			fmt.Fprintf(&b, "- **%s** — [%s](%s) (%s)\n", s.ID(), s.Title(), rel, s.Type())
		}
	}
	if out == "" || out == "-" {
		fmt.Print(b.String())
		return nil
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("Wrote", out)
	return nil
}

func cmdGraph(mermaid bool) error {
	st := mustStore()
	specs := st.Specs()
	edges := [][2]string{}
	for _, s := range specs {
		for _, dep := range s.Field("depends_on") {
			edges = append(edges, [2]string{s.ID(), dep})
		}
	}
	if mermaid {
		fmt.Println("graph TD")
		for _, s := range specs {
			fmt.Printf("  %s[\"%s: %s\"]\n", strings.ReplaceAll(s.ID(), "-", "_"), s.ID(), s.Title())
		}
		for _, e := range edges {
			fmt.Printf("  %s --> %s\n", strings.ReplaceAll(e[0], "-", "_"), strings.ReplaceAll(e[1], "-", "_"))
		}
		return nil
	}
	if len(edges) == 0 {
		fmt.Println("No dependencies declared (use `depends_on` frontmatter).")
		return nil
	}
	for _, e := range edges {
		fmt.Printf("%s --> %s\n", e[0], e[1])
	}
	return nil
}

func cmdTrace() error {
	st := mustStore()
	specs := st.Specs()
	reqs := map[string]string{} // req id -> defining spec
	var order []string
	for _, s := range specs {
		if s.Type() != "project" {
			continue
		}
		for _, m := range reqPattern.FindAllString(s.Body, -1) {
			if _, ok := reqs[m]; !ok {
				reqs[m] = s.ID()
				order = append(order, m)
			}
		}
	}
	if len(order) == 0 {
		fmt.Println("No requirements found in project specs.")
		return nil
	}
	covered := 0
	fmt.Printf("%-9s %-10s %s\n", "REQ", "DEFINED-IN", "IMPLEMENTED-BY")
	for _, req := range order {
		var impls []string
		for _, s := range specs {
			if s.Type() != "feature" {
				continue
			}
			for _, m := range reqPattern.FindAllString(s.Body, -1) {
				if m == req {
					impls = append(impls, s.ID())
				}
			}
		}
		if len(impls) > 0 {
			covered++
		}
		label := strings.Join(impls, ", ")
		if label == "" {
			label = "—"
		}
		fmt.Printf("%-9s %-10s %s\n", req, reqs[req], label)
	}
	fmt.Printf("\nCoverage: %d/%d requirements traced to features\n", covered, len(order))
	return nil
}

func cmdDraftFeature(title string) error {
	st := mustStore()
	cfg := loadConfig(st.Root)
	ag, aerr := resolveAgent(cfg)
	if aerr != nil {
		fmt.Fprintln(os.Stderr, "warning:", aerr, "— falling back to heuristic draft")
	}
	if ag != nil {
		prompt := fmt.Sprintf(draftPromptFmt, title,
			fmt.Sprintf(featureTemplate, title), repoContext(st, 50))
		out, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err, "— falling back to heuristic draft")
		} else {
			s, err := st.Create("feature", title, bodyFromAgentOutput(out), nil)
			if err != nil {
				return err
			}
			fmt.Println("Drafted", s.Path, "via", ag.name)
			return nil
		}
	}
	// Offline fallback: infer relevant requirements by matching
	// title keywords against requirement text in project specs.
	words := map[string]bool{}
	for _, w := range regexp.MustCompile(`[a-z0-9]+`).FindAllString(strings.ToLower(title), -1) {
		words[w] = true
	}
	var reqs []string
	for _, s := range st.Specs() {
		if s.Type() != "project" {
			continue
		}
		for _, line := range strings.Split(s.Body, "\n") {
			m := reqPattern.FindString(line)
			if m == "" {
				continue
			}
			for w := range words {
				if len(w) > 3 && strings.Contains(strings.ToLower(line), w) {
					reqs = append(reqs, m)
					break
				}
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Summary\n\nTODO: generated draft — expand this summary.\n\n", title)
	b.WriteString("## Motivation\n\nTODO\n\n## Requirements\n\nAddresses:")
	if len(reqs) == 0 {
		b.WriteString(" TODO (no matching requirements detected)")
	} else {
		b.WriteString(" " + strings.Join(reqs, ", "))
	}
	b.WriteString("\n\n## Design\n\nTODO\n\n## Acceptance Criteria\n\n- [ ] TODO\n")
	s, err := st.Create("feature", title, b.String(), nil)
	if err != nil {
		return err
	}
	fmt.Println("Drafted", s.Path)
	if len(reqs) > 0 {
		fmt.Println("Linked requirements:", strings.Join(reqs, ", "))
	}
	return nil
}
