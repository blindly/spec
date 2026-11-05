package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFrontmatterRoundTrip(t *testing.T) {
	meta := map[string][]string{
		"id":         {"FEAT-001"},
		"depends_on": {"PROJ-001", "FEAT-002"},
	}
	text := dumpFrontmatter(meta, "# Hello\n\nbody\n")
	got, body := parseFrontmatter(text)
	if got["id"][0] != "FEAT-001" || len(got["depends_on"]) != 2 || got["depends_on"][1] != "FEAT-002" {
		t.Fatalf("bad meta: %v", got)
	}
	if !strings.Contains(body, "# Hello") {
		t.Fatalf("bad body: %q", body)
	}
}

func TestFrontmatterRewriteIsStable(t *testing.T) {
	text := dumpFrontmatter(map[string][]string{"id": {"X-1"}}, "# Title\n\nbody\n")
	for i := 0; i < 3; i++ {
		meta, body := parseFrontmatter(text)
		if again := dumpFrontmatter(meta, body); again != text {
			t.Fatalf("rewrite %d changed the file:\n%q\nvs\n%q", i, text, again)
		}
	}
}

func TestFrontmatterNoHeader(t *testing.T) {
	meta, body := parseFrontmatter("# Just markdown\n")
	if len(meta) != 0 || body != "# Just markdown\n" {
		t.Fatalf("expected passthrough, got %v %q", meta, body)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.md")
	os.WriteFile(path, []byte("original"), 0o644)
	if err := writeFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "new" {
		t.Fatalf("got %q", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
	// failed write (unwritable dir) leaves the original intact
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a non-root POSIX user to make the dir unwritable")
	}
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	if err := writeFileAtomic(path, []byte("clobbered"), 0o644); err == nil {
		t.Fatal("expected error writing into read-only dir")
	}
	if data, _ := os.ReadFile(path); string(data) != "new" {
		t.Fatalf("original changed: %q", data)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func setupRepo(t *testing.T) Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".xdg"))
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	if err := cmdInit(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	return Store{Root: dir}
}

func TestInitAndNew(t *testing.T) {
	st := setupRepo(t)
	for _, p := range []string{
		filepath.Join(".devin", "skills", "spec", "SKILL.md"),
		filepath.Join(".claude", "skills", "spec", "SKILL.md"),
	} {
		data, err := os.ReadFile(filepath.Join(st.Root, p))
		if err != nil || !strings.Contains(string(data), "name: spec") {
			t.Fatalf("agent skill not seeded at %s: %v", p, err)
		}
	}
	for _, typ := range []string{"project", "feature", "adr"} {
		if err := cmdNew(io.Discard, io.Discard, typ, "My "+typ); err != nil {
			t.Fatal(err)
		}
	}
	specs := st.Specs()
	if len(specs) != 3 {
		t.Fatalf("want 3 specs, got %d", len(specs))
	}
	if specs[1].ID() != "FEAT-001" || specs[0].ID() != "ADR-001" {
		t.Fatalf("bad ids: %v", specs)
	}
	if err := cmdNew(io.Discard, io.Discard, "bogus", "x"); err == nil {
		t.Fatal("expected error for bad type")
	}
}

func TestValidateCatchesMissingSection(t *testing.T) {
	setupRepo(t)
	if err := cmdNew(io.Discard, io.Discard, "feature", "broken"); err != nil {
		t.Fatal(err)
	}
	st, _ := mustStore()
	s := st.Get("FEAT-001")
	if err := os.WriteFile(s.Path, []byte("---\nid: FEAT-001\ntype: feature\n---\n# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdValidate(io.Discard, io.Discard); err == nil {
		t.Fatal("expected validation failure")
	}
}

func TestValidateOK(t *testing.T) {
	setupRepo(t)
	cmdNew(io.Discard, io.Discard, "project", "p")
	cmdNew(io.Discard, io.Discard, "feature", "f")
	if err := cmdValidate(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestSearchAndTrace(t *testing.T) {
	st := setupRepo(t)
	body := "# p\n\n## Overview\n\nx\n\n## Requirements\n\n- REQ-001: login support\n"
	if _, err := st.Create("project", "p", body, nil); err != nil {
		t.Fatal(err)
	}
	if err := cmdSearch(io.Discard, io.Discard, "login support"); err != nil {
		t.Fatal(err)
	}
	if err := cmdDraftFeature(io.Discard, io.Discard, "add login page"); err != nil {
		t.Fatal(err)
	}
	feat := st.Get("FEAT-001")
	if feat == nil || !strings.Contains(feat.Body, "REQ-001") {
		t.Fatalf("draft should link REQ-001: %+v", feat)
	}
	if err := cmdTrace(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestImportAndExport(t *testing.T) {
	st := setupRepo(t)
	src := filepath.Join(st.Root, "notes.md")
	os.WriteFile(src, []byte("# Notes\n\nhello\n"), 0o644)
	if err := cmdImport(io.Discard, io.Discard, src); err != nil {
		t.Fatal(err)
	}
	if st.Get("DOC-001") == nil {
		t.Fatal("import failed")
	}
	out := filepath.Join(st.Root, "meta.json")
	if err := cmdExport(io.Discard, io.Discard, out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if !strings.Contains(string(data), "DOC-001") {
		t.Fatal("export missing spec")
	}
}

func TestImportPreservesFrontmatterKeys(t *testing.T) {
	st := setupRepo(t)
	src := filepath.Join(st.Root, "guide.md")
	os.WriteFile(src, []byte("---\ntitle: Style Guide\nstatus: proposed\ndepends_on: [PROJ-001]\nauthor: jane\n---\n# Guide\n\nbody\n"), 0o644)
	if err := cmdImport(io.Discard, io.Discard, src); err != nil {
		t.Fatal(err)
	}
	s := st.Get("DOC-001")
	if s == nil {
		t.Fatal("import failed")
	}
	if s.Status() != "proposed" || s.Field("author")[0] != "jane" || s.Field("depends_on")[0] != "PROJ-001" {
		t.Fatalf("source frontmatter keys lost: %+v", s.Meta)
	}
	if s.Field("source")[0] != src || s.Type() != "imported" || s.Title() != "Style Guide" {
		t.Fatalf("tool keys wrong: %+v", s.Meta)
	}
	// no status in source → defaults to accepted, not draft
	src2 := filepath.Join(st.Root, "plain.md")
	os.WriteFile(src2, []byte("# Plain doc\n\nx\n"), 0o644)
	if err := cmdImport(io.Discard, io.Discard, src2); err != nil {
		t.Fatal(err)
	}
	if s := st.Get("DOC-002"); s == nil || s.Status() != "accepted" {
		t.Fatalf("import without status should default to accepted: %+v", s)
	}
}

func TestBootstrapAndRoadmapAndGraph(t *testing.T) {
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, "go.mod"), []byte("module x\n"), 0o644)
	if err := cmdBootstrap(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if st.Get("PROJ-001") == nil {
		t.Fatal("bootstrap created no project spec")
	}
	if err := cmdRoadmap(io.Discard, io.Discard, filepath.Join(st.Root, "ROADMAP.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Root, "ROADMAP.md")); err != nil {
		t.Fatal(err)
	}
	if err := cmdGraph(io.Discard, io.Discard, false); err != nil {
		t.Fatal(err)
	}
	if err := cmdGraph(io.Discard, io.Discard, true); err != nil {
		t.Fatal(err)
	}
	if err := cmdAnalyze(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapSkipsGeneratedDirs(t *testing.T) {
	st := setupRepo(t)
	os.MkdirAll(filepath.Join(st.Root, "node_modules", "dep"), 0o755)
	os.WriteFile(filepath.Join(st.Root, "node_modules", "dep", "package.json"), []byte("{}"), 0o644)
	os.MkdirAll(filepath.Join(st.Root, "docs"), 0o755)
	os.WriteFile(filepath.Join(st.Root, "docs", "guide.md"), []byte("# Guide\n"), 0o644)
	os.WriteFile(filepath.Join(st.Root, "package.json"), []byte("{}"), 0o644)
	if err := cmdBootstrap(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	s := st.Get("PROJ-001")
	if s == nil {
		t.Fatal("bootstrap created no project spec")
	}
	if s.Status() != "implemented" {
		t.Fatalf("bootstrapped spec should be implemented, got %q", s.Status())
	}
	if strings.Contains(s.Body, "node_modules") {
		t.Fatalf("dependency dir leaked into spec:\n%s", s.Body)
	}
	if !strings.Contains(s.Body, "`package.json`") || !strings.Contains(s.Body, "docs/guide.md") {
		t.Fatalf("real files missing from spec:\n%s", s.Body)
	}
}

func TestAgentOversizedPromptFallsBackToStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "agent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\ncat > /dev/null\nprintf 'ok\\n'\n"), 0o755)
	ag := &agent{name: "fake", argv: []string{"sh", fake, "{prompt}"}}
	// bigger than one argv entry may legally be (128 KiB) → prompt must go
	// via stdin instead of failing with E2BIG
	out, err := ag.run(strings.Repeat("x", 200*1024), dir, 30)
	if err != nil || out != "ok\n" {
		t.Fatalf("oversized prompt: out=%q err=%v", out, err)
	}
	// a small prompt still travels in argv; stdin is left empty for the script
	out, err = ag.run("hello", dir, 30)
	if err != nil || out != "ok\n" {
		t.Fatalf("small prompt: out=%q err=%v", out, err)
	}
}

func TestList(t *testing.T) {
	st := setupRepo(t)
	if _, err := st.Create("feature", "Alpha", "# Alpha\n", nil); err != nil {
		t.Fatal(err)
	}
	arch, err := st.Create("adr", "Old decision", "# Old\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	arch.Meta["status"] = []string{"deprecated"}
	if _, err := st.Archive(arch); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := cmdList(&buf, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "FEAT-001") || !strings.Contains(out, "Alpha") ||
		!strings.Contains(out, "specs/features/FEAT-001-alpha.md") {
		t.Fatalf("listing missing spec or path:\n%s", out)
	}
	if strings.Contains(out, "ADR-001") {
		t.Fatalf("archived spec listed by default:\n%s", out)
	}
	buf.Reset()
	if err := cmdList(&buf, io.Discard, []string{"--all"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ADR-001") || !strings.Contains(buf.String(), "(archived)") {
		t.Fatalf("--all did not list archived spec:\n%s", buf.String())
	}
	buf.Reset()
	if err := cmdList(&buf, io.Discard, []string{"--paths"}); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	if got != "specs/features/FEAT-001-alpha.md" {
		t.Fatalf("--paths output: %q", got)
	}
}

func TestConfigParsing(t *testing.T) {
	st := setupRepo(t)
	cfg := loadConfig(st.Root)
	if cfg.Agent != "none" || cfg.AgentTimeout != 3600 {
		t.Fatalf("defaults: %+v", cfg)
	}
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent: none\nagent_timeout: 42\n"), 0o644)
	cfg = loadConfig(st.Root)
	if cfg.Agent != "none" || cfg.AgentTimeout != 42 {
		t.Fatalf("parsed: %+v", cfg)
	}
}

func TestBodyFromAgentOutput(t *testing.T) {
	withFM := "---\nid: X\n---\n\n# T\n\nbody\n"
	if got := bodyFromAgentOutput(withFM); !strings.Contains(got, "# T") || strings.Contains(got, "---") {
		t.Fatalf("frontmatter not stripped: %q", got)
	}
	fenced := "```markdown\n# T\n\nbody\n```\n"
	if got := bodyFromAgentOutput(fenced); strings.Contains(got, "```") {
		t.Fatalf("fence not stripped: %q", got)
	}
	prose := "I'll explore the repository first.\n\nLet me check.\n\n# Real Spec\n\nbody\n"
	if got := bodyFromAgentOutput(prose); strings.HasPrefix(got, "I'll") || !strings.HasPrefix(got, "# Real Spec") {
		t.Fatalf("preamble not stripped: %q", got)
	}
	glued := "I'll explore the repository first.# Real Spec\n\nbody\n"
	if got := bodyFromAgentOutput(glued); !strings.HasPrefix(got, "# Real Spec") {
		t.Fatalf("glued preamble not stripped: %q", got)
	}
}

func TestAgentDraftAndRefine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	st := setupRepo(t)
	fake := filepath.Join(st.Root, "fakeagent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\ncat > /dev/null\nprintf '# Agent Spec\\n\\n## Summary\\n\\nAI content.\\n\\n## Design\\n\\nD.\\n\\n## Acceptance Criteria\\n\\n- [ ] x\\n'\n"), 0o755)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent_cmd: sh "+fake+"\n"), 0o644)
	if ag, err := resolveAgent(loadConfig(st.Root)); err != nil || ag == nil {
		t.Fatalf("agent_cmd not resolved: %v", err)
	}
	if err := cmdDraftFeature(io.Discard, io.Discard, "agent feature"); err != nil {
		t.Fatal(err)
	}
	feat := st.Get("FEAT-001")
	if feat == nil || !strings.Contains(feat.Body, "AI content") {
		t.Fatalf("agent body not used: %+v", feat)
	}
	if err := cmdRefine(io.Discard, io.Discard, "FEAT-001"); err != nil {
		t.Fatal(err)
	}
	if st.Get("FEAT-001").Meta["id"][0] != "FEAT-001" {
		t.Fatal("refine clobbered frontmatter")
	}
}

func TestStatusGroupsFinished(t *testing.T) {
	st := setupRepo(t)
	body := "# x\n\n## Summary\n\ns\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [ ] x\n"
	st.Create("feature", "a", body, map[string][]string{"status": {"implemented"}})
	st.Create("feature", "b", body, map[string][]string{"status": {"accepted"}})
	st.Create("feature", "c", body, map[string][]string{"status": {"deprecated"}})
	var buf bytes.Buffer
	if err := cmdStatus(&buf, io.Discard); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// implemented is live (ADR-003): listed with active work, before "finished"
	acc, impl, fin, dep := strings.Index(out, "accepted"), strings.Index(out, "implemented"), strings.Index(out, "finished:"), strings.Index(out, "deprecated")
	if acc < 0 || impl < 0 || fin < 0 || dep < 0 || !(acc < fin && impl < fin && fin < dep) {
		t.Fatalf("accepted and implemented should precede the finished group (deprecated):\n%s", out)
	}
}

func TestValidateTypeMismatch(t *testing.T) {
	st := setupRepo(t)
	adr := "# x\n\n## Context\n\nc\n\n## Decision\n\nd\n\n## Consequences\n\nq\n"
	// type adr, but FEAT id in specs/features → both checks fire
	os.MkdirAll(filepath.Join(st.Root, specDir, "features"), 0o755)
	os.WriteFile(filepath.Join(st.Root, specDir, "features", "FEAT-001-x.md"),
		[]byte("---\nid: FEAT-001\ntype: adr\nstatus: draft\n---\n"+adr), 0o644)
	var buf bytes.Buffer
	if err := cmdValidate(&buf, io.Discard); err == nil {
		t.Fatal("expected validation failure")
	}
	out := buf.String()
	if !strings.Contains(out, "doesn't match id prefix") || !strings.Contains(out, "doesn't match directory") {
		t.Fatalf("missing mismatch errors:\n%s", out)
	}
	// correct id but wrong directory → only the directory check fires
	os.Remove(filepath.Join(st.Root, specDir, "features", "FEAT-001-x.md"))
	os.WriteFile(filepath.Join(st.Root, specDir, "features", "ADR-001-x.md"),
		[]byte("---\nid: ADR-001\ntype: adr\nstatus: draft\n---\n"+adr), 0o644)
	buf.Reset()
	cmdValidate(&buf, io.Discard)
	if out := buf.String(); strings.Contains(out, "id prefix") || !strings.Contains(out, "want specs/adr") {
		t.Fatalf("wrong mismatch errors:\n%s", out)
	}
	// nested dir under the right type dir is fine
	os.Remove(filepath.Join(st.Root, specDir, "features", "ADR-001-x.md"))
	os.MkdirAll(filepath.Join(st.Root, specDir, "adr", "infra"), 0o755)
	os.WriteFile(filepath.Join(st.Root, specDir, "adr", "infra", "ADR-001-x.md"),
		[]byte("---\nid: ADR-001\ntype: adr\nstatus: draft\n---\n"+adr), 0o644)
	if err := cmdValidate(io.Discard, io.Discard); err != nil {
		t.Fatalf("nested adr should validate: %v", err)
	}
}

func TestExtractTypeSuggestion(t *testing.T) {
	out := "# Spec\n\nbody\n\nSPEC-TYPE-SUGGESTION: adr: records a decision between two options\n"
	rest, typ, reason := extractTypeSuggestion(out, "feature")
	if typ != "adr" || reason != "records a decision between two options" || strings.Contains(rest, "SPEC-TYPE") {
		t.Fatalf("got %q %q rest=%q", typ, reason, rest)
	}
	// same type, unknown type, or no marker → no suggestion, marker still stripped
	for _, in := range []string{
		"# S\n\nSPEC-TYPE-SUGGESTION: feature: fits\n",
		"# S\n\nSPEC-TYPE-SUGGESTION: bug: nope\n",
		"# S\n\nbody\n",
	} {
		rest, typ, _ := extractTypeSuggestion(in, "feature")
		if typ != "" || strings.Contains(rest, "SPEC-TYPE") {
			t.Fatalf("%q: got %q rest=%q", in, typ, rest)
		}
	}
}

func TestRefineWarnsOnTypeMismatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	st := setupRepo(t)
	cmdNew(io.Discard, io.Discard, "feature", "pick a db")
	fake := filepath.Join(st.Root, "fakeagent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\ncat > /dev/null\nprintf '# Pick a db\\n\\n## Summary\\n\\nS.\\n\\n## Design\\n\\nD.\\n\\n## Acceptance Criteria\\n\\n- [ ] x\\n\\nSPEC-TYPE-SUGGESTION: adr: chooses Postgres over SQLite\\n'\n"), 0o755)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("agent_cmd: sh "+fake+"\n"), 0o644)
	var errBuf bytes.Buffer
	if err := cmdRefine(io.Discard, &errBuf, "FEAT-001"); err != nil {
		t.Fatal(err)
	}
	if w := errBuf.String(); !strings.Contains(w, "agent suggests adr") || !strings.Contains(w, "Postgres") {
		t.Fatalf("missing type warning: %q", w)
	}
	s := st.Get("FEAT-001")
	if s.Type() != "feature" || strings.Contains(s.Body, "SPEC-TYPE") {
		t.Fatalf("type changed or marker saved: %s %q", s.Type(), s.Body)
	}
}

func TestGlobalConfigMerge(t *testing.T) {
	st := setupRepo(t)
	// global config sets web_port + agent_cmd; repo overrides agent_cmd only
	gdir := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "spec")
	if err := os.MkdirAll(gdir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(gdir, "config.yml"),
		[]byte("web_port: 9999\nagent_cmd: globalcmd {prompt}\n"), 0o644)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent_cmd: repocmd {prompt}\n"), 0o644)
	cfg := loadConfig(st.Root)
	if cfg.WebPort != 9999 {
		t.Fatalf("global web_port not applied: %+v", cfg)
	}
	if cfg.AgentCmd != "repocmd {prompt}" {
		t.Fatalf("repo should override global agent_cmd: %+v", cfg)
	}
}

func TestConfigInitRegenerate(t *testing.T) {
	st := setupRepo(t)
	cfgPath := filepath.Join(st.Root, configDir, "config.yml")
	// config already exists (from spec init) — plain init must refuse
	if err := cmdConfig(io.Discard, io.Discard, []string{"init", "repo"}); err == nil {
		t.Fatal("expected refusal to overwrite existing config")
	}
	// --force regenerates; new file must document every supported key
	if err := cmdConfig(io.Discard, io.Discard, []string{"init", "repo", "--force"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(cfgPath)
	for _, key := range []string{"agent:", "agent_cmd:", "agent_timeout:",
		"web_host:", "web_port:", "statuses:", "verify:", "editor:"} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("regenerated config missing key %q", key)
		}
	}
	// user edits survive until --force
	os.WriteFile(cfgPath, []byte("agent_timeout: 42\n"), 0o644)
	if err := cmdConfig(io.Discard, io.Discard, []string{"init", "repo"}); err == nil {
		t.Fatal("expected refusal without --force")
	}
	if cfg := loadConfig(st.Root); cfg.AgentTimeout != 42 {
		t.Fatalf("config clobbered without --force: %+v", cfg)
	}
}

func TestConfigStringShowsAllKeys(t *testing.T) {
	setupRepo(t)
	var buf bytes.Buffer
	if err := cmdConfig(&buf, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"verify:", "editor:"} {
		if !strings.Contains(buf.String(), key) {
			t.Fatalf("spec config output missing %q: %s", key, buf.String())
		}
	}
}

func TestInlineCommentStripping(t *testing.T) {
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent: devin  # claude | gemini | amp\nweb_port: 8471 # web port\n"), 0o644)
	cfg := loadConfig(st.Root)
	if cfg.Agent != "devin" || cfg.WebPort != 8471 {
		t.Fatalf("inline comments not stripped: %+v", cfg)
	}
	os.WriteFile(factsPath(st.Root), []byte("domain: exemplo.pt # prod domain\n"), 0o644)
	if got := factsMap(loadFacts(st.Root))["domain"]; got != "exemplo.pt" {
		t.Fatalf("fact inline comment: %q", got)
	}
}

func TestMissingAgentErrors(t *testing.T) {
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("agent: nosuchtool\n"), 0o644)
	if _, err := resolveAgent(loadConfig(st.Root)); err == nil {
		t.Fatal("expected error for unknown agent")
	}
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("agent: claude\n"), 0o644)
	if _, err := resolveAgent(loadConfig(st.Root)); err == nil {
		t.Skip("claude is installed; can't test missing binary")
	}
}

func TestRefineBadAgentOutputDumpsToStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	st := setupRepo(t)
	cmdNew(io.Discard, io.Discard, "feature", "x")
	fake := filepath.Join(st.Root, "badagent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\ncat > /dev/null\necho 'sorry I cannot help with that'\n"), 0o755)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent_cmd: sh "+fake+"\n"), 0o644)
	var outBuf, errBuf bytes.Buffer
	err := cmdRefine(&outBuf, &errBuf, "FEAT-001")
	if err == nil {
		t.Fatal("expected refine to fail on garbage agent output")
	}
	if !strings.Contains(errBuf.String(), "sorry I cannot help") {
		t.Fatalf("agent output not dumped to stderr: %q", errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "Refining FEAT-001") {
		t.Fatalf("missing start announcement: %q", outBuf.String())
	}
}

func TestApplyPromptIncludesVerify(t *testing.T) {
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("verify: [go test ./..., spec validate]\n"), 0o644)
	s, err := st.Create("feature", "x", "# x\n\n## Summary\n\ns\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	meta := s.Meta
	meta["status"] = []string{"accepted"}
	os.WriteFile(s.Path, []byte(dumpFrontmatter(meta, s.Body)), 0o644)
	var buf bytes.Buffer
	if err := cmdApply(strings.NewReader(""), &buf, io.Discard, nil, applyOpts{Print: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "unit tests") || !strings.Contains(out, "go test ./...") {
		t.Fatalf("verify/test instructions missing from apply prompt:\n%s", out)
	}
}

// ADR-003: agents must keep other specs accurate, and archived specs are
// flagged as history in the prompt's spec list.
func TestApplyPromptKeepsSpecsCurrent(t *testing.T) {
	st := setupRepo(t)
	featureWithStatus(t, st, "target", "accepted")
	old := featureWithStatus(t, st, "old", "deprecated")
	if err := cmdArchive(io.Discard, []string{old}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := cmdApply(strings.NewReader(""), &buf, io.Discard, []string{"FEAT-001"}, applyOpts{Print: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"update that spec's text in the same change",
		"Do not change other specs' status",
		"list any other specs you updated",
		old + " (feature, deprecated, archived — history, may be outdated)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("apply prompt missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Do not edit the spec files under specs/") {
		t.Fatal("old no-edit rule still in the prompt")
	}
}

func TestRefineWithoutAgentFails(t *testing.T) {
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("agent: none\n"), 0o644)
	cmdNew(io.Discard, io.Discard, "feature", "x")
	if err := cmdRefine(io.Discard, io.Discard, "FEAT-001"); err == nil {
		t.Fatal("expected error with agent: none")
	}
}

func TestToUTF8(t *testing.T) {
	// UTF-16LE with BOM
	u16le := append([]byte{0xFF, 0xFE}, []byte("#\x00 \x00t\x00i\x00t\x00l\x00e\x00")...)
	if got := toUTF8(u16le); got != "# title" {
		t.Fatalf("UTF-16LE BOM: %q", got)
	}
	// UTF-16BE with BOM
	u16be := append([]byte{0xFE, 0xFF}, []byte("\x00#\x00 \x00t\x00i\x00t\x00l\x00e\x00")...)
	if got := toUTF8(u16be); got != "# title" {
		t.Fatalf("UTF-16BE BOM: %q", got)
	}
	// UTF-16LE without BOM
	if got := toUTF8([]byte("H\x00i\x00 \x00t\x00h\x00e\x00r\x00e\x00")); got != "Hi there" {
		t.Fatalf("UTF-16LE no BOM: %q", got)
	}
	// UTF-8 passthrough
	if got := toUTF8([]byte("# Título\n")); got != "# Título\n" {
		t.Fatalf("UTF-8 passthrough: %q", got)
	}
}

func TestFacts(t *testing.T) {
	st := setupRepo(t)
	// list on empty file
	var buf bytes.Buffer
	if err := cmdFact(&buf, io.Discard, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No facts") {
		t.Fatalf("expected empty facts notice, got %q", buf.String())
	}
	// set + get
	if err := cmdFact(io.Discard, io.Discard, []string{"set", "domain", "exemplo.pt"}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := cmdFact(&buf, io.Discard, []string{"get", "domain"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "exemplo.pt" {
		t.Fatalf("get: %q", buf.String())
	}
	// overwrite preserves file
	if err := cmdFact(io.Discard, io.Discard, []string{"set", "domain", "novo.pt"}); err != nil {
		t.Fatal(err)
	}
	if v := factsMap(loadFacts(st.Root))["domain"]; v != "novo.pt" {
		t.Fatalf("set overwrite: %q", v)
	}
	// substitution at create time
	if _, err := st.Create("feature", "sub test", "# t\n\n## Summary\n\nDomain: {fact:domain}\n", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.Get("FEAT-001").Body, "Domain: novo.pt") {
		t.Fatalf("substitution failed: %q", st.Get("FEAT-001").Body)
	}
	// check: undefined reference fails
	st.Create("feature", "bad ref", "# t\n\n## Summary\n\n{fact:missing}\n", nil)
	buf.Reset()
	if err := cmdCheck(&buf, io.Discard); err == nil {
		t.Fatal("check should fail on undefined fact reference")
	}
	if !strings.Contains(buf.String(), "{fact:missing}") {
		t.Fatalf("check output: %q", buf.String())
	}
	// unset
	if err := cmdFact(io.Discard, io.Discard, []string{"unset", "domain"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := factsMap(loadFacts(st.Root))["domain"]; ok {
		t.Fatal("unset failed")
	}
	if err := cmdFact(io.Discard, io.Discard, []string{"unset", "domain"}); err == nil {
		t.Fatal("unset of missing key should error")
	}
}

func TestSlugify(t *testing.T) {
	if got := slugify("Hello, World! 2"); got != "hello-world-2" {
		t.Fatal(got)
	}
	if got := slugify("!!!"); got != "untitled" {
		t.Fatal(got)
	}
}
