package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeRefineAgent configures an agent that ignores its prompt and prints
// output verbatim.
func fakeRefineAgent(t *testing.T, st Store, output string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	outFile := filepath.Join(st.Root, "agent-out.txt")
	os.WriteFile(outFile, []byte(output), 0o644)
	fake := filepath.Join(st.Root, "fakeagent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\ncat > /dev/null\ncat '"+outFile+"'\n"), 0o755)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"), []byte("agent_cmd: sh "+fake+"\n"), 0o644)
}

func runRefine(id, input string, force bool) (string, error) {
	var out bytes.Buffer
	err := refine(strings.NewReader(input), &out, io.Discard, id, force)
	return out.String(), err
}

const refinedBody = "# pick a db\n\n## Notes\n\nPostgres or SQLite for the cache?\n"

func TestRefineIdeaRecordsSuggestion(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "pick a db", nil)
	fakeRefineAgent(t, st, refinedBody+"\nSPEC-TYPE-SUGGESTION: adr: choosing between two databases\n")
	setTerminal(t, false)
	out, err := runRefine("IDEA-001", "", false)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	s := st.Get("IDEA-001")
	if metaGet(s.Meta, "suggested_type", "") != "adr" || metaGet(s.Meta, "suggested_reason", "") != "choosing between two databases" {
		t.Fatalf("suggestion not recorded: %v", s.Meta)
	}
	if s.Status() != "refined" {
		t.Fatalf("new idea should become refined, got %s", s.Status())
	}
	if strings.Contains(s.Body, "SPEC-TYPE") || !strings.Contains(s.Body, "Postgres or SQLite") {
		t.Fatalf("bad saved body:\n%s", s.Body)
	}
	if !strings.Contains(out, "Suggested: adr — choosing between two databases") || !strings.Contains(out, "To promote: spec promote IDEA-001 adr") {
		t.Fatalf("non-terminal output should print the promote command:\n%s", out)
	}
	if strings.Contains(out, "[y/N]") {
		t.Fatal("must not prompt when not on a terminal")
	}
	// refining again keeps "refined"
	if _, err := runRefine("IDEA-001", "", false); err != nil || st.Get("IDEA-001").Status() != "refined" {
		t.Fatalf("refined idea should stay refined: %v", err)
	}
}

func TestRefineIdeaMissingOrInvalidSuggestion(t *testing.T) {
	for name, tail := range map[string]string{
		"missing": "",
		"invalid": "\nSPEC-TYPE-SUGGESTION: bug: not a type\n",
		"idea":    "\nSPEC-TYPE-SUGGESTION: idea: still an idea\n",
	} {
		t.Run(name, func(t *testing.T) {
			st := setupRepo(t)
			newIdea(t, st, "pick a db", map[string][]string{"suggested_type": {"feature"}, "suggested_reason": {"earlier"}})
			fakeRefineAgent(t, st, refinedBody+tail)
			setTerminal(t, false)
			out, err := runRefine("IDEA-001", "", false)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "No type suggestion from agent (kept earlier suggestion: feature)") {
				t.Fatalf("output:\n%s", out)
			}
			s := st.Get("IDEA-001")
			if metaGet(s.Meta, "suggested_type", "") != "feature" || metaGet(s.Meta, "suggested_reason", "") != "earlier" {
				t.Fatalf("earlier suggestion lost: %v", s.Meta)
			}
			if !strings.Contains(s.Body, "Postgres or SQLite") || strings.Contains(s.Body, "SPEC-TYPE") {
				t.Fatalf("body not saved cleanly:\n%s", s.Body)
			}
		})
	}
}

func TestRefineDiscardedIdeaNeedsForce(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "dead", map[string][]string{"status": {"discarded"}})
	fakeRefineAgent(t, st, "# dead\n\n## Notes\n\nn\n\nSPEC-TYPE-SUGGESTION: feature: f\n")
	setTerminal(t, false)
	if _, err := runRefine("IDEA-001", "", false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
	if _, err := runRefine("IDEA-001", "", true); err != nil {
		t.Fatal(err)
	}
	if s := st.Get("IDEA-001"); metaGet(s.Meta, "suggested_type", "") != "feature" {
		t.Fatal("--force refine didn't record the suggestion")
	}
}

