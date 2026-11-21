package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestSecretFactCommands(t *testing.T) {
	st := setupRepo(t)
	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if err := cmdFact(&out, io.Discard, args); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	writeFile(t, st, secretsFile, "# keep this comment\nother: unchanged\n")
	run("set", "k", "old-public-value")
	value := "  credential # with punctuation and a newline\n"
	if got := run("set", "--secret", "k", value); !strings.Contains(got, "<secret>") || strings.Contains(got, value) {
		t.Fatal(got)
	}
	info, err := os.Stat(secretsPath(st.Root))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("permission = %o", info.Mode().Perm())
	}
	if got := run("get", "k"); got != value+"\n" {
		t.Fatalf("get returned %q", got)
	}
	data, err := os.ReadFile(factsPath(st.Root))
	if err != nil || strings.Contains(string(data), "k:") {
		t.Fatalf("promoted key remained public: %q, %v", data, err)
	}
	run("set", "k", "updated-credential")
	if got := run("get", "k"); got != "updated-credential\n" {
		t.Fatal(got)
	}
	writeFile(t, st, ".spec/facts.yml", "k: shadowed-value\ndomain: example.test\n")
	if got := run("list"); strings.Count(got, "<secret>") != 2 ||
		strings.Contains(got, "updated-credential") || strings.Contains(got, "shadowed-value") ||
		!strings.Contains(got, "example.test") {
		t.Fatal(got)
	}
	run("unset", "k")
	if err := cmdFact(io.Discard, io.Discard, []string{"get", "k"}); err == nil {
		t.Fatal("unset left a definition")
	}
	data, err = os.ReadFile(secretsPath(st.Root))
	if err != nil || string(data) != "# keep this comment\nother: unchanged\n" {
		t.Fatalf("secrets file = %q, %v", data, err)
	}
}

func TestCheckSecretsGitignore(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st)
	if err := cmdFact(io.Discard, io.Discard, []string{"set", "--secret", "k", "credential"}); err != nil {
		t.Fatal(err)
	}
	check := func(wantFailure bool) {
		t.Helper()
		var out bytes.Buffer
		err := cmdCheck(&out, io.Discard)
		if (err != nil) != wantFailure || strings.Contains(out.String(), "must be untracked and ignored") != wantFailure {
			t.Fatalf("check: %v\n%s", err, out.String())
		}
	}
	check(false)
	writeFile(t, st, ".gitignore", "/.spec/secrets.yml/\n")
	check(true)
	if err := cmdInit(io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	check(false)
	if out, err := exec.Command("git", "add", "-f", "--", secretsFile).CombinedOutput(); err != nil {
		t.Fatalf("force add: %v\n%s", err, out)
	}
	check(true)
	if err := os.Remove(secretsPath(st.Root)); err != nil {
		t.Fatal(err)
	}
	check(true)
}

