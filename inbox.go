package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Ideas are untyped inbox entries (type idea, specs/inbox/, IDEA-xxx) with
// their own lifecycle: new → refined, or discarded. Promoting an idea turns
// it into a real spec and deletes the idea file.

const (
	ideaRefined   = "refined"
	ideaDiscarded = "discarded"
	ideaTodo      = "TODO: what's the idea?"
)

func getIdea(st Store, id string) (*Spec, error) {
	s := st.Get(id)
	if s == nil {
		return nil, fmt.Errorf("no spec with id %s", id)
	}
	if !s.IsIdea() {
		return nil, fmt.Errorf("%s is a %s, not an idea", id, s.Type())
	}
	return s, nil
}

// setIdeaMeta rewrites an idea's frontmatter keys (nil value deletes).
func setIdeaMeta(s *Spec, kv map[string][]string) error {
	meta := map[string][]string{}
	for k, v := range s.Meta {
		meta[k] = v
	}
	for k, v := range kv {
		if v == nil {
			delete(meta, k)
		} else {
			meta[k] = v
		}
	}
	if err := writeFileAtomic(s.Path, []byte(dumpFrontmatter(meta, s.Body)), 0o644); err != nil {
		return err
	}
	s.Meta = meta
	return nil
}

func discardIdea(st Store, id string) error {
	s, err := getIdea(st, id)
	if err != nil {
		return err
	}
	return setIdeaMeta(s, map[string][]string{"status": {ideaDiscarded}})
}

// ideaNotes extracts an idea's content for carrying into a promoted spec:
// the title heading, the "## Notes" heading, and the template TODO are
// dropped, and remaining headings are demoted one level so they nest under
// the target section instead of clashing with the template's sections.
func ideaNotes(body string) string {
	var out []string
	titleSeen := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case !titleSeen && strings.HasPrefix(t, "# "):
			titleSeen = true
			continue
		case strings.EqualFold(t, "## Notes"), t == ideaTodo:
			continue
		case strings.HasPrefix(t, "#"):
			line = "#" + line
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// fillFirstSection puts notes into a template's first "## " section,
// replacing its TODO placeholder line (or inserting after the heading).
func fillFirstSection(tmpl, notes string) string {
	lines := strings.Split(tmpl, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		for j := i + 1; j < len(lines) && !strings.HasPrefix(lines[j], "## "); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "TODO") {
				lines[j] = notes
				return strings.Join(lines, "\n")
			}
		}
		rest := append([]string{"", notes}, lines[i+1:]...)
		return strings.Join(append(lines[:i+1], rest...), "\n")
	}
	return tmpl + "\n" + notes + "\n"
}

// promoteIdea turns an idea into a spec of type typ (or its suggested_type
// when typ is empty), rewrites depends_on references, and deletes the idea.
func promoteIdea(st Store, id, typ string, force bool) (Spec, error) {
	s, err := getIdea(st, id)
	if err != nil {
		return Spec{}, err
	}
	if s.Status() == ideaDiscarded && !force {
		return Spec{}, fmt.Errorf("%s is discarded (use --force to promote anyway)", id)
	}
	if typ == "" {
		if typ = metaGet(s.Meta, "suggested_type", ""); typ == "" {
			return Spec{}, fmt.Errorf("%s has no suggested type — pass one: spec promote %s <project|feature|adr>", id, id)
		}
	}
	if !specTypes[typ] {
		return Spec{}, fmt.Errorf("can't promote to %q (want project|feature|adr)", typ)
	}
	body := templateFor(typ, s.Title())
	if notes := ideaNotes(s.Body); notes != "" {
		body = fillFirstSection(body, notes)
	}
	extra := map[string][]string{"promoted_from": {s.ID()}}
	if deps := s.Field("depends_on"); len(deps) > 0 {
		extra["depends_on"] = deps
	}
	ns, err := st.Create(typ, s.Title(), body, extra)
	if err != nil {
		return Spec{}, err
	}
	for _, o := range st.Specs() {
		if o.ID() == s.ID() || o.ID() == ns.ID() {
			continue
		}
		deps, changed := o.Field("depends_on"), false
		for i, d := range deps {
			if d == s.ID() {
				deps[i], changed = ns.ID(), true
			}
		}
		if changed {
			if err := writeFileAtomic(o.Path, []byte(dumpFrontmatter(o.Meta, o.Body)), 0o644); err != nil {
				return ns, fmt.Errorf("created %s, but updating depends_on in %s failed: %w", ns.ID(), o.ID(), err)
			}
		}
	}
	if err := os.Remove(s.Path); err != nil {
		return ns, fmt.Errorf("created %s, but deleting %s failed: %w", ns.ID(), s.ID(), err)
	}
	return ns, nil
}

func cmdDiscard(out io.Writer, ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("usage: spec discard <IDEA-id>...")
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := discardIdea(st, id); err != nil {
			return err
		}
		fmt.Fprintf(out, "Discarded %s\n", id)
	}
	return nil
}

func cmdPromote(out io.Writer, args []string) error {
	var pos []string
	force := false
	for _, a := range args {
		switch {
		case a == "--force":
			force = true
		case strings.HasPrefix(a, "--"):
			return fmt.Errorf("unknown flag %s", a)
		default:
			pos = append(pos, a)
		}
	}
	if len(pos) < 1 || len(pos) > 2 {
		return fmt.Errorf("usage: spec promote <IDEA-id> [project|feature|adr] [--force]")
	}
	typ := ""
	if len(pos) == 2 {
		typ = pos[1]
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	ns, err := promoteIdea(st, pos[0], typ, force)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Promoted %s → %s (%s)\nCreated %s — fill in its sections: spec edit %s\n",
		pos[0], ns.ID(), ns.Type(), ns.Path, ns.ID())
	return nil
}
