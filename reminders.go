package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Reminders live in .spec/reminders.yml — a list of "- key: value" items
// with indented fields, like facts.yml but multi-field. They track
// recurring obligations (the operational when) separately from the specs
// that mandate them (the why); an optional spec field links the two.
// The file is committed: due state is shared team/agent truth.

type Reminder struct {
	ID       string
	Title    string
	Every    string // "7d"/"30d"/… or weekly|monthly|quarterly|yearly
	LastDone string // YYYY-MM-DD; empty means never done
	Spec     string // optional spec id that mandates the cadence
}

func remindersPath(root string) string {
	return filepath.Join(root, configDir, "reminders.yml")
}

var reminderField = regexp.MustCompile(`^([a-z_]+):\s*(.*)$`)

// loadReminders parses .spec/reminders.yml preserving declaration order.
// Lines before the first "- " item are the preamble (comments) and are
// preserved verbatim on write; comments between entries are not kept.
func loadReminders(root string) []Reminder {
	data, err := os.ReadFile(remindersPath(root))
	if err != nil {
		return nil
	}
	var out []Reminder
	var cur *Reminder
	set := func(key, val string) {
		if cur == nil {
			return
		}
		val = strings.Trim(val, `"'`)
		switch key {
		case "id":
			cur.ID = val
		case "title":
			cur.Title = val
		case "every":
			cur.Every = val
		case "last_done":
			cur.LastDone = val
		case "spec":
			cur.Spec = val
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "-") {
			out = append(out, Reminder{})
			cur = &out[len(out)-1]
			if m := reminderField.FindStringSubmatch(strings.TrimSpace(t[1:])); m != nil {
				set(m[1], m[2])
			}
			continue
		}
		if m := reminderField.FindStringSubmatch(t); m != nil {
			set(m[1], m[2])
		}
	}
	return out
}

// writeReminders rewrites the file, keeping the preamble (comment/blank
// lines before the first entry) verbatim.
func writeReminders(root string, rs []Reminder) error {
	path := remindersPath(root)
	var b strings.Builder
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "-") {
				break
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	for _, r := range rs {
		fmt.Fprintf(&b, "- id: %s\n  title: %s\n  every: %s\n", r.ID, r.Title, r.Every)
		if r.LastDone != "" {
			fmt.Fprintf(&b, "  last_done: %s\n", r.LastDone)
		}
		if r.Spec != "" {
			fmt.Fprintf(&b, "  spec: %s\n", r.Spec)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(b.String()), 0o644)
}

var dayInterval = regexp.MustCompile(`^(\d+)d$`)

// validEvery reports whether every is a supported cadence: a day interval
// (7d, 30d, …) or a calendar period.
func validEvery(every string) bool {
	switch every {
	case "weekly", "monthly", "quarterly", "yearly":
		return true
	}
	if m := dayInterval.FindStringSubmatch(every); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n > 0
	}
	return false
}

func quarterIndex(t time.Time) int {
	return t.Year()*4 + int(t.Month()-1)/3
}

func sameDayUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Due reports whether the reminder needs doing: never done or unparseable
// last_done → due; a future last_done → not due. Day intervals are due
// when last_done is ≥ N days ago; calendar periods are due when last_done
// falls in an earlier week/month/quarter/year than now.
func (r Reminder) Due(now time.Time) bool {
	d, err := time.ParseInLocation("2006-01-02", r.LastDone, time.UTC)
	if err != nil {
		return true
	}
	if sameDayUTC(d).After(sameDayUTC(now)) {
		return false
	}
	switch r.Every {
	case "weekly":
		y1, w1 := d.ISOWeek()
		y2, w2 := now.ISOWeek()
		return y1 < y2 || (y1 == y2 && w1 < w2)
	case "monthly":
		return d.Year() < now.Year() || (d.Year() == now.Year() && d.Month() < now.Month())
	case "quarterly":
		return quarterIndex(d) < quarterIndex(now)
	case "yearly":
		return d.Year() < now.Year()
	}
	if m := dayInterval.FindStringSubmatch(r.Every); m != nil {
		n, _ := strconv.Atoi(m[1])
		return !d.AddDate(0, 0, n).After(sameDayUTC(now))
	}
	return true // unparseable cadence — nag rather than silently skip
}