func TestRefineIdeaPromoteNow(t *testing.T) {
	st := setupRepo(t)
	newIdea(t, st, "pick a db", nil)
	newIdea(t, st, "second", nil)
	fakeRefineAgent(t, st, refinedBody+"\nSPEC-TYPE-SUGGESTION: adr: a choice\n")
	setTerminal(t, true)

	out, err := runRefine("IDEA-002", "n\n", false)
	if err != nil || !strings.Contains(out, "Promote now? [y/N]") || st.Get("IDEA-002") == nil || st.Get("ADR-001") != nil {
		t.Fatalf("answering n should not promote: %v\n%s", err, out)
	}
	out, err = runRefine("IDEA-001", "y\n", false)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if st.Get("IDEA-001") != nil || st.Get("ADR-001") == nil || !strings.Contains(out, "Promoted IDEA-001 → ADR-001") {
		t.Fatalf("answering y should promote:\n%s", out)
	}
	if !strings.Contains(st.Get("ADR-001").Body, "Postgres or SQLite") {
		t.Fatal("refined notes not carried into the ADR")
	}
}

func TestRefineIdeaHeuristicWithoutAgent(t *testing.T) {
	st := setupRepo(t)
	notes := "# cache store\n\n## Notes\n\nShould we use Postgres vs SQLite? Need to decide.\n"
	idea := newIdea(t, st, "cache store", nil)
	os.WriteFile(idea.Path, []byte(dumpFrontmatter(idea.Meta, notes)), 0o644)
	setTerminal(t, false)
	out, err := runRefine("IDEA-001", "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := st.Get("IDEA-001")
	if metaGet(s.Meta, "suggested_type", "") != "adr" || !strings.HasPrefix(metaGet(s.Meta, "suggested_reason", ""), "heuristic: ") {
		t.Fatalf("heuristic suggestion not recorded: %v\n%s", s.Meta, out)
	}
	if strings.TrimSpace(s.Body) != strings.TrimSpace(notes) || s.Status() != "new" {
		t.Fatalf("heuristic must not rewrite the body or status: %q %s", s.Body, s.Status())
	}
	// no signal → message, nothing recorded
	newIdea(t, st, "vague", nil)
	out, err = runRefine("IDEA-002", "", false)
	if err != nil || !strings.Contains(out, "no clear type signal") || metaGet(st.Get("IDEA-002").Meta, "suggested_type", "") != "" {
		t.Fatalf("no-signal case: %v\n%s", err, out)
	}
}

func TestSuggestTypeHeuristic(t *testing.T) {
	cases := map[string]string{
		"Postgres vs SQLite — which should we choose?":    "adr",
		"Use X instead of Y; record the alternatives":     "adr",
		"Users can export reports.\n- [ ] CSV\n- [ ] PDF": "feature",
		"The API must return 404 and should log it":       "feature",
		"# vague\n\n## Notes\n\nTODO: what's the idea?\n": "",
		"something about search":                          "",
	}
	for in, want := range cases {
		if got, reason := suggestTypeHeuristic(in); got != want || (want != "" && !strings.HasPrefix(reason, "heuristic: ")) {
			t.Fatalf("%q: got %q (%s), want %q", in, got, reason, want)
		}
	}
}

func TestRefineTypedSpecUnchanged(t *testing.T) {
	st := setupRepo(t)
	featureWithStatus(t, st, "plain", "draft")
	fakeRefineAgent(t, st, "# plain\n\n## Summary\n\ns\n\n## Design\n\nd\n\n## Acceptance Criteria\n\n- [ ] x\n")
	setTerminal(t, true)
	out, err := runRefine("FEAT-001", "y\n", false)
	if err != nil {
		t.Fatal(err)
	}
	s := st.Get("FEAT-001")
	if s.Status() != "draft" || metaGet(s.Meta, "suggested_type", "") != "" || strings.Contains(out, "Promote") {
		t.Fatalf("typed-spec refine changed behavior: %v\n%s", s.Meta, out)
	}
}
