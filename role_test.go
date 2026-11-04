package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRoleLoadingAndContext(t *testing.T) {
	st := setupRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "spec"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "spec", "role.md"), []byte("GLOBAL ROLE"), 0644); err != nil {
		t.Fatal(err)
	}
	baseline := repoContext(st, 50)
	for _, content := range []string{"", "  \n\t", "  Senior Go engineer.\nUse stdlib only. \n"} {
		writeFile(t, st, ".spec/role.md", content)
		want := strings.TrimSpace(content)
		if got := loadRole(st.Root); got != want {
			t.Fatalf("role = %q, want %q", got, want)
		}
		ctx := repoContext(st, 50)
		if strings.Contains(ctx, "GLOBAL ROLE") {
			t.Fatal("global role leaked")
		}
		var out, errs bytes.Buffer
		if code := run([]string{"role"}, &out, &errs); code != 0 {
			t.Fatal(errs.String())
		}
		var status bytes.Buffer
		if err := cmdStatus(&status, &errs); err != nil {
			t.Fatal(err)
		}
		if want == "" {
			if ctx != baseline || out.Len() != 0 || !strings.Contains(errs.String(), "No role set") || strings.Contains(status.String(), "Role:") {
				t.Fatal("unset role changed context/output")
			}
		} else if !strings.Contains(ctx, "Development role (follow this persona and these conventions):\n"+want) || out.String() != want+"\n" || !strings.Contains(status.String(), "Role: .spec/role.md") {
			t.Fatal("role missing from context/output")
		}
	}
}

func TestRoleRefine(t *testing.T) {
	for _, current := range []string{"", " \n", "Use Go and small diffs."} {
		t.Run(current, func(t *testing.T) {
			st := setupRepo(t)
			if current != "" {
				writeFile(t, st, ".spec/role.md", current)
			}
			fakeRefineAgent(t, st, "# Development role\n\nSenior Go engineer.\n\nUse stdlib only.\n")
			// Capture the actual prompt sent to the configured agent.
			writeFile(t, st, "fakeagent.sh", "#!/bin/sh\ncat > prompt.txt\ncat agent-out.txt\n")
			var out, errs bytes.Buffer
			if code := run([]string{"role", "refine"}, &out, &errs); code != 0 {
				t.Fatal(errs.String())
			}
			want := "# Development role\n\nSenior Go engineer.\n\nUse stdlib only."
			if got := loadRole(st.Root); got != want {
				t.Fatal(got)
			}
			prompt, err := os.ReadFile(filepath.Join(st.Root, "prompt.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(prompt), "Repository root:") || !strings.Contains(string(prompt), "Current role:\n"+strings.TrimSpace(current)) {
				t.Fatal(string(prompt))
			}
			// narration before the document is stripped
			fakeRefineAgent(t, st, "I'll explore the repository first.\n\n# Development role\n\nSenior Go engineer.\n\nUse stdlib only.\n")
			if err := cmdRole([]string{"refine"}, &out, &errs); err != nil {
				t.Fatal(err)
			}
			if got := loadRole(st.Root); got != want {
				t.Fatalf("narration kept: %q", got)
			}
			// output without a heading is rejected and leaves the role alone
			fakeRefineAgent(t, st, "Just plain text, no heading.\n")
			if err := cmdRole([]string{"refine"}, &out, &errs); err == nil {
				t.Fatal("heading-less output accepted")
			}
			if loadRole(st.Root) != want {
				t.Fatal("role lost on bad output")
			}
			fakeRefineAgent(t, st, " \n")
			if err := cmdRole([]string{"refine"}, &out, &errs); err == nil {
				t.Fatal("empty output accepted")
			}
			if loadRole(st.Root) != want {
				t.Fatal("role lost on failure")
			}
			writeFile(t, st, ".spec/config.yml", "agent_cmd: sh -c 'exit 1'\n")
			if err := cmdRole([]string{"refine"}, &out, &errs); err == nil {
				t.Fatal("agent failure ignored")
			}
			if loadRole(st.Root) != want {
				t.Fatal("role lost on agent failure")
			}
		})
	}
}

func TestRoleRefineRequiresAgent(t *testing.T) {
	st := setupRepo(t)
	writeFile(t, st, ".spec/config.yml", "agent: none\nagent_cmd: \n")
	var out bytes.Buffer
	if err := cmdRole([]string{"refine"}, &out, &out); err == nil || !strings.Contains(err.Error(), "requires an LLM agent") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(st.Root, ".spec/role.md")); !os.IsNotExist(err) {
		t.Fatal("role created without agent")
	}
}

func TestRoleEdit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell editor")
	}
	st := setupRepo(t)
	editor := filepath.Join(st.Root, "editor.sh")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\ntest -f \"$1\" || exit 1\nprintf '\\nEdited' >> \"$1\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, st, ".spec/config.yml", "editor: "+editor+"\n")
	var out bytes.Buffer
	for _, want := range []string{"Edited", "Edited\nEdited"} {
		if err := cmdRole([]string{"edit"}, &out, &out); err != nil {
			t.Fatal(err)
		}
		if got := loadRole(st.Root); got != want {
			t.Fatalf("got %q", got)
		}
	}
}

func TestRoleInDriftReviewPrompt(t *testing.T) {
	st, _, _ := driftedSpec(t)
	writeFile(t, st, ".spec/role.md", "Senior Go engineer; minimal diffs.")
	promptFile := fakeReviewAgent(t, st, "VERDICT: accurate\nREASON: still accurate\n")
	var out bytes.Buffer
	if err := cmdDrift(strings.NewReader(""), &out, &out, []string{"--review"}); err != nil {
		t.Fatal(err)
	}
	prompt, err := os.ReadFile(promptFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prompt), "Development role (follow this persona and these conventions):\nSenior Go engineer; minimal diffs.") {
		t.Fatal("role absent from drift review")
	}
}
