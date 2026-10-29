package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func mustStore() (Store, error) {
	root, err := findRoot(".")
	if err != nil {
		return Store{}, err
	}
	return Store{Root: root}, nil
}

func cmdInit(out, errOut io.Writer) error {
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
		defaults := `version: 1

# Repo-level overrides. Global defaults live in ~/.config/spec/config.yml —
# any key here wins over the same key there.

# Agent (opt-in; none = no subprocess ever runs)
# agent: claude
# agent_cmd: mycli --flag {prompt}
# agent_timeout: 300

# Web
# web_host: 127.0.0.1   # use 0.0.0.0 to expose on all interfaces
# web_port: 8080

# Lifecycle
# statuses: [draft, proposed, accepted, implemented, deprecated, rejected]
`
		if err := os.WriteFile(config, []byte(defaults), 0o644); err != nil {
			return err
		}
	}
	facts := filepath.Join(configDir, "facts.yml")
	if _, err := os.Stat(facts); os.IsNotExist(err) {
		seed := `# Definite project facts — referenced in specs as {fact:key}
# domain: exemplo.pt
# phone: "+351 210 000 000"
# company_name: Empresa Lda
`
		if err := os.WriteFile(facts, []byte(seed), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "Initialized spec repository.")
	return nil
}

func cmdNew(out, errOut io.Writer, specType, title string) error {
	if !validTypes[specType] {
		return fmt.Errorf("unknown spec type %q (want project|feature|adr)", specType)
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	s, err := st.Create(specType, title, templateFor(specType, title), nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Created", s.Path)
	return nil
}

func cmdBootstrap(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
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
		fmt.Fprintln(errOut, "warning:", aerr, "— falling back to heuristic bootstrap")
	}
	if ag != nil {
		var ctx strings.Builder
		fmt.Fprintf(&ctx, "Manifest files: %s\nDocumentation files: %s\n",
			strings.Join(features, ", "), strings.Join(docs, ", "))
		ctx.WriteString(repoContext(st, 50))
		prompt := fmt.Sprintf(bootstrapPromptFmt, title,
			fmt.Sprintf(projectTemplate, title), ctx.String())
		output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if err != nil {
			fmt.Fprintln(errOut, "warning:", err, "— falling back to heuristic bootstrap")
		} else {
			s, err := st.Create("project", title, bodyFromAgentOutput(output), nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "Created", s.Path, "via", ag.name)
			return nil
		}
	}
	var b strings.Builder
	overview := "Bootstrapped from existing repository."
	if r := readmeExcerpt(st.Root, 2048); r != "" {
		// first non-heading paragraph of the README
		for _, para := range strings.Split(r, "\n\n") {
			para = strings.TrimSpace(para)
			if para != "" && !strings.HasPrefix(para, "#") {
				overview = para
				break
			}
		}
	}
	fmt.Fprintf(&b, "# %s\n\n## Overview\n\n%s\n\n", title, overview)
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
	fmt.Fprintln(out, "Created", s.Path)
	return nil
}

func cmdRefine(out, errOut io.Writer, id string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
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
	output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
	if err != nil {
		return err
	}
	body := bodyFromAgentOutput(output)
	if err := os.WriteFile(s.Path, []byte(dumpFrontmatter(s.Meta, body)), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Refined", s.Path, "via", ag.name)
	return nil
}

func cmdConfig(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	fmt.Fprintf(out, "global config: %s\n", globalConfigPath())
	fmt.Fprintln(out, cfg.String())
	ag, err := resolveAgent(cfg)
	if err != nil {
		fmt.Fprintln(out, "resolved agent: error:", err)
	} else if ag != nil {
		fmt.Fprintln(out, "resolved agent:", ag.name)
	} else {
		fmt.Fprintln(out, "resolved agent: none")
	}
	return nil
}

func cmdImport(out, errOut io.Writer, file string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	title := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	text := toUTF8(data)
	meta, body := parseFrontmatter(text)
	if t := metaGet(meta, "title", ""); t != "" {
		title = t
	} else {
		for _, line := range strings.Split(text, "\n") {
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
	fmt.Fprintln(out, "Imported", file, "->", s.Path)
	return nil
}

var langByExt = map[string]string{
	".go": "Go", ".py": "Python", ".js": "JavaScript", ".ts": "TypeScript",
	".rs": "Rust", ".java": "Java", ".c": "C", ".h": "C/C++", ".cpp": "C++",
	".rb": "Ruby", ".sh": "Shell", ".md": "Markdown", ".yml": "YAML", ".yaml": "YAML",
	".json": "JSON", ".toml": "TOML",
}

func cmdAnalyze(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
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
	fmt.Fprintf(out, "Repository: %s\n", st.Root)
	fmt.Fprintf(out, "Files: %d\n", files)
	fmt.Fprintln(out, "Languages:")
	keys := make([]string, 0, len(langs))
	for k := range langs {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return langs[keys[i]] > langs[keys[j]] })
	for _, k := range keys {
		fmt.Fprintf(out, "  %-12s %d\n", k, langs[k])
	}
	byType := map[string]int{}
	byStatus := map[string]int{}
	for _, s := range specs {
		byType[s.Type()]++
		byStatus[s.Status()]++
	}
	fmt.Fprintf(out, "Specs: %d\n", len(specs))
	for k, v := range byType {
		fmt.Fprintf(out, "  %-10s %d\n", k, v)
	}
	for k, v := range byStatus {
		fmt.Fprintf(out, "  status %-12s %d\n", k, v)
	}
	return nil
}

func cmdSearch(out, errOut io.Writer, query string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	q := strings.ToLower(query)
	found := false
	for _, s := range st.Specs() {
		data, _ := os.ReadFile(s.Path)
		rel, _ := filepath.Rel(st.Root, s.Path)
		header := false
		for i, line := range strings.Split(toUTF8(data), "\n") {
			if !strings.Contains(strings.ToLower(line), q) {
				continue
			}
			if !header {
				fmt.Fprintf(out, "%s  %s  %s\n", s.ID(), s.Status(), rel)
				header = true
				found = true
			}
			fmt.Fprintf(out, "    %d: %s\n", i+1, strings.TrimSpace(line))
		}
	}
	if !found {
		fmt.Fprintln(out, "No matches.")
	}
	return nil
}

var reqPattern = regexp.MustCompile(`REQ-\d+`)

func cmdStatus(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	specs := st.Specs()

	byType := map[string]int{}
	byStatus := map[string]int{}
	for _, s := range specs {
		byType[s.Type()]++
		byStatus[s.Status()]++
	}

	fmt.Fprintf(out, "Repository: %s\n\n", st.Root)
	fmt.Fprintf(out, "Specs: %d\n", len(specs))
	for _, t := range []string{"project", "feature", "adr", "imported"} {
		if byType[t] > 0 {
			fmt.Fprintf(out, "  %-10s %d\n", t, byType[t])
		}
	}
	fmt.Fprintln(out, "By status:")
	for _, status := range cfg.Statuses {
		if byStatus[status] > 0 {
			fmt.Fprintf(out, "  %-12s %d\n", status, byStatus[status])
		}
	}
	for status, n := range byStatus {
		if !cfg.validStatus(status) {
			fmt.Fprintf(out, "  %-12s %d (not in config)\n", status, n)
		}
	}

	// Requirement coverage
	defined := map[string]bool{}
	var order []string
	for _, s := range specs {
		if s.Type() != "project" {
			continue
		}
		for _, m := range reqPattern.FindAllString(s.Body, -1) {
			if !defined[m] {
				defined[m] = true
				order = append(order, m)
			}
		}
	}
	if len(order) > 0 {
		covered := 0
		for _, req := range order {
			for _, s := range specs {
				if s.Type() != "feature" {
					continue
				}
				if strings.Contains(strings.Join(reqPattern.FindAllString(s.Body, -1), " "), req) {
					covered++
					break
				}
			}
		}
		fmt.Fprintf(out, "\nRequirements: %d/%d traced to features\n", covered, len(order))
	}

	deps := 0
	for _, s := range specs {
		deps += len(s.Field("depends_on"))
	}
	if deps > 0 {
		fmt.Fprintf(out, "Dependencies: %d declared\n", deps)
	}
	if c := findCycle(specs); c != nil {
		fmt.Fprintf(out, "Warning: dependency cycle: %s\n", strings.Join(c, " -> "))
	}

	ag, aerr := resolveAgent(cfg)
	switch {
	case aerr != nil:
		fmt.Fprintf(out, "Agent: error: %s\n", aerr)
	case ag != nil:
		fmt.Fprintf(out, "Agent: %s\n", ag.name)
	default:
		fmt.Fprintln(out, "Agent: none configured")
	}
	return nil
}

func cmdValidate(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	specs := st.Specs()
	errs := 0
	fail := func(format string, args ...any) {
		fmt.Fprintf(out, "FAIL  "+format+"\n", args...)
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
		if !cfg.validStatus(s.Status()) {
			fail("%s (%s): unknown status %q", rel, s.ID(), s.Status())
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
	if c := findCycle(specs); c != nil {
		fail("dependency cycle: %s", strings.Join(c, " -> "))
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
		fmt.Fprintf(out, "OK  %d specs valid\n", len(specs))
		return nil
	}
	return fmt.Errorf("%d validation error(s)", errs)
}

func cmdExport(out, errOut io.Writer, file string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
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
	if file == "" || file == "-" {
		fmt.Fprintln(out, string(data))
		return nil
	}
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Exported", len(list), "specs ->", file)
	return nil
}

func cmdRoadmap(out, errOut io.Writer, file string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	groups := map[string][]Spec{}
	for _, s := range st.Specs() {
		groups[s.Status()] = append(groups[s.Status()], s)
	}
	// configured statuses first, then any others found in the wild
	order := append([]string{}, cfg.Statuses...)
	for s := range groups {
		if !cfg.validStatus(s) {
			order = append(order, s)
		}
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
	if file == "" || file == "-" {
		fmt.Fprint(out, b.String())
		return nil
	}
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Wrote", file)
	return nil
}

// findCycle returns one dependency cycle (as an id path) or nil if the
// depends_on graph is acyclic.
func findCycle(specs []Spec) []string {
	deps := map[string][]string{}
	for _, s := range specs {
		deps[s.ID()] = s.Field("depends_on")
	}
	const (
		white, gray, black = 0, 1, 2
	)
	color := map[string]int{}
	var stack []string
	var cycle []string
	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = gray
		stack = append(stack, id)
		for _, dep := range deps[id] {
			if _, known := deps[dep]; !known {
				continue
			}
			switch color[dep] {
			case gray: // back edge — found a cycle
				for i, v := range stack {
					if v == dep {
						cycle = append([]string{}, stack[i:]...)
						cycle = append(cycle, dep)
						return true
					}
				}
			case white:
				if visit(dep) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}
	for _, s := range specs {
		if color[s.ID()] == white && visit(s.ID()) {
			return cycle
		}
	}
	return nil
}

func cmdGraph(out, errOut io.Writer, mermaid bool) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	specs := st.Specs()
	edges := [][2]string{}
	for _, s := range specs {
		for _, dep := range s.Field("depends_on") {
			edges = append(edges, [2]string{s.ID(), dep})
		}
	}
	if c := findCycle(specs); c != nil {
		fmt.Fprintln(errOut, "warning: dependency cycle:", strings.Join(c, " -> "))
	}
	if mermaid {
		fmt.Fprintln(out, "graph TD")
		for _, s := range specs {
			fmt.Fprintf(out, "  %s[\"%s: %s\"]\n", strings.ReplaceAll(s.ID(), "-", "_"), s.ID(), s.Title())
		}
		for _, e := range edges {
			fmt.Fprintf(out, "  %s --> %s\n", strings.ReplaceAll(e[0], "-", "_"), strings.ReplaceAll(e[1], "-", "_"))
		}
		return nil
	}
	if len(edges) == 0 {
		fmt.Fprintln(out, "No dependencies declared (use `depends_on` frontmatter).")
		return nil
	}
	for _, e := range edges {
		fmt.Fprintf(out, "%s --> %s\n", e[0], e[1])
	}
	return nil
}

func cmdTrace(out, errOut io.Writer) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
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
		fmt.Fprintln(out, "No requirements found in project specs.")
		return nil
	}
	covered := 0
	fmt.Fprintf(out, "%-9s %-10s %s\n", "REQ", "DEFINED-IN", "IMPLEMENTED-BY")
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
		fmt.Fprintf(out, "%-9s %-10s %s\n", req, reqs[req], label)
	}
	fmt.Fprintf(out, "\nCoverage: %d/%d requirements traced to features\n", covered, len(order))
	return nil
}

// applyableStatus reports whether a spec is eligible for `spec apply`
// without --force.
func applyableStatus(s Spec) bool {
	return s.Type() == "feature" && (s.Status() == "accepted" || s.Status() == "proposed")
}

func cmdApply(out, errOut io.Writer, ids []string, runIt, force bool) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	var targets []Spec
	if len(ids) == 0 {
		for _, s := range st.Specs() {
			if applyableStatus(s) {
				targets = append(targets, s)
			}
		}
		if len(targets) == 0 {
			fmt.Fprintln(out, "No accepted/proposed feature specs to apply.")
			return nil
		}
	} else {
		for _, id := range ids {
			s := st.Get(id)
			if s == nil {
				return fmt.Errorf("no spec with id %s", id)
			}
			if !applyableStatus(*s) && !force {
				return fmt.Errorf("%s is %s (type %s) — apply targets accepted/proposed features (use --force to override)", s.ID(), s.Status(), s.Type())
			}
			targets = append(targets, *s)
		}
	}

	var ag *agent
	cfg := loadConfig(st.Root)
	if runIt {
		var aerr error
		ag, aerr = resolveAgent(cfg)
		if aerr != nil {
			return aerr
		}
		if ag == nil {
			return fmt.Errorf("--run requires an LLM agent (none configured; set agent/agent_cmd in .spec/config.yml)")
		}
	}

	for _, s := range targets {
		prompt := fmt.Sprintf(applyPromptFmt, s.ID(), s.Title(), s.Body, repoContext(st, 50))
		if !runIt {
			fmt.Fprintf(out, "# ---- apply prompt for %s ----\n\n%s\n\n", s.ID(), prompt)
			continue
		}
		fmt.Fprintf(out, "Applying %s via %s…\n", s.ID(), ag.name)
		output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s\n%s\n", s.ID(), strings.TrimSpace(output))
	}
	if !runIt {
		fmt.Fprintln(out, "# Paste the prompt(s) above into your agent CLI. To run the configured agent directly: spec apply --run")
	}
	return nil
}

func cmdDraftFeature(out, errOut io.Writer, title string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	ag, aerr := resolveAgent(cfg)
	if aerr != nil {
		fmt.Fprintln(errOut, "warning:", aerr, "— falling back to heuristic draft")
	}
	if ag != nil {
		prompt := fmt.Sprintf(draftPromptFmt, title,
			fmt.Sprintf(featureTemplate, title), repoContext(st, 50))
		output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if err != nil {
			fmt.Fprintln(errOut, "warning:", err, "— falling back to heuristic draft")
		} else {
			s, err := st.Create("feature", title, bodyFromAgentOutput(output), nil)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, "Drafted", s.Path, "via", ag.name)
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
	fmt.Fprintln(out, "Drafted", s.Path)
	if len(reqs) > 0 {
		fmt.Fprintln(out, "Linked requirements:", strings.Join(reqs, ", "))
	}
	return nil
}
