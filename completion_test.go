package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// FEAT-028: spec <cmd> --help / spec help <cmd> print that command's block.
func TestCommandHelp(t *testing.T) {
	st := setupRepo(t)
	_ = st
	for _, args := range [][]string{
		{"skill", "--help"},
		{"skill", "-h"},
		{"help", "skill"},
	} {
		var out bytes.Buffer
		if code := run(args, &out, io.Discard); code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		for _, want := range []string{"spec skill", "use|unuse", "--global"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("%v: help missing %q:\n%s", args, want, out.String())
			}
		}
	}
}

// FEAT-028: full help still works; every dispatchable command has a doc
// block; unknown commands still error.
func TestCommandHelpCoverage(t *testing.T) {
	for name := range commands {
		if _, ok := commandDocByName[name]; !ok {
			t.Fatalf("command %q has no help block", name)
		}
	}
	var out bytes.Buffer
	if code := run([]string{"help"}, &out, io.Discard); code != 0 {
		t.Fatalf("spec help: exit %d", code)
	}
	for _, want := range []string{"spec init", "spec skill", "spec completion"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("full help missing %q", want)
		}
	}
	// main help is assembled from the same blocks
	for name, doc := range commandDocByName {
		if name == "list" { // alias shares ls's block
			continue
		}
		if !strings.Contains(out.String(), doc) {
			t.Fatalf("full help missing block for %q", name)
		}
	}
	var errOut bytes.Buffer
	if code := run([]string{"frobnicate", "--help"}, io.Discard, &errOut); code == 0 {
		t.Fatal("unknown command --help should fail")
	}
	if !strings.Contains(errOut.String(), "unknown command") {
		t.Fatalf("expected unknown-command error:\n%s", errOut.String())
	}
	// --help reaches the doc, not the command's arg parser
	out.Reset()
	if code := run([]string{"skill", "use", "--help"}, &out, io.Discard); code != 0 {
		t.Fatalf("subcommand --help: exit %d", code)
	}
	if !strings.Contains(out.String(), "spec skill") {
		t.Fatalf("expected skill help:\n%s", out.String())
	}
}

// FEAT-029: each shell's script generates and embeds command + sub lists.
func TestCompletionScripts(t *testing.T) {
	for _, tc := range []struct {
		shell   string
		markers []string
	}{
		{"bash", []string{"complete -", "spec __complete skill", "--global"}},
		{"zsh", []string{"#compdef spec", "spec __complete skill", "compadd"}},
		{"fish", []string{"complete -c spec", "spec __complete spec-id", "use unuse adopt"}},
	} {
		var out bytes.Buffer
		if err := cmdCompletion(&out, io.Discard, []string{tc.shell}); err != nil {
			t.Fatalf("%s: %v", tc.shell, err)
		}
		for _, m := range tc.markers {
			if !strings.Contains(out.String(), m) {
				t.Fatalf("%s script missing %q", tc.shell, m)
			}
		}
	}
	var errOut bytes.Buffer
	if err := cmdCompletion(io.Discard, &errOut, []string{"tcsh"}); err == nil {
		t.Fatal("unknown shell should error")
	}
}

// FEAT-029: dynamic kinds resolve repo state; static kinds always work.
func TestCompleteCandidates(t *testing.T) {
	st := setupRepo(t)
	if _, err := st.Create("feature", "Widget spec", "# x\n", nil); err != nil {
		t.Fatal(err)
	}
	var ids bytes.Buffer
	cmdCompleteKinds(&ids, []string{"spec-id"})
	if !strings.Contains(ids.String(), "FEAT-001") {
		t.Fatalf("spec-id candidates missing FEAT-001:\n%s", ids.String())
	}
	var out bytes.Buffer
	cmdCompleteKinds(&out, []string{"shell"})
	if out.String() != "bash\nzsh\nfish\n" {
		t.Fatalf("shell candidates: %q", out.String())
	}
	out.Reset()
	cmdCompleteKinds(&out, []string{"skill"})
	if !strings.Contains(out.String(), "spec") {
		t.Fatalf("skill candidates missing built-in:\n%s", out.String())
	}
	out.Reset()
	cmdCompleteKinds(&out, []string{"persona-or-none"})
	if !strings.Contains(out.String(), "none") {
		t.Fatalf("persona-or-none missing none:\n%s", out.String())
	}
	// every commands-map command has a table entry
	for name := range commands {
		if _, ok := completionTable[name]; !ok {
			t.Fatalf("command %q missing from completionTable", name)
		}
	}
}

// FEAT-029: repo-free — dynamic kinds degrade silently, static still work.
func TestCompleteOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	defer func() { os.Chdir(old) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmdCompleteKinds(&out, []string{"spec-id"})
	if out.String() != "" {
		t.Fatalf("spec-id outside a repo should be empty, got %q", out.String())
	}
	cmdCompleteKinds(&out, []string{"shell"})
	if out.String() == "" {
		t.Fatal("static kinds should work outside a repo")
	}
}
