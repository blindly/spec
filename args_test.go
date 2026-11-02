package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSplitArgs(t *testing.T) {
	cases := map[string][]string{
		"mycli --model 'gpt 5' {prompt}": {"mycli", "--model", "gpt 5", "{prompt}"},
		`mycli "say \"hi\"" x`:           {"mycli", `say "hi"`, "x"},
		`mycli "a\\b"`:                   {"mycli", `a\b`},
		`mycli a\ b`:                     {"mycli", "a b"},
		"go test ./...":                  {"go", "test", "./..."},
		"  spaced   out  ":               {"spaced", "out"},
		"x '' y":                         {"x", "", "y"},
		"pre'mid dle'post":               {"premid dlepost"},
		"":                               nil,
	}
	for in, want := range cases {
		got, err := splitArgs(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: got %q want %q", in, got, want)
		}
	}
	for _, bad := range []string{"mycli 'open", `mycli "open`} {
		if _, err := splitArgs(bad); err == nil {
			t.Fatalf("%q: expected unterminated-quote error", bad)
		}
	}
}

func TestUnquoteKeepsInnerQuotes(t *testing.T) {
	if got := unquote(`"mycli --model 'gpt 5' {prompt}"`); got != "mycli --model 'gpt 5' {prompt}" {
		t.Fatalf("got %q", got)
	}
	if got := unquote("mycli --model 'gpt 5' {prompt}"); got != "mycli --model 'gpt 5' {prompt}" {
		t.Fatalf("unquoted value changed: %q", got)
	}
}

func TestAgentCmdQuotedArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	st := setupRepo(t)
	// fake agent prints its 1st arg; must arrive as one argument with a space
	fake := filepath.Join(st.Root, "fakeagent.sh")
	os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s' \"$1\"\n"), 0o755)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent_cmd: sh "+fake+" 'gpt 5' {prompt}\n"), 0o644)
	ag, err := resolveAgent(loadConfig(st.Root))
	if err != nil || ag == nil {
		t.Fatalf("resolve: %v", err)
	}
	out, err := ag.run("hello", st.Root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if out != "gpt 5" {
		t.Fatalf("quoted arg split: %q", out)
	}
	// unterminated quote → error, nothing runs
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("agent_cmd: sh "+fake+" 'gpt {prompt}\n"), 0o644)
	if _, err := resolveAgent(loadConfig(st.Root)); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("expected unterminated-quote error, got %v", err)
	}
}

func TestVerifyQuotedArgsAndBadQuote(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh")
	}
	st := setupRepo(t)
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("verify: [sh -c 'test \"$0\" = \"a b\"' 'a b']\n"), 0o644)
	if err := cmdVerify(io.Discard, io.Discard); err != nil {
		t.Fatalf("quoted verify arg failed: %v", err)
	}
	marker := filepath.Join(st.Root, "ran")
	os.WriteFile(filepath.Join(st.Root, configDir, "config.yml"),
		[]byte("verify: [touch "+marker+", sh -c 'oops]\n"), 0o644)
	var out bytes.Buffer
	if err := cmdVerify(&out, io.Discard); err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("expected quoting error, got %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a verify command ran despite a quoting error")
	}
}
