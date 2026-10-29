package main

import (
	"os"
	"path/filepath"
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

func TestSlugify(t *testing.T) {
	if got := slugify("Hello, World! 2"); got != "hello-world-2" {
		t.Fatal(got)
	}
	if got := slugify("!!!"); got != "untitled" {
		t.Fatal(got)
	}
}
