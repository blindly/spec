package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReminderDueSemantics(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) // Thursday, ISO week 45, Q4
	cases := []struct {
		name string
		r    Reminder
		want bool
	}{
		{"never done", Reminder{Every: "monthly"}, true},
		{"unparseable last_done", Reminder{Every: "monthly", LastDone: "soon"}, true},
		{"future last_done", Reminder{Every: "30d", LastDone: "2026-12-01"}, false},
		{"30d boundary due", Reminder{Every: "30d", LastDone: "2026-10-06"}, true},
		{"30d not yet", Reminder{Every: "30d", LastDone: "2026-10-07"}, false},
		{"7d due", Reminder{Every: "7d", LastDone: "2026-10-28"}, true},
		{"7d same day", Reminder{Every: "7d", LastDone: "2026-11-05"}, false},
		{"monthly prior month", Reminder{Every: "monthly", LastDone: "2026-10-31"}, true},
		{"monthly same month", Reminder{Every: "monthly", LastDone: "2026-11-01"}, false},
		{"monthly done first, due next month", Reminder{Every: "monthly", LastDone: "2026-10-01"}, true},
		{"weekly prior week", Reminder{Every: "weekly", LastDone: "2026-10-30"}, true},
		{"weekly same week", Reminder{Every: "weekly", LastDone: "2026-11-02"}, false},
		{"quarterly prior quarter", Reminder{Every: "quarterly", LastDone: "2026-09-30"}, true},
		{"quarterly same quarter", Reminder{Every: "quarterly", LastDone: "2026-10-01"}, false},
		{"yearly prior year", Reminder{Every: "yearly", LastDone: "2025-12-31"}, true},
		{"yearly same year", Reminder{Every: "yearly", LastDone: "2026-01-01"}, false},
		{"invalid cadence nags", Reminder{Every: "sometimes", LastDone: "2026-11-05"}, true},
	}
	for _, c := range cases {
		if got := c.r.Due(now); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRemindAddDoneRemove(t *testing.T) {
	st := setupRepo(t)
	path := filepath.Join(st.Root, ".spec", "reminders.yml")
	os.WriteFile(path, []byte("# team recurring tasks\n"), 0o644)
	var out bytes.Buffer

	if err := cmdRemind(&out, &out, []string{"add", "write the monthly blog post", "--every", "monthly"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "# team recurring tasks\n") {
		t.Fatalf("comment preamble lost:\n%s", data)
	}
	if !strings.Contains(string(data), "id: write-the-monthly-blog-post") || !strings.Contains(string(data), "every: monthly") {
		t.Fatalf("entry not written:\n%s", data)
	}
	// duplicate title gets a deduplicated id
	if err := cmdRemind(&out, &out, []string{"add", "write the monthly blog post", "--every", "30d"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(slurp(path)), "id: write-the-monthly-blog-post-2") {
		t.Fatalf("duplicate id not deduplicated:\n%s", slurp(path))
	}
	// new entries are due
	out.Reset()
	if err := cmdRemind(&out, &out, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "DUE write-the-monthly-blog-post") {
		t.Fatalf("new reminder not due:\n%s", out.String())
	}
	// done stamps today and clears the DUE marker
	if err := cmdRemind(&out, &out, []string{"done", "write-the-monthly-blog-post"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(slurp(path), "last_done: "+time.Now().Format("2006-01-02")) {
		t.Fatalf("last_done not stamped:\n%s", slurp(path))
	}
	out.Reset()
	cmdRemind(&out, &out, nil)
	if strings.Contains(out.String(), "DUE write-the-monthly-blog-post ") {
		t.Fatalf("completed reminder still due:\n%s", out.String())
	}
	// remove deletes the entry
	if err := cmdRemind(&out, &out, []string{"remove", "write-the-monthly-blog-post-2"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(slurp(path), "blog-post-2") {
		t.Fatal("entry not removed")
	}
	if err := cmdRemind(&out, &out, []string{"done", "write-the-monthly-blog-post-2"}); err == nil {
		t.Fatal("done on removed id accepted")
	}
}

func slurp(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func TestRemindValidation(t *testing.T) {
	st := setupRepo(t)
	var out bytes.Buffer
	if err := cmdRemind(&out, &out, []string{"add", "x"}); err == nil {
		t.Fatal("add without --every accepted")
	}
	if err := cmdRemind(&out, &out, []string{"add", "x", "--every", "sometimes"}); err == nil {
		t.Fatal("invalid cadence accepted")
	}
	if err := cmdRemind(&out, &out, []string{"add", "x", "--every", "monthly", "--spec", "FEAT-999"}); err == nil {
		t.Fatal("unknown spec link accepted")
	}
	// malformed entry warns in check instead of breaking
	os.WriteFile(filepath.Join(st.Root, ".spec", "reminders.yml"),
		[]byte("- id: bad\n  title: broken\n  every: fortnightly\n  spec: FEAT-999\n"), 0o644)
	var check bytes.Buffer
	if err := cmdCheck(&check, &check); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(check.String(), `invalid cadence "fortnightly"`) ||
		!strings.Contains(check.String(), "unknown spec FEAT-999") ||
		!strings.Contains(check.String(), `reminder "bad" is due`) {
		t.Fatalf("check should warn on misconfig and due:\n%s", check.String())
	}
}

func TestRemindSurfacing(t *testing.T) {
	st := setupRepo(t)
	writeFile(t, st, ".spec/reminders.yml",
		"- id: blog-post\n  title: Write the monthly blog post\n  every: monthly\n  spec: PROJ-001\n"+
			"- id: deps\n  title: Dependency review\n  every: yearly\n  last_done: "+time.Now().Format("2006-01-02")+"\n")

	var status, check bytes.Buffer
	if err := cmdStatus(&status, &status); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.String(), "Reminders: 1 due (blog-post)") {
		t.Fatalf("status missing due reminders:\n%s", status.String())
	}
	if err := cmdCheck(&check, &check); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(check.String(), `reminder "blog-post" is due`) || strings.Contains(check.String(), `"deps" is due`) {
		t.Fatalf("check should warn only on due reminders:\n%s", check.String())
	}
	ctx := repoContext(st, 50)
	if !strings.Contains(ctx, "Recurring tasks due:") || !strings.Contains(ctx, "Write the monthly blog post (blog-post, see PROJ-001)") {
		t.Fatalf("due reminder missing from repoContext:\n%s", ctx)
	}
	if strings.Contains(ctx, "Dependency review") {
		t.Fatal("non-due reminder leaked into repoContext")
	}
}
