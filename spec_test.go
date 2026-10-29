package main

import (
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
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
	if err := cmdInit(); err != nil {
		t.Fatal(err)
	}
	return Store{Root: dir}
}

func TestInitAndNew(t *testing.T) {
	st := setupRepo(t)
	for _, typ := range []string{"project", "feature", "adr"} {
		if err := cmdNew(typ, "My "+typ); err != nil {
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
	if err := cmdNew("bogus", "x"); err == nil {
		t.Fatal("expected error for bad type")
	}
}

func TestValidateCatchesMissingSection(t *testing.T) {
	setupRepo(t)
	if err := cmdNew("feature", "broken"); err != nil {
		t.Fatal(err)
	}
	st := mustStore()
	s := st.Get("FEAT-001")
	if err := os.WriteFile(s.Path, []byte("---\nid: FEAT-001\ntype: feature\n---\n# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdValidate(); err == nil {
		t.Fatal("expected validation failure")
	}
}

func TestValidateOK(t *testing.T) {
	setupRepo(t)
	cmdNew("project", "p")
	cmdNew("feature", "f")
	if err := cmdValidate(); err != nil {
		t.Fatal(err)
	}
}

func TestSearchAndTrace(t *testing.T) {
	st := setupRepo(t)
	body := "# p\n\n## Overview\n\nx\n\n## Requirements\n\n- REQ-001: login support\n"
	if _, err := st.Create("project", "p", body, nil); err != nil {
		t.Fatal(err)
	}
	if err := cmdSearch("login support"); err != nil {
		t.Fatal(err)
	}
	if err := cmdDraftFeature("add login page"); err != nil {
		t.Fatal(err)
	}
	feat := st.Get("FEAT-001")
	if feat == nil || !strings.Contains(feat.Body, "REQ-001") {
		t.Fatalf("draft should link REQ-001: %+v", feat)
	}
	if err := cmdTrace(); err != nil {
		t.Fatal(err)
	}
}

func TestImportAndExport(t *testing.T) {
	st := setupRepo(t)
	src := filepath.Join(st.Root, "notes.md")
	os.WriteFile(src, []byte("# Notes\n\nhello\n"), 0o644)
	if err := cmdImport(src); err != nil {
		t.Fatal(err)
	}
	if st.Get("DOC-001") == nil {
		t.Fatal("import failed")
	}
	out := filepath.Join(st.Root, "meta.json")
	if err := cmdExport(out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if !strings.Contains(string(data), "DOC-001") {
		t.Fatal("export missing spec")
	}
}

func TestBootstrapAndRoadmapAndGraph(t *testing.T) {
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, "go.mod"), []byte("module x\n"), 0o644)
	if err := cmdBootstrap(); err != nil {
		t.Fatal(err)
	}
	if st.Get("PROJ-001") == nil {
		t.Fatal("bootstrap created no project spec")
	}
	if err := cmdRoadmap(filepath.Join(st.Root, "ROADMAP.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Root, "ROADMAP.md")); err != nil {
		t.Fatal(err)
	}
	if err := cmdGraph(false); err != nil {
		t.Fatal(err)
	}
	if err := cmdGraph(true); err != nil {
		t.Fatal(err)
	}
	if err := cmdAnalyze(); err != nil {
		t.Fatal(err)
	}
}

func TestConfigParsing(t *testing.T) {
	st := setupRepo(t)
	cfg := loadConfig(st.Root)
	if cfg.Agent != "auto" || cfg.AgentTimeout != 300 {
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
	if ag := resolveAgent(loadConfig(st.Root)); ag == nil {
		t.Fatal("agent_cmd not resolved")
	}
	if err := cmdDraftFeature("agent feature"); err != nil {
		t.Fatal(err)
	}
	feat := st.Get("FEAT-001")
	if feat == nil || !strings.Contains(feat.Body, "AI content") {
		t.Fatalf("agent body not used: %+v", feat)
	}
	if err := cmdRefine("FEAT-001"); err != nil {
		t.Fatal(err)
	}
	if st.Get("FEAT-001").Meta["id"][0] != "FEAT-001" {
		t.Fatal("refine clobbered frontmatter")
	}
}

func TestRefineWithoutAgentFails(t *testing.T) {
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("agent: none\n"), 0o644)
	cmdNew("feature", "x")
	if err := cmdRefine("FEAT-001"); err == nil {
		t.Fatal("expected error with agent: none")
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
