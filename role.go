package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// rolePointerMarker delimits the AGENTS.md block that points agents at the
// repo's role. Referencing the role sources (not the text) keeps the block
// valid when the role changes.
const rolePointerMarker = "<!-- spec:role -->"

const rolePointerBlock = rolePointerMarker + `
## Development role

This repo defines a development persona for agents: ` + "`.spec/role.md`" + `, or a
shared persona selected by ` + "`persona:`" + ` in ` + "`.spec/config.yml`" + ` (library:
` + "`~/.config/spec/personas/`" + `). When one is set, adopt that persona and its
conventions for all work here.
<!-- /spec:role -->
`

// ensureRolePointer appends the marker-delimited role block to AGENTS.md
// when absent (creating a minimal AGENTS.md when missing), so agents run
// directly in the repo adopt the same role spec injects into prompts.
// Idempotent; callers invoke it after a role becomes available.
func ensureRolePointer(root string) error {
	path := filepath.Join(root, "AGENTS.md")
	data, _ := os.ReadFile(path)
	s := string(data)
	if strings.Contains(s, rolePointerMarker) {
		return nil
	}
	s = strings.TrimRight(s, "\n")
	if s != "" {
		s += "\n\n"
	}
	return writeFileAtomic(path, []byte(s+rolePointerBlock), 0o644)
}

// loadRole deliberately reads only repository context, never global config.
func loadRole(root string) string {
	data, err := os.ReadFile(filepath.Join(root, configDir, "role.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func cmdRole(args []string, out, errOut io.Writer) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "edit" && args[0] != "refine" && args[0] != "adopt") {
		return fmt.Errorf("usage: spec role [edit|refine|adopt]")
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	role, source := roleFor(st, cfg)
	if len(args) == 0 {
		if role == "" {
			fmt.Fprintln(errOut, "No role set; use spec role edit, spec role refine, or spec persona use <name>.")
		} else {
			fmt.Fprintln(out, role)
			if source != filepath.Join(configDir, "role.md") {
				fmt.Fprintf(errOut, "(from %s)\n", source)
			}
		}
		return nil
	}
	path := filepath.Join(st.Root, configDir, "role.md")
	if args[0] == "adopt" {
		if loadRole(st.Root) != "" {
			return fmt.Errorf(".spec/role.md already set — edit it directly")
		}
		persona := loadPersona(cfg.Persona)
		if persona == "" {
			return fmt.Errorf("no persona in use (set one with spec persona use <name>)")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := writeFileAtomic(path, []byte(persona+"\n"), 0o644); err != nil {
			return err
		}
		if err := ensureRolePointer(st.Root); err != nil {
			return err
		}
		fmt.Fprintf(out, "Adopted persona %q → %s\n", cfg.Persona, path)
		return nil
	}
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
		if err := cmd.Run(); err != nil {
			return err
		}
		// the user may have just written a role — make sure AGENTS.md knows
		return ensureRolePointer(st.Root)
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
	if err := ensureRolePointer(st.Root); err != nil {
		return err
	}
	fmt.Fprintln(out, "Refined", path, "via", ag.name)
	return nil
}
