package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverSeeding(t *testing.T) {
	for _, agent := range []bool{false, true} {
		t.Run(map[bool]string{false: "offline", true: "agent"}[agent], func(t *testing.T) {
			if agent && runtime.GOOS == "windows" {
				t.Skip("shell-script fake agent")
			}
			st := setupRepo(t)
			cfg := "agent: none\n"
			if agent {
				script := filepath.Join(st.Root, "agent.sh")
				os.WriteFile(script, []byte("#!/bin/sh\ncat >/dev/null\nprintf 'email: hi@example.test — README.md\\n'\n"), 0755)
				cfg = "agent_cmd: sh " + script + "\n"
			}
			os.WriteFile(filepath.Join(st.Root, ".spec", "config.yml"), []byte(cfg), 0644)
			st.Create("feature", "references", "# references\n{fact:missing} {fact:missing}", nil)
			os.MkdirAll(filepath.Join(st.Root, "specs", "archive", "features"), 0755)
			os.WriteFile(filepath.Join(st.Root, "specs", "archive", "features", "FEAT-099-old.md"), []byte("---\nid: FEAT-099\ntype: feature\nstatus: rejected\ntitle: old\n---\n{fact:archived}\n"), 0644)
			original := "# keep this\nexisting: value # inline\n"
			os.WriteFile(factsPath(st.Root), []byte(original), 0644)
			var out bytes.Buffer
			if err := discoverFacts(st, strings.NewReader("n\n"), &out, io.Discard, false); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(factsPath(st.Root))
			if !strings.HasPrefix(string(data), original) || strings.Count(string(data), "missing:") != 1 || !strings.Contains(string(data), "archived: \n") {
				t.Fatalf("facts: %s", data)
			}
			if _, ok := factsMap(loadFacts(st.Root))["email"]; ok {
				t.Fatal("declined proposal written")
			}
			if agent && !strings.Contains(out.String(), "email: hi@example.test — README.md") {
				t.Fatal(out.String())
			}
			var check bytes.Buffer
			if err := cmdCheck(&check, io.Discard); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(check.String(), "empty value") {
				t.Fatal(check.String())
			}
		})
	}
}

func TestDiscoverStructuredConfirmationAndIdempotency(t *testing.T) {
	for _, answer := range []string{"n\n", "yes\n", "--yes"} {
		t.Run(strings.TrimSpace(answer), func(t *testing.T) {
			st := setupRepo(t)
			os.WriteFile(filepath.Join(st.Root, ".spec", "config.yml"), []byte("agent: none\n"), 0644)
			os.WriteFile(filepath.Join(st.Root, "go.mod"), []byte("module example.test/tools\n"), 0644)
			os.WriteFile(filepath.Join(st.Root, "package.json"), []byte(`{"name":"tools","homepage":"https://example.test"}`), 0644)
			os.WriteFile(filepath.Join(st.Root, "pyproject.toml"), []byte("[unrelated]\nname='wrong'\n[project]\nname = 'python-tools' # comment\n"), 0644)
			for _, args := range [][]string{{"init"}, {"remote", "add", "origin", "git@example.test:org/tools.git"}} {
				cmd := exec.Command("git", args...)
				cmd.Dir = st.Root
				if data, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v", data, err)
				}
			}
			var out bytes.Buffer
			if err := discoverFacts(st, strings.NewReader(answer), &out, io.Discard, answer == "--yes"); err != nil {
				t.Fatal(err)
			}
			facts := factsMap(loadFacts(st.Root))
			if answer == "n\n" {
				if len(facts) != 0 {
					t.Fatal(facts)
				}
				return
			}
			for k, v := range map[string]string{"go_module": "example.test/tools", "package_name": "tools", "homepage": "https://example.test", "python_project_name": "python-tools", "git_remote_host": "example.test"} {
				if facts[k] != v {
					t.Fatalf("%s: %q", k, facts[k])
				}
			}
			before, _ := os.ReadFile(factsPath(st.Root))
			out.Reset()
			if err := discoverFacts(st, strings.NewReader(""), &out, io.Discard, true); err != nil {
				t.Fatal(err)
			}
			after, _ := os.ReadFile(factsPath(st.Root))
			if !bytes.Equal(before, after) || !strings.Contains(out.String(), "Proposed 0 candidates") {
				t.Fatal(out.String())
			}
		})
	}
}

func TestDiscoverAgentConflictsAndFallback(t *testing.T) {
	for _, mode := range []string{"success", "failure", "invalid", "resolve"} {
		t.Run(mode, func(t *testing.T) {
			// the failure modes exercise the fallback path, which a missing
			// sh produces anyway; only success needs the script to run
			if mode == "success" && runtime.GOOS == "windows" {
				t.Skip("shell-script fake agent")
			}
			st := setupRepo(t)
			setFact(st.Root, "domain", "existing.test")
			os.WriteFile(filepath.Join(st.Root, "go.mod"), []byte("module fallback.test/tools\n"), 0644)
			script := filepath.Join(st.Root, "mock.sh")
			body := "cat > prompt.txt\nprintf 'domain: proposed.test — README.md\\nemail: hello@example.test — contact.txt\\n'\n"
			if mode == "failure" {
				body = "exit 1\n"
			}
			if mode == "invalid" {
				body = "echo nonsense\n"
			}
			os.WriteFile(script, []byte(body), 0755)
			cfg := "agent_cmd: sh " + script + "\nagent_timeout: 5\n"
			if mode == "resolve" {
				cfg = "agent: unknown\n"
			}
			os.WriteFile(filepath.Join(st.Root, ".spec", "config.yml"), []byte(cfg), 0644)
			var out, warn bytes.Buffer
			if err := discoverFacts(st, strings.NewReader(""), &out, &warn, true); err != nil {
				t.Fatal(err)
			}
			facts := factsMap(loadFacts(st.Root))
			if facts["domain"] != "existing.test" {
				t.Fatal(facts)
			}
			if mode == "success" {
				if facts["email"] != "hello@example.test" || !strings.Contains(out.String(), "Conflict domain") {
					t.Fatal(out.String())
				}
				prompt, _ := os.ReadFile(filepath.Join(st.Root, "prompt.txt"))
				if !strings.Contains(string(prompt), "Never invent") || !strings.Contains(string(prompt), "existing.test") {
					t.Fatal(string(prompt))
				}
			} else if facts["go_module"] != "fallback.test/tools" || !strings.Contains(warn.String(), "falling back") {
				t.Fatalf("%v %s", facts, warn.String())
			}
		})
	}
}

func TestDiscoverUsage(t *testing.T) {
	setupRepo(t)
	if err := cmdFact(io.Discard, io.Discard, []string{"discover", "--bad"}); err == nil {
		t.Fatal("invalid flag accepted")
	}
	if !strings.Contains(usage, "discover [--yes]") {
		t.Fatal("missing usage")
	}
}