func dueReminders(root string) []Reminder {
	var out []Reminder
	for _, r := range loadReminders(root) {
		if r.Due(time.Now()) {
			out = append(out, r)
		}
	}
	return out
}

func cmdRemind(out, errOut io.Writer, args []string) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		rs := loadReminders(st.Root)
		if len(rs) == 0 {
			fmt.Fprintln(out, "No reminders (.spec/reminders.yml).")
			return nil
		}
		now := time.Now()
		for _, r := range rs {
			mark := "    "
			if r.Due(now) {
				mark = "DUE "
			}
			last := r.LastDone
			if last == "" {
				last = "never"
			}
			fmt.Fprintf(out, "%s%-16s every %-10s last done %s", mark, r.ID, r.Every, last)
			if r.Spec != "" {
				fmt.Fprintf(out, "  (%s)", r.Spec)
			}
			fmt.Fprintln(out)
			if !validEvery(r.Every) {
				fmt.Fprintf(out, "     warning: invalid cadence %q\n", r.Every)
			}
		}
		return nil
	case "add":
		var every, specID string
		var title []string
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "--every" && i+1 < len(args):
				i++
				every = args[i]
			case args[i] == "--spec" && i+1 < len(args):
				i++
				specID = args[i]
			default:
				title = append(title, args[i])
			}
		}
		if len(title) == 0 || every == "" {
			return fmt.Errorf("usage: spec remind add <title> --every <7d|30d|weekly|monthly|quarterly|yearly> [--spec <id>]")
		}
		if !validEvery(every) {
			return fmt.Errorf("invalid cadence %q (use e.g. 30d or weekly|monthly|quarterly|yearly)", every)
		}
		if specID != "" && st.Get(specID) == nil {
			return fmt.Errorf("no spec with id %s", specID)
		}
		rs := loadReminders(st.Root)
		base := slugify(strings.Join(title, " "))
		id := base
		for n := 2; ; n++ {
			clash := false
			for _, r := range rs {
				if r.ID == id {
					clash = true
				}
			}
			if !clash {
				break
			}
			id = fmt.Sprintf("%s-%d", base, n)
		}
		rs = append(rs, Reminder{ID: id, Title: strings.Join(title, " "), Every: every, Spec: specID})
		if err := writeReminders(st.Root, rs); err != nil {
			return err
		}
		fmt.Fprintf(out, "Added %s — due now (mark done: spec remind done %s)\n", id, id)
		return nil
	case "done":
		if len(args) != 2 {
			return fmt.Errorf("usage: spec remind done <id>")
		}
		rs := loadReminders(st.Root)
		for i, r := range rs {
			if r.ID == args[1] {
				rs[i].LastDone = time.Now().Format("2006-01-02")
				if err := writeReminders(st.Root, rs); err != nil {
					return err
				}
				fmt.Fprintf(out, "Done %s (last_done: %s)\n", r.ID, rs[i].LastDone)
				return nil
			}
		}
		return fmt.Errorf("no reminder %q", args[1])
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: spec remind remove <id>")
		}
		rs := loadReminders(st.Root)
		for i, r := range rs {
			if r.ID == args[1] {
				rs = append(rs[:i], rs[i+1:]...)
				if err := writeReminders(st.Root, rs); err != nil {
					return err
				}
				fmt.Fprintf(out, "Removed %s\n", r.ID)
				return nil
			}
		}
		return fmt.Errorf("no reminder %q", args[1])
	}
	return fmt.Errorf("usage: spec remind [list|add <title> --every <period> [--spec <id>]|done <id>|remove <id>]")
}
