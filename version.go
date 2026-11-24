package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// versionRe parses "<major>.<yy>.<mm>-build-<ddhhmm>", tolerating a
// leading v and missing minor/build parts.
var versionRe = regexp.MustCompile(`^v?(\d+)(?:\.(\d+)\.(\d+)(?:[-_]build-(\d+))?)?`)

// stampVersion rewrites <root>/VERSION as <major>.<yy>.<mm>-build-<ddhhmm>
// and returns the stamp. An absent file means the repo hasn't opted into
// versioned releases — it returns "" and writes nothing. major > 0 forces
// the major; otherwise the file's own major is kept (so bumping it is just
// editing VERSION) and only auto-increments when the fresh stamp would
// sort before the stored one — a clock regression.
func stampVersion(root string, major int) (string, error) {
	path := filepath.Join(root, "VERSION")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if major == 0 {
		major = 1
	}
	prevYY, prevMM, prevStamp := -1, -1, -1
	if m := versionRe.FindStringSubmatch(strings.TrimSpace(string(data))); m != nil {
		if forced := atoi(m[1]); major == 1 && forced > 0 {
			major = forced
		}
		if m[2] != "" {
			prevYY, prevMM, prevStamp = atoi(m[2]), atoi(m[3]), atoi(m[4])
		}
	}
	now := time.Now()
	yy, mm := now.Year()%100, int(now.Month())
	ds := fmt.Sprintf("%02d%02d%02d", now.Day(), now.Hour(), now.Minute())
	dayStamp, _ := strconv.Atoi(ds)
	if prevYY >= 0 && (yy < prevYY ||
		yy == prevYY && (mm < prevMM || mm == prevMM && dayStamp < prevStamp)) {
		major++
	}
	stamp := fmt.Sprintf("%d.%02d.%02d-build-%s", major, yy, mm, ds)
	if err := os.WriteFile(path, []byte(stamp+"\n"), 0o644); err != nil {
		return "", err
	}
	return stamp, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// cmdRelease stamps VERSION, commits it alone as "chore: bump <stamp>"
// (the prefix self-attests — no trailer needed), pushes the current
// branch and tags v<stamp>. The tag triggers the release workflow.
func cmdRelease(out, errOut io.Writer, args []string) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	fs.SetOutput(errOut)
	major := fs.Int("major", 0, "set the major explicitly")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := mustStore()
	if err != nil {
		return err
	}
	if dirty := strings.TrimSpace(mustGit(st.Root, "status", "--porcelain")); dirty != "" {
		return fmt.Errorf("uncommitted changes — spec commit first")
	}
	stamp, err := stampVersion(st.Root, *major)
	if err != nil {
		return fmt.Errorf("VERSION: %w", err)
	}
	if stamp == "" {
		return fmt.Errorf("no VERSION file — create one to opt into versioned releases")
	}
	tag := "v" + stamp
	if out2 := strings.TrimSpace(mustGit(st.Root, "rev-parse", "--verify", "--quiet", tag)); out2 != "" {
		return fmt.Errorf("tag %s already exists", tag)
	}
	// Commit VERSION only when the stamp actually changed — HEAD often
	// already carries it (spec commit just stamped the same minute).
	if _, err := gitOut(st.Root, "diff", "--quiet", "--", "VERSION"); err != nil {
		if out2, err := gitOut(st.Root, "add", "VERSION"); err != nil {
			return fmt.Errorf("git add: %s", out2)
		}
		if out2, err := gitOut(st.Root, "commit", "-m", "chore: bump "+stamp, "--", "VERSION"); err != nil {
			return fmt.Errorf("git commit: %s", out2)
		}
	}
	branch := strings.TrimSpace(mustGit(st.Root, "branch", "--show-current"))
	if out2, err := gitOut(st.Root, "push", "origin", branch); err != nil {
		return fmt.Errorf("git push: %s", out2)
	}
	if out2, err := gitOut(st.Root, "tag", tag); err != nil {
		return fmt.Errorf("git tag: %s", out2)
	}
	if out2, err := gitOut(st.Root, "push", "origin", tag); err != nil {
		fmt.Fprintf(errOut, "warning: tag %s created locally but push failed: %s", tag, out2)
		return fmt.Errorf("tag push failed — retry with: git push origin %s", tag)
	}
	fmt.Fprintf(out, "released %s\n", tag)
	return nil
}
