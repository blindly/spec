package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePersona(t *testing.T, st Store, name, content string) {
	t.Helper()
	dir := personaDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPersonaResolutionAndUse(t *testing.T) {
	st := setupRepo(t)
	writePersona(t, st, "dpa", "# DPA\n\nYou are a DPA dev.\n")
	writePersona(t, st, "go", "# Go\n\nYou are a Go dev.\n")

	role, source := roleFor(st, loadConfig(st.Root))
	if role != "" || source != "" {
		t.Fatalf("no role expected, got %q from %q", role, source)
	}

	var out, errs bytes.Buffer
	if err := cmdPersona([]string{"use", "dpa"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	cfg := loadConfig(st.Root)
	if cfg.Persona != "dpa" {
		t.Fatalf("persona not written to config: %q", cfg.Persona)
	}
	role, source = roleFor(st, cfg)
	if role != "# DPA\n\nYou are a DPA dev." || !strings.Contains(source, `persona "dpa"`) {
		t.Fatalf("role=%q source=%q", role, source)
	}
	if !strings.Contains(repoContext(st, 50), "You are a DPA dev.") {
		t.Fatal("persona absent from repoContext")
	}

	// listing marks the active persona
	out.Reset()
	if err := cmdPersona(nil, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "* dpa") || !strings.Contains(out.String(), "  go") {
		t.Fatalf("listing: %q", out.String())
	}

	// spec role shows effective role + source note
	out.Reset()
	errs.Reset()
	if code := run([]string{"role"}, &out, &errs); code != 0 {
		t.Fatal(errs.String())
	}
	if !strings.Contains(out.String(), "You are a DPA dev.") || !strings.Contains(errs.String(), `(from persona "dpa"`) {
		t.Fatalf("role output %q / %q", out.String(), errs.String())
	}

	// project role wins
	writeFile(t, st, ".spec/role.md", "PROJECT ROLE")
	if role, source = roleFor(st, loadConfig(st.Root)); role != "PROJECT ROLE" || source != ".spec/role.md" {
		t.Fatalf("project role lost: %q %q", role, source)
	}
	if !strings.Contains(repoContext(st, 50), "PROJECT ROLE") {
		t.Fatal("project role absent from repoContext")
	}

	// use none clears the key
	out.Reset()
	if err := cmdPersona([]string{"use", "none"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if cfg := loadConfig(st.Root); cfg.Persona != "" {
		t.Fatalf("persona not cleared: %q", cfg.Persona)
	}

	// using a missing persona fails
	if err := cmdPersona([]string{"use", "missing"}, &out, &errs); err == nil {
		t.Fatal("use of missing persona accepted")
	}
	// traversal names are rejected everywhere
	if loadPersona("../role") != "" || cmdPersona([]string{"use", "../role"}, &out, &errs) == nil {
		t.Fatal("traversal name accepted")
	}
}

func TestPersonaView(t *testing.T) {
	st := setupRepo(t)
	writePersona(t, st, "dpa", "# DPA\n\nYou are a DPA dev.\n")
	writePersona(t, st, "go", "# Go\n\nYou are a Go dev.\n")

	var out, errs bytes.Buffer
	// verbatim file contents to stdout
	if err := cmdPersona([]string{"view", "go"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if out.String() != "# Go\n\nYou are a Go dev.\n" {
		t.Fatalf("view output: %q", out.String())
	}
	// no name and no active persona errors
	if err := cmdPersona([]string{"view"}, &out, &errs); err == nil {
		t.Fatal("view without name or active persona accepted")
	}
	// no name cats the active persona
	if err := cmdPersona([]string{"use", "dpa"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := cmdPersona([]string{"view"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if out.String() != "# DPA\n\nYou are a DPA dev.\n" {
		t.Fatalf("view output: %q", out.String())
	}
	// missing and invalid names are rejected
	if err := cmdPersona([]string{"view", "missing"}, &out, &errs); err == nil {
		t.Fatal("view of missing persona accepted")
	}
	if err := cmdPersona([]string{"view", "../role"}, &out, &errs); err == nil {
		t.Fatal("traversal name accepted")
	}
}

func TestRoleAdopt(t *testing.T) {
	st := setupRepo(t)
	writePersona(t, st, "dpa", "# DPA\n\nYou are a DPA dev.\n")
	var out, errs bytes.Buffer

	// nothing to adopt
	if err := cmdRole([]string{"adopt"}, &out, &errs); err == nil {
		t.Fatal("adopt without persona accepted")
	}
	if err := cmdPersona([]string{"use", "dpa"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if err := cmdRole([]string{"adopt"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if got := loadRole(st.Root); got != "# DPA\n\nYou are a DPA dev." {
		t.Fatalf("adopted role: %q", got)
	}
	// second adopt refuses — project role exists now
	if err := cmdRole([]string{"adopt"}, &out, &errs); err == nil {
		t.Fatal("adopt over existing role accepted")
	}
}

func TestPersonaRefineIsRepoBlind(t *testing.T) {
	st := setupRepo(t)
	writePersona(t, st, "dpa", "# DPA\n\nOld text.\n")
	fakeRefineAgent(t, st, "# DPA\n\nYou are a DPA dev.\n")
	writeFile(t, st, "fakeagent.sh", "#!/bin/sh\ncat > prompt.txt\ncat agent-out.txt\n")
	var out, errs bytes.Buffer
	if err := cmdPersona([]string{"refine", "dpa"}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if got := loadPersona("dpa"); got != "# DPA\n\nYou are a DPA dev." {
		t.Fatalf("persona not updated: %q", got)
	}
	prompt, err := os.ReadFile(filepath.Join(st.Root, "prompt.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(prompt), "Repository root:") {
		t.Fatal("repo context leaked into persona refine")
	}
	if !strings.Contains(string(prompt), "Current persona:\n# DPA\n\nOld text.") {
		t.Fatal("current persona missing from prompt")
	}
	// bad output leaves the persona alone
	fakeRefineAgent(t, st, "no heading here\n")
	if err := cmdPersona([]string{"refine", "dpa"}, &out, &errs); err == nil {
		t.Fatal("heading-less output accepted")
	}
	if loadPersona("dpa") != "# DPA\n\nYou are a DPA dev." {
		t.Fatal("persona lost on bad output")
	}
}
