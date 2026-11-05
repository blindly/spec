package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// personaNamePattern keeps persona names to safe file names — a persona is
// resolved to <personaDir>/<name>.md, so no separators or traversal.
var personaNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// personaDir is the user-level persona library: <global config dir>/personas/.
func personaDir() string {
	p := globalConfigPath()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), "personas")
}

func personaPath(name string) string {
	return filepath.Join(personaDir(), name+".md")
}

// loadPersona returns a persona's content ("" when unset or invalid name).
func loadPersona(name string) string {
	if !personaNamePattern.MatchString(name) {
		return ""
	}
	data, err := os.ReadFile(personaPath(name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// roleFor resolves the effective development role: the project role file
// when set, else the persona named by cfg.Persona. Returns the content and
// a source label for display; ("", "") means no role.
func roleFor(st Store, cfg Config) (string, string) {
	if role := loadRole(st.Root); role != "" {
		return role, roleFileRel
	}
	if cfg.Persona != "" && cfg.Persona != "none" {
		if p := loadPersona(cfg.Persona); p != "" {
			return p, fmt.Sprintf("persona %q (%s)", cfg.Persona, personaPath(cfg.Persona))
		}
	}
	return "", ""
}

func cmdPersona(args []string, out, errOut io.Writer) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	dir := personaDir()
	switch sub {
	case "":
		entries, _ := os.ReadDir(dir)
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				names = append(names, strings.TrimSuffix(e.Name(), ".md"))
			}
		}
		if len(names) == 0 {
			fmt.Fprintln(errOut, "No personas; create one with spec persona new <name>.")
			return nil
		}
		sort.Strings(names)
		for _, n := range names {
			mark := "  "
			if n == cfg.Persona {
				mark = "* "
			}
			fmt.Fprintf(out, "%s%s\n", mark, n)
		}
		return nil
	case "use":
		if len(args) != 2 {
			return fmt.Errorf("usage: spec persona use <name|none>")
		}
		if args[1] == "none" {
			if err := setConfigKey(st.Root, "persona", ""); err != nil {
				return err
			}
			fmt.Fprintln(out, "Persona cleared")
			return nil
		}
		if loadPersona(args[1]) == "" {
			return fmt.Errorf("no persona %q (create it with spec persona new %s)", args[1], args[1])
		}
		if err := setConfigKey(st.Root, "persona", args[1]); err != nil {
			return err
		}
		if err := ensureRolePointer(st.Root); err != nil {
			return err
		}
		fmt.Fprintf(out, "Using persona %q\n", args[1])
		if loadRole(st.Root) != "" {
			fmt.Fprintln(errOut, "note: .spec/role.md is set and takes precedence")
		}
		return nil
	case "new", "edit":
		if len(args) != 2 || !personaNamePattern.MatchString(args[1]) {
			return fmt.Errorf("usage: spec persona %s <name>", sub)
		}
		if dir == "" {
			return fmt.Errorf("cannot determine home directory for the persona library")
		}
		path := personaPath(args[1])
		if sub == "new" {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("persona %q already exists", args[1])
			}
		} else if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("no persona %q (create it with spec persona new %s)", args[1], args[1])
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
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
	case "refine":
		if len(args) != 2 || !personaNamePattern.MatchString(args[1]) {
			return fmt.Errorf("usage: spec persona refine <name>")
		}
		ag, err := resolveAgent(cfg)
		if err != nil {
			return err
		}
		if ag == nil {
			return fmt.Errorf("persona refine requires an LLM agent (none configured; set agent/agent_cmd in .spec/config.yml)")
		}
		// deliberately repo-blind: no repoContext, so project specifics
		// can't leak into a persona shared by many repos
		prompt := fmt.Sprintf(`You are improving a reusable development persona file named %q, shared across spec-managed repositories. The file tells the agent who it is and how it develops.

Improve the persona below, or draft one when it is empty. It must stay repository-agnostic — no project names, paths, or repo-specific conventions. Describe the persona and concrete development conventions the agent should adopt.

Your reply is written verbatim to the file, so output ONLY the file contents: a markdown document that starts with a single "# " heading followed by a one-sentence persona statement ("You are a ..."). No commentary, no narration, no frontmatter, no surrounding code fences.

Current persona:
%s`, args[1], loadPersona(args[1]))
		fmt.Fprintf(out, "Refining persona %q via %s…\n", args[1], ag.name)
		output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if err != nil {
			return err
		}
		body := strings.TrimSpace(bodyFromAgentOutput(output))
		if !strings.HasPrefix(body, "# ") {
			dumpAgentOutput(errOut, output)
			return fmt.Errorf("agent returned no markdown document — persona left unchanged")
		}
		path := personaPath(args[1])
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := writeFileAtomic(path, []byte(body+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Fprintln(out, "Refined", path, "via", ag.name)
		return nil
	}
	return fmt.Errorf("usage: spec persona [new|edit|refine <name>|use <name|none>]")
}
