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

func TestFrontmatterNoHeader(t *testing.T) {
	meta, body := parseFrontmatter("# Just markdown\n")
	if len(meta) != 0 || body != "# Just markdown\n" {
		t.Fatalf("expected passthrough, got %v %q", meta, body)
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
	if err := cmdApply(&buf, io.Discard, nil, true, false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "unit tests") || !strings.Contains(out, "go test ./...") {
		t.Fatalf("verify/test instructions missing from apply prompt:\n%s", out)
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
