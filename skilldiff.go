package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// cmdSkillDiff answers "what would --force discard": per target dir it
// diffs the installed copy against the resolved source. --library
// instead diffs .spec/skills/<name> against the library entry — the
// pre-push question.
func cmdSkillDiff(st Store, cfg Config, targets []string, name string, library bool, out io.Writer) error {
	if library {
		repoDir := repoSkillDir(st.Root, name)
		libDir := librarySkillDir(name)
		repoFiles, _ := skillFiles(repoDir)
		libFiles, _ := skillFiles(libDir)
		if repoFiles == nil {
			return fmt.Errorf("no repo skill %q", name)
		}
		if libFiles == nil {
			return fmt.Errorf("no library skill %q", name)
		}
		diffFileSets(out, repoDir, repoFiles, libDir, libFiles)
		return nil
	}
	src := resolveSkill(st, cfg, name)
	if src.kind == "" {
		return fmt.Errorf("skill %q has no resolvable source", name)
	}
	srcFiles, err := skillFiles(src.dir)
	if err != nil {
		return err
	}
	srcLabel := src.dir
	if srcLabel == "" {
		srcLabel = "(embedded)"
	}
	for _, t := range targets {
		dst := filepath.Join(st.Root, t, name)
		status := skillInstallStatus(st.Root, dst, src, srcFiles)
		if status == "current" || status == "missing" {
			fmt.Fprintf(out, "%-8s %s\n", status, dst)
			continue
		}
		// stale/copied/relink — diff the installed files against source.
		// A link to another source resolves to what it actually serves.
		var instFiles map[string][]byte
		if li, err := os.Lstat(dst); err == nil && li.Mode()&os.ModeSymlink != 0 {
			if p, err := filepath.EvalSymlinks(dst); err == nil {
				instFiles, _ = skillFiles(p)
			}
		} else {
			instFiles, _ = skillFiles(dst)
		}
		fmt.Fprintf(out, "%-8s %s (source: %s)\n", status, dst, srcLabel)
		if instFiles != nil && !skillFilesEqual(instFiles, srcFiles) {
			diffFileSets(out, dst, instFiles, srcLabel, srcFiles)
		}
	}
	return nil
}

// diffFileSets prints file-level adds/removes and a unified diff per
// file present on both sides but differing.
func diffFileSets(out io.Writer, aLabel string, a map[string][]byte, bLabel string, b map[string][]byte) {
	var names []string
	seen := map[string]bool{}
	for n := range a {
		names = append(names, n)
		seen[n] = true
	}
	for n := range b {
		if !seen[n] {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, rel := range names {
		av, inA := a[rel]
		bv, inB := b[rel]
		switch {
		case !inA:
			fmt.Fprintf(out, "only in %s: %s\n", bLabel, rel)
		case !inB:
			fmt.Fprintf(out, "only in %s: %s\n", aLabel, rel)
		case bytes.Equal(av, bv):
		case isBinary(av) || isBinary(bv):
			fmt.Fprintf(out, "binary files differ: %s\n", rel)
		default:
			fmt.Fprint(out, unifiedDiff(
				filepath.ToSlash(filepath.Join(aLabel, rel)),
				filepath.ToSlash(filepath.Join(bLabel, rel)), av, bv))
		}
	}
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

type diffOp struct {
	kind byte // ' ' context, '-' removed, '+' added
	text string
}

// unifiedDiff renders a as removed ("-") and b as added ("+") with
// three lines of context, in the usual unified shape.
func unifiedDiff(aLabel, bLabel string, a, b []byte) string {
	al := strings.Split(strings.TrimSuffix(string(a), "\n"), "\n")
	bl := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	ops := diffOps(al, bl)
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", aLabel, bLabel)
	const ctx = 3
	for i := 0; i < len(ops); {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i == len(ops) {
			break
		}
		h0 := i - ctx
		if h0 < 0 {
			h0 = 0
		}
		last := i
		for j := i; j < len(ops); {
			if ops[j].kind != ' ' {
				last = j
				j++
				continue
			}
			k := j
			for k < len(ops) && ops[k].kind == ' ' {
				k++
			}
			if k == len(ops) || k-j > 2*ctx {
				break
			}
			j = k
		}
		h1 := last + ctx + 1
		if h1 > len(ops) {
			h1 = len(ops)
		}
		var aStart, bStart, aLen, bLen int
		for _, o := range ops[:h0] {
			if o.kind != '+' {
				aStart++
			}
			if o.kind != '-' {
				bStart++
			}
		}
		for _, o := range ops[h0:h1] {
			if o.kind != '+' {
				aLen++
			}
			if o.kind != '-' {
				bLen++
			}
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", aStart+1, aLen, bStart+1, bLen)
		for _, o := range ops[h0:h1] {
			fmt.Fprintf(&sb, "%c%s\n", o.kind, o.text)
		}
		i = h1
	}
	return sb.String()
}

// diffOps computes the line edit script from a to b: common prefix and
// suffix become context for free, the middle is solved with a small
// LCS (capped — pathological inputs degrade to a wholesale rewrite).
func diffOps(a, b []string) []diffOp {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	var ops []diffOp
	for _, l := range a[:pre] {
		ops = append(ops, diffOp{' ', l})
	}
	ops = append(ops, midOps(a[pre:len(a)-suf], b[pre:len(b)-suf])...)
	for _, l := range a[len(a)-suf:] {
		ops = append(ops, diffOp{' ', l})
	}
	return ops
}

const maxLCSCells = 4 << 20

func midOps(a, b []string) []diffOp {
	var ops []diffOp
	n, m := len(a), len(b)
	if n*m > maxLCSCells {
		for _, l := range a {
			ops = append(ops, diffOp{'-', l})
		}
		for _, l := range b {
			ops = append(ops, diffOp{'+', l})
		}
		return ops
	}
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] >= dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}
