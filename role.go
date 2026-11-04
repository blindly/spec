package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// loadRole deliberately reads only repository context, never global config.
func loadRole(root string) string {
	data, err := os.ReadFile(filepath.Join(root, configDir, "role.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cmdRole(args []string, out, errOut io.Writer) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "edit" && args[0] != "refine") {
		return fmt.Errorf("usage: spec role [edit|refine]")
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	role := loadRole(st.Root)
	if len(args) == 0 {
		if role == "" {
			fmt.Fprintln(errOut, "No role set; use spec role edit or spec role refine to create .spec/role.md.")
		} else {
			fmt.Fprintln(out, role)
		}
		return nil
	}
	path := filepath.Join(st.Root, configDir, "role.md")
	cfg := loadConfig(st.Root)
	if args[0] == "edit" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Fprintf(out, "Opening %s with %s\n", path, cfg.editor())
		cmd := exec.Command(cfg.editor(), path)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	}
	ag, err := resolveAgent(cfg)
	if err != nil {
		return err
	}
	if ag == nil {
		return fmt.Errorf("role refine requires an LLM agent (none configured; set agent/agent_cmd in .spec/config.yml)")
	}
	prompt := fmt.Sprintf(`You are writing the development role file %s/role.md for a spec-managed repository. The file's entire contents tell the agent who it is and how it develops in this repo.

Improve the current role below, or draft one when no role is set. Describe the persona and concrete development conventions the agent should adopt. A role is often defined before development begins, so do not assume an established codebase — the repository context below is optional input, not a requirement. Preserve project-specific intent and avoid inventing constraints. Read files only; do not modify files.

Your reply is written verbatim to the file, so output ONLY the file contents: a markdown document that starts with a single "# " heading followed by a one-sentence persona statement ("You are a ..."). Keep the rest focused on concrete conventions — do not re-document tools or workflows the agent already knows. No commentary, no narration, no frontmatter, no surrounding code fences.

Current role:
%s

%s`, configDir, role, repoContext(st, 50))
	fmt.Fprintf(out, "Refining role via %s…\n", ag.name)
	output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
	if err != nil {
		return err
	}
	body := strings.TrimSpace(bodyFromAgentOutput(output))
	if !strings.HasPrefix(body, "# ") {
		dumpAgentOutput(errOut, output)
		return fmt.Errorf("agent returned no markdown document — role left unchanged")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, []byte(body+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Refined", path, "via", ag.name)
	return nil
}
