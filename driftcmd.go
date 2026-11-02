package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const noGitMsg = "Drift detection needs git (this isn't a git repository)."

// splitFlags separates --flags from positional args, rejecting unknown ones.
func splitFlags(args []string, known ...string) (pos []string, flags map[string]bool, err error) {
	flags = map[string]bool{}
	for _, a := range args {
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		if !containsStr(known, a) {
			return nil, nil, fmt.Errorf("unknown flag %s", a)
		}
		flags[a] = true
	}
	return pos, flags, nil
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func printDrift(out io.Writer, d *driftInfo) {
	fmt.Fprintf(out, "%-9s may be stale — %s changed since it was last updated\n", d.Spec.ID(), strings.Join(d.Files, ", "))
	for _, c := range d.Commits {
		fmt.Fprintf(out, "            %s %s %s\n", short(c.SHA), c.Date, c.Subject)
	}
	if len(d.Uncommitted) > 0 {
		fmt.Fprintf(out, "            (uncommitted changes: %s)\n", strings.Join(d.Uncommitted, ", "))
	}
}

func cmdDrift(in io.Reader, out, errOut io.Writer, args []string) error {
	ids, flags, err := splitFlags(args, "--review", "--yes")
	if err != nil {
		return err
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	g := loadGit(st.Root)
	if g == nil {
		fmt.Fprintln(out, noGitMsg)
		return nil
	}
	for _, id := range ids {
		if st.Get(id) == nil {
			return fmt.Errorf("no spec with id %s", id)
		}
	}
	wanted := func(id string) bool { return len(ids) == 0 || containsStr(ids, id) }
	all, uncovered := g.systemDrift(st, cfg)
	var stale []*driftInfo
	for _, d := range all {
		if wanted(d.Spec.ID()) {
			stale = append(stale, d)
		}
	}
	if flags["--review"] {
		return reviewDrift(in, out, errOut, st, cfg, g, stale, flags["--yes"])
	}
	for _, d := range stale {
		printDrift(out, d)
	}
	var unc []string
	for _, id := range uncovered {
		if wanted(id) {
			unc = append(unc, id)
		}
	}
	if len(unc) > 0 {
		fmt.Fprintf(out, "No coverage, can't check (run spec covers --infer): %s\n", strings.Join(unc, ", "))
	}
	if len(stale) == 0 {
		fmt.Fprintln(out, "No drift detected.")
		return nil
	}
	fmt.Fprintf(out, "\n%d spec(s) may be stale. Review: spec drift --review [id...] · mark accurate: spec verified <id>\n", len(stale))
	return nil
}

const driftReviewPromptFmt = `You are checking whether a spec still describes the system accurately.

Spec %s (%s):

%s

Files this spec covers changed since the spec was last updated. Diff since then:

%s

Decide whether the spec is still accurate. Only behavior the spec actually
describes matters — unrelated changes to the same files don't make it stale.

Reply in exactly this format:
VERDICT: accurate|stale
REASON: <one sentence>

If stale, follow with the complete updated spec body (markdown, no
frontmatter). Keep every "## " section and change only what is now untrue.`

var (
	verdictPattern = regexp.MustCompile(`(?im)^\s*VERDICT:\s*(accurate|stale)\b`)
	reasonPattern  = regexp.MustCompile(`(?im)^\s*REASON:\s*(.+)$`)
)

// gitDiff returns the diff of files from commit to the working tree,
// truncated so a big change can't flood the prompt.
func gitDiff(root, commit string, files []string) string {
	out, _ := runGit(root, append([]string{"diff", commit, "--"}, files...)...)
	const max = 30000
	if len(out) > max {
		return string(out[:max]) + "\n… (diff truncated)"
	}
	return string(out)
}

// textDiff shows a unified diff between two texts via git diff --no-index.
func textDiff(a, b string) string {
	dir, err := os.MkdirTemp("", "spec-drift-")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir)
	pa, pb := filepath.Join(dir, "current"), filepath.Join(dir, "proposed")
	os.WriteFile(pa, []byte(a), 0o644)
	os.WriteFile(pb, []byte(b), 0o644)
	out, _ := runGit(dir, "diff", "--no-index", "--no-color", "current", "proposed")
	return string(out)
}

func reviewDrift(in io.Reader, out, errOut io.Writer, st Store, cfg Config, g *gitRepo, stale []*driftInfo, yes bool) error {
	if len(stale) == 0 {
		fmt.Fprintln(out, "No drift detected — nothing to review.")
		return nil
	}
	ag, err := resolveAgent(cfg)
	if err != nil {
		return err
	}
	if ag == nil {
		return fmt.Errorf("spec drift --review needs an LLM agent (set agent/agent_cmd in .spec/config.yml)")
	}
	reader := bufio.NewReader(in)
	for _, d := range stale {
		s := d.Spec
		prompt := fmt.Sprintf(driftReviewPromptFmt, s.ID(), s.Title(), s.Body, gitDiff(st.Root, d.Checkpoint, d.Files))
		fmt.Fprintf(out, "Reviewing %s against %d change(s) via %s…\n", s.ID(), len(d.Commits)+len(d.Uncommitted), ag.name)
		output, err := ag.run(prompt, st.Root, cfg.AgentTimeout)
		if err != nil {
			return err
		}
		m := verdictPattern.FindStringSubmatch(output)
		reason := ""
		if r := reasonPattern.FindStringSubmatch(output); r != nil {
			reason = strings.TrimSpace(r[1])
		}
		if m == nil {
			dumpAgentOutput(errOut, output)
			fmt.Fprintf(out, "%s: agent gave no VERDICT — left unchanged\n", s.ID())
			continue
		}
		if strings.EqualFold(m[1], "accurate") {
			if err := setSpecMeta(&s, map[string][]string{"verified": {g.head}}); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s: ACCURATE — %s\n  stamped verified: %s\n", s.ID(), reason, short(g.head))
			continue
		}
		fmt.Fprintf(out, "%s: STALE — %s\n", s.ID(), reason)
		rest := output
		if loc := reasonPattern.FindStringIndex(output); loc != nil {
			rest = output[loc[1]:]
		}
		body := bodyFromAgentOutput(rest)
		if !strings.Contains(body, "# ") {
			fmt.Fprintf(out, "  agent proposed no updated body — update %s by hand\n", s.ID())
			continue
		}
		fmt.Fprint(out, textDiff(s.Body, body))
		apply := yes
		if !yes {
			if !stdinIsTerminal() {
				fmt.Fprintf(out, "  not applied (run interactively, or pass --yes)\n")
				continue
			}
			fmt.Fprintf(out, "Apply this update to %s? [y/N] ", s.ID())
			answer, _ := reader.ReadString('\n')
			a := strings.ToLower(strings.TrimSpace(answer))
			apply = a == "y" || a == "yes"
		}
		if !apply {
			fmt.Fprintf(out, "  not applied\n")
			continue
		}
		s.Body = body
		if err := setSpecMeta(&s, map[string][]string{"verified": {g.head}}); err != nil {
			return err
		}
		fmt.Fprintf(out, "  updated %s\n", relPath(st, s.Path))
	}
	return nil
}

// trackCoverage runs after an apply: it merges the code files the agent
// changed into the applied spec's covers (when it reached implemented), and
// warns about other system specs covering those files whose spec files the
// run didn't update.
func trackCoverage(out io.Writer, st Store, cfg Config, id string, changed []string, implemented bool) {
	if len(changed) == 0 {
		return
	}
	changedSet := map[string]bool{}
	var code []string
	for _, f := range changed {
		changedSet[f] = true
		if isCodeFile(f) {
			code = append(code, f)
		}
	}
	if len(code) == 0 {
		return
	}
	if s := st.Get(id); s != nil && implemented {
		covers := s.Field("covers")
		added := false
		for _, f := range code {
			if !coveredBy(covers, f) {
				covers = append(covers, f)
				added = true
			}
		}
		if added {
			if err := setSpecMeta(s, map[string][]string{"covers": covers}); err == nil {
				fmt.Fprintf(out, "%s now covers: %s\n", id, strings.Join(covers, ", "))
			}
		}
	}
	var others []string
	var otherIDs []string
	for _, o := range st.Specs() {
		if o.ID() == id || !inSystem(cfg, o) {
			continue
		}
		rel, _ := filepath.Rel(st.Root, o.Path)
		if changedSet[filepath.ToSlash(rel)] {
			continue // updated in this run
		}
		var hit []string
		for _, f := range code {
			if coveredBy(o.Field("covers"), f) {
				hit = append(hit, f)
			}
		}
		if len(hit) > 0 {
			others = append(others, fmt.Sprintf("%s (%s)", o.ID(), strings.Join(hit, ", ")))
			otherIDs = append(otherIDs, o.ID())
		}
	}
	if len(others) > 0 {
		fmt.Fprintf(out, "\nOther specs cover files this run changed but weren't updated — check they're still accurate:\n  %s\n  spec drift --review %s\n",
			strings.Join(others, "\n  "), strings.Join(otherIDs, " "))
	}
}

func cmdCovers(out io.Writer, args []string) error {
	ids, flags, err := splitFlags(args, "--infer")
	if err != nil {
		return err
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	var targets []Spec
	if len(ids) == 0 {
		for _, s := range st.Specs() {
			if inSystem(cfg, s) {
				targets = append(targets, s)
			}
		}
	} else {
		for _, id := range ids {
			s := st.Get(id)
			if s == nil {
				return fmt.Errorf("no spec with id %s", id)
			}
			targets = append(targets, *s)
		}
	}
	if !flags["--infer"] {
		for _, s := range targets {
			covers := strings.Join(s.Field("covers"), ", ")
			if covers == "" {
				covers = "(none)"
			}
			fmt.Fprintf(out, "%-9s %s\n", s.ID(), covers)
		}
		return nil
	}
	g := loadGit(st.Root)
	if g == nil {
		return fmt.Errorf("spec covers --infer needs git (this isn't a git repository)")
	}
	for _, s := range targets {
		if len(s.Field("covers")) > 0 {
			if len(ids) > 0 {
				fmt.Fprintf(out, "%-9s already has covers — edit the spec to change it\n", s.ID())
			}
			continue
		}
		files := g.inferCovers(s)
		if len(files) == 0 {
			fmt.Fprintf(out, "%-9s no code found in the commits that touched it — add covers: by hand\n", s.ID())
			continue
		}
		s := s
		if err := setSpecMeta(&s, map[string][]string{"covers": files}); err != nil {
			return err
		}
		fmt.Fprintf(out, "%-9s covers %s\n", s.ID(), strings.Join(files, ", "))
	}
	return nil
}

func cmdVerified(out io.Writer, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("usage: spec verified <id>...")
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	g := loadGit(st.Root)
	if g == nil || g.head == "" {
		return fmt.Errorf("spec verified needs a git repository with at least one commit")
	}
	for _, id := range ids {
		s := st.Get(id)
		if s == nil {
			return fmt.Errorf("no spec with id %s", id)
		}
		if err := setSpecMeta(s, map[string][]string{"verified": {g.head}}); err != nil {
			return err
		}
		fmt.Fprintf(out, "Verified %s at %s\n", id, short(g.head))
	}
	return nil
}
