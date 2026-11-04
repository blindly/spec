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
	prompt := "Improve the current development role below, or draft one from the repository when no role is set. Describe the persona and concrete development conventions the agent should adopt. Preserve project-specific intent and infer conventions from repository files without inventing constraints. Read files only; do not modify files. Return ONLY the free-form markdown role, without frontmatter or surrounding code fences.\n\nCurrent role:\n" + role + "\n\n" + repoContext(st, 50)
	fmt.Fprintf(out, "Refining role via %s…\n", ag.name)
	output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
	if err != nil {
		return err
	}
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Errorf("agent returned an empty role — role left unchanged")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := writeFileAtomic(path, []byte(output+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintln(out, "Refined", path, "via", ag.name)
	return nil
}