func TestSecretReferencesAndPrompt(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st)
	writeFile(t, st, secretsFile, "credential: local-credential-value\n")
	writeFile(t, st, ".spec/facts.yml", "domain: example.test\ncredential: shadowed-value\n")
	spec, err := st.Create("feature", "secret reference", "# Feature\n\n{secret:credential} {fact:domain}\n", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(spec.Body, "{secret:credential} example.test") {
		t.Fatal(spec.Body)
	}
	var out bytes.Buffer
	if err := cmdCheck(&out, io.Discard); err != nil || !strings.Contains(out.String(), "defined in both tiers") {
		t.Fatalf("check: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "secret fact \"credential\" is never referenced") {
		t.Fatal(out.String())
	}
	prompt := repoContext(st, 50)
	for _, want := range []string{"Secret facts (values withheld", "spec fact get <key>", "- credential\n", "- domain: example.test"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
	for _, leaked := range []string{"local-credential-value", "shadowed-value"} {
		if strings.Contains(prompt, leaked) {
			t.Fatal("prompt exposed a secret or shadowed value")
		}
	}
	bad, err := st.Create("feature", "bad references", "# Feature\n\n{fact:credential} {secret:missing} {secret:domain}\n", nil)
	if err != nil || !strings.Contains(bad.Body, "{fact:credential}") {
		t.Fatalf("bad reference was expanded: %q, %v", bad.Body, err)
	}
	out.Reset()
	if err := cmdCheck(&out, io.Discard); err == nil ||
		!strings.Contains(out.String(), "use {secret:credential}") ||
		!strings.Contains(out.String(), "undefined secret fact reference FEAT-002: {secret:missing}") ||
		!strings.Contains(out.String(), "undefined secret fact reference FEAT-002: {secret:domain}") {
		t.Fatalf("check: %v\n%s", err, out.String())
	}
}

func TestCheckSecretLiteralWarnings(t *testing.T) {
	st := setupRepo(t)
	gitInit(t, st)
	token := "ghp_" + strings.Repeat("A", 36)
	accessKey := "AKIA" + strings.Repeat("B", 16)
	apiKey := "sk-" + strings.Repeat("c", 20)
	entropy := "mN7rF2bK1Jq8xU0zWYvA6DTs"
	custom := "custom credential phrase"
	writeFile(t, st, secretsFile, token+": "+custom+"\n")
	writeFile(t, st, ".spec/facts.yml", "token: "+token+"\nrandom: "+entropy+"\n")
	for _, body := range []string{accessKey, custom, "{secret:" + token + "}"} {
		if _, err := st.Create("feature", body[:4], "# Feature\n\n"+body+"\n", nil); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, st, ".spec/skills/demo/SKILL.md", "token "+apiKey)
	writeFile(t, st, ".spec/skills/demo/script.sh", custom)
	writeFile(t, st, ".spec/skills/demo/example.md", "{secret:"+token+"}")
	if err := cmdSkill(io.Discard, io.Discard, []string{"sync"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := cmdCheck(&out, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`fact "token" contains`, `fact "random" contains`,
		"FEAT-001 contains", "FEAT-002 contains", "SKILL.md contains", "script.sh contains"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q from check:\n%s", want, out.String())
		}
	}
	for _, unexpected := range []string{"FEAT-003 contains", "example.md contains", accessKey, apiKey, entropy, custom} {
		if strings.Contains(out.String(), unexpected) {
			t.Fatalf("check emitted %q", unexpected)
		}
	}
}

func TestAgentEnvironmentGrants(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	st := setupRepo(t)
	t.Setenv("FEAT027_ALLOWED", "from-parent")
	t.Setenv("FEAT027_HIDDEN", "hidden-from-agent")
	t.Setenv("FEAT027_INJECTED", "parent-value")
	writeFile(t, st, secretsFile, "k: from-secret-file\n")
	writeFile(t, st, ".spec/facts.yml", "plain: public-only\n")
	writeFile(t, st, "probe.sh", "echo started > started\nenv\n")
	run := func(config string) (string, error) {
		t.Helper()
		writeFile(t, st, ".spec/config.yml", "agent_cmd: sh probe.sh\n"+config)
		ag, err := resolveAgent(loadConfig(st.Root))
		if err != nil {
			t.Fatal(err)
		}
		return ag.run("prompt", st.Root, 5)
	}
	output, err := run("agent_env: [FEAT027_ALLOWED, FEAT027_INJECTED=k, FEAT027_MISSING]\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"FEAT027_ALLOWED=from-parent\n", "FEAT027_INJECTED=from-secret-file\n", "PATH="} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing %q from subprocess environment", want)
		}
	}
	for _, unexpected := range []string{"FEAT027_HIDDEN=", "FEAT027_MISSING=", "parent-value"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("subprocess received unexpected %q", unexpected)
		}
	}
	for _, config := range []string{"", "agent_env: []\n"} {
		output, err := run(config)
		if err != nil || !strings.Contains(output, "FEAT027_HIDDEN=hidden-from-agent\n") ||
			!strings.Contains(output, "FEAT027_INJECTED=parent-value\n") {
			t.Fatalf("subprocess did not inherit environment: %v", err)
		}
	}
	if err := os.Remove("started"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"missing", "plain"} {
		if _, err := run("agent_env: [FEAT027_INJECTED=" + key + "]\n"); err == nil ||
			!strings.Contains(err.Error(), "undefined secret fact") || !strings.Contains(err.Error(), key) {
			t.Fatalf("unexpected grant error: %v", err)
		}
		if _, err := os.Stat("started"); !os.IsNotExist(err) {
			t.Fatalf("agent started despite an invalid grant: %v", err)
		}
	}
}

func TestDiscoverSecretCandidates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake agent")
	}
	st := setupRepo(t)
	token := "sk-" + strings.Repeat("e", 24)
	writeFile(t, st, ".spec/config.yml", "agent_cmd: sh discover.sh\n")
	writeFile(t, st, "discover.sh", "printf '%s\\n' 'token: "+token+" — manifest' 'domain: example.test — manifest'\n")
	if _, err := st.Create("feature", "secret proposal", "# Feature\n\n{secret:token}\n", nil); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := discoverFacts(st, strings.NewReader("n\n"), &out, io.Discard, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "token: <secret> — manifest") || strings.Contains(out.String(), token) {
		t.Fatal("secret proposal was missing or displayed its value")
	}
	if _, err := os.Stat(secretsPath(st.Root)); !os.IsNotExist(err) {
		t.Fatal("declining wrote secrets.yml")
	}
	if err := cmdFact(&out, io.Discard, []string{"discover", "--yes"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(secretsPath(st.Root))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets.yml was not written with mode 0600: %v %v", info, err)
	}
	if factsMap(loadSecretFacts(st.Root))["token"] != token {
		t.Fatal("discovered token missing from secrets.yml")
	}
	plain := factsMap(readFacts(factsPath(st.Root)))
	if _, ok := plain["token"]; ok || plain["domain"] != "example.test" {
		t.Fatalf("incorrect public entries: %v", plain)
	}
}
