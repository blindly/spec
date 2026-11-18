package main

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var tokenPattern = regexp.MustCompile(`\b(?:ghp_[A-Za-z0-9]+|sk-[A-Za-z0-9_-]{8,}|AKIA[A-Z0-9]{16})|-----BEGIN [A-Z][A-Z ]*-----`)

func highEntropy(value string) bool {
	if len(value) < 20 || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	counts := map[rune]int{}
	total := 0
	for _, c := range value {
		counts[c]++
		total++
	}
	entropy := 0.0
	for _, n := range counts {
		p := float64(n) / float64(total)
		entropy -= p * math.Log2(p)
	}
	return entropy >= 4
}

func containsSecretLiteral(body string, secrets []Fact) bool {
	body = secretRefPattern.ReplaceAllString(body, " ")
	if tokenPattern.MatchString(body) {
		return true
	}
	for _, f := range secrets {
		if f.Value != "" && strings.Contains(body, f.Value) {
			return true
		}
	}
	return false
}

func checkFacts(st Store, out io.Writer) int {
	facts := readFacts(factsPath(st.Root))
	secretFacts := loadSecretFacts(st.Root)
	plain, secrets := factsMap(facts), factsMap(secretFacts)
	problems := 0
	tracked := exec.Command("git", "ls-files", "--error-unmatch", "--", secretsFile)
	tracked.Dir = st.Root
	trackedSecret := tracked.Run() == nil
	_, statErr := os.Lstat(secretsPath(st.Root))
	if trackedSecret || !os.IsNotExist(statErr) {
		ignored := exec.Command("git", "check-ignore", "-q", "--", secretsFile)
		ignored.Dir = st.Root
		if trackedSecret || ignored.Run() != nil {
			fmt.Fprintf(out, "FAIL  %s must be untracked and ignored by git\n", secretsFile)
			problems++
		}
	}
	plainRefs, secretRefs := map[string]bool{}, map[string]bool{}
	for _, s := range st.Specs() {
		for _, m := range factRefPattern.FindAllStringSubmatch(s.Body, -1) {
			key := m[1]
			if _, secret := secrets[key]; secret {
				fmt.Fprintf(out, "FAIL  %s: %s names a secret fact; use {secret:%s}\n", s.ID(), m[0], key)
				problems++
			} else if _, ok := plain[key]; !ok {
				fmt.Fprintf(out, "FAIL  undefined fact reference %s: %s\n", s.ID(), m[0])
				problems++
			}
			plainRefs[key] = true
		}
		for _, m := range secretRefPattern.FindAllStringSubmatch(s.Body, -1) {
			if _, ok := secrets[m[1]]; !ok {
				fmt.Fprintf(out, "FAIL  undefined secret fact reference %s: %s\n", s.ID(), m[0])
				problems++
			}
			secretRefs[m[1]] = true
		}
		if containsSecretLiteral(s.Body, secretFacts) {
			fmt.Fprintf(out, "WARN  %s contains a possible secret literal\n", s.ID())
		}
	}

	seen := map[string]bool{}
	for _, f := range facts {
		if _, duplicate := secrets[f.Key]; duplicate && !seen[f.Key] {
			fmt.Fprintf(out, "WARN  fact %q is defined in both tiers; the secret wins\n", f.Key)
		}
		if f.Value == "" {
			fmt.Fprintf(out, "WARN  fact %q has an empty value\n", f.Key)
		}
		if seen[f.Key] {
			fmt.Fprintf(out, "WARN  fact %q is defined more than once\n", f.Key)
		}
		seen[f.Key] = true
		if !plainRefs[f.Key] {
			fmt.Fprintf(out, "WARN  fact %q is never referenced ({fact:%s})\n", f.Key, f.Key)
		}
		if tokenPattern.MatchString(f.Value) || highEntropy(f.Value) {
			fmt.Fprintf(out, "WARN  .spec/facts.yml fact %q contains a possible secret literal\n", f.Key)
		}
	}

	for _, f := range secretFacts {
		if f.Value == "" {
			fmt.Fprintf(out, "WARN  secret fact %q has an empty value\n", f.Key)
		}
		if !secretRefs[f.Key] {
			fmt.Fprintf(out, "WARN  secret fact %q is never referenced ({secret:%s})\n", f.Key, f.Key)
		}
	}
	filepath.WalkDir(filepath.Join(st.Root, configDir, "skills"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if data, err := os.ReadFile(path); err == nil && containsSecretLiteral(string(data), secretFacts) {
				rel, _ := filepath.Rel(st.Root, path)
				fmt.Fprintf(out, "WARN  %s contains a possible secret literal\n", rel)
			}
		}
		return nil
	})
	return problems
}
