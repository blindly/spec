package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"html"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

func webToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// projectIdent returns the repo's display name — the project spec's title
// when present, else the directory name — plus a stable accent hue derived
// from it so different repos get different color schemes.
func projectIdent(st Store) (string, int) {
	name := filepath.Base(st.Root)
	for _, s := range st.Specs() {
		if s.Type() == "project" {
			name = s.Title()
			break
		}
	}
	h := fnv.New32a()
	h.Write([]byte(name))
	return name, int(h.Sum32() % 360)
}

func page(w http.ResponseWriter, title, body, projName string, hue int) {
	accent := fmt.Sprintf("hsl(%d,65%%,45%%)", hue)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<html><head><title>%s · %s</title>
<link rel="stylesheet" href="/assets/water/water.min.css">
<style>nav a{margin-right:1em}textarea{font-family:monospace}
nav{border-bottom:4px solid %s;padding-bottom:.5em}
.proj{display:inline-block;background:%s;color:#fff;padding:.15em .6em;border-radius:4px;margin-right:1em;font-weight:bold}
.board{display:flex;gap:1em;align-items:flex-start;overflow-x:auto}
.col{flex:1 1 0;min-width:12em}
.col h2{font-size:1em;border-bottom:2px solid %s;padding-bottom:.3em}
.col .count{font-weight:normal;color:var(--text-muted,#777);font-size:.85em}
.card{border:1px solid var(--border,#444);border-radius:6px;padding:.5em .7em;margin-bottom:.6em}
.card .title{margin:.2em 0}
.card .type{font-size:.8em;color:var(--text-muted,#777)}
.inbox td{vertical-align:middle}
.inbox form{display:inline;margin:0}
.inbox select,.inbox button{padding:.2em .5em;margin:0 .2em}
.badge{display:inline-block;font-size:.8em;padding:.05em .5em;border-radius:3px;border:1px solid %s}
.muted{color:var(--text-muted,#777)}
footer{margin-top:2em;font-size:.85em;color:var(--text-muted,#777)}</style></head><body>
<nav><span class="proj">%s</span><a href="/">inbox</a><a href="/board">board</a><a href="/new">new spec</a><a href="/readme">readme</a><a href="/agents">agents</a><a href="/help">help</a></nav>%s
<footer>spec %s</footer></body></html>`,
		html.EscapeString(projName), html.EscapeString(title), accent, accent,
		accent, accent, html.EscapeString(projName), body, html.EscapeString(version))
}

//go:embed assets/easymde assets/fontawesome assets/water assets/marked
var assetsFS embed.FS

// mdBlock renders markdown client-side via vendored marked.js. The raw
// markdown goes in a <script type="text/markdown"> block; </script is
// escaped so it can't terminate the tag early.
func mdBlock(markdown string) string {
	src := strings.ReplaceAll(markdown, "</script", `<\/script`)
	return `<div id="md" class="markdown"></div>
<script type="text/markdown" id="md-src">` + src + `</script>
<script src="/assets/marked/marked.min.js"></script>
<script>document.getElementById('md').innerHTML=marked.parse(document.getElementById('md-src').textContent)</script>`
}

// webHandler builds the mux; extracted from cmdWeb so tests can use httptest.
func webHandler(st Store, cfg Config, token string) http.Handler {
	statuses := cfg.Statuses
	var writeMu sync.Mutex

	// resolve branding per request so creating a project spec while the
	// server runs updates the badge without a restart
	render := func(w http.ResponseWriter, title, body string) {
		name, hue := projectIdent(st)
		page(w, title, body, name, hue)
	}

	checkToken := func(r *http.Request) bool {
		return r.FormValue("token") == token
	}

	mux := http.NewServeMux()
	mux.Handle("/assets/", http.FileServerFS(assetsFS))

	// inbox (landing page): untyped ideas as a list, newest first
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		showDiscarded := r.URL.Query().Get("all") == "1"
		var ideas []Spec
		discarded := 0
		for _, s := range st.Specs() {
			if !s.IsIdea() || s.Archived {
				continue
			}
			if s.Status() == ideaDiscarded && !showDiscarded {
				discarded++
				continue
			}
			ideas = append(ideas, s)
		}
		sort.SliceStable(ideas, func(i, j int) bool {
			ci, cj := metaGet(ideas[i].Meta, "created", ""), metaGet(ideas[j].Meta, "created", "")
			if ci != cj {
				return ci > cj
			}
			return ideas[i].ID() > ideas[j].ID()
		})
		var b strings.Builder
		fmt.Fprintf(&b, `<h1>Inbox</h1>
<form method="post" action="/idea"><input type="hidden" name="token" value="%s">
<p><input name="title" placeholder="Capture an idea — just a title, decide the type later" size="60" required autofocus> <button>Add</button></p></form>`, token)
		if len(ideas) == 0 && discarded == 0 {
			b.WriteString(`<p>Inbox empty — capture an idea above, or go to the <a href="/board">board</a>.</p>`)
		} else if len(ideas) > 0 {
			b.WriteString(`<table class="inbox"><thead><tr><th>id</th><th>idea</th><th>status</th><th>suggested</th><th>created</th><th></th></tr></thead><tbody>`)
			for _, s := range ideas {
				id := html.EscapeString(s.ID())
				suggested := metaGet(s.Meta, "suggested_type", "")
				badge := ""
				if suggested != "" {
					badge = fmt.Sprintf(`<span class="badge" title="%s">%s</span>`,
						html.EscapeString(metaGet(s.Meta, "suggested_reason", "")), html.EscapeString(suggested))
				}
				fmt.Fprintf(&b, `<tr><td><a href="/spec/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td><td class="muted">%s</td><td>`,
					id, id, html.EscapeString(s.Title()), html.EscapeString(s.Status()), badge,
					html.EscapeString(metaGet(s.Meta, "created", "")))
				if s.Status() != ideaDiscarded {
					fmt.Fprintf(&b, `<form method="post" action="/promote/%s"><input type="hidden" name="token" value="%s"><select name="type" required>`, id, token)
					if suggested == "" {
						b.WriteString(`<option value="" selected disabled>type…</option>`)
					}
					for _, t := range []string{"project", "feature", "adr"} {
						sel := ""
						if t == suggested {
							sel = " selected"
						}
						fmt.Fprintf(&b, `<option%s>%s</option>`, sel, t)
					}
					fmt.Fprintf(&b, `</select><button>promote</button></form>
<form method="post" action="/discard/%s"><input type="hidden" name="token" value="%s"><button>discard</button></form>`, id, token)
				}
				b.WriteString(`</td></tr>`)
			}
			b.WriteString(`</tbody></table>`)
		}
		switch {
		case discarded > 0:
			fmt.Fprintf(&b, `<p class="toggle muted">%d discarded — <a href="/?all=1">show</a></p>`, discarded)
		case showDiscarded:
			b.WriteString(`<p class="toggle muted"><a href="/">hide discarded</a></p>`)
		}
		render(w, "inbox", b.String())
	})

	// postOnly guards inbox write endpoints: POST with a valid form token.
	postOnly := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodPost || !checkToken(r) {
			http.Error(w, "bad token", http.StatusForbidden)
			return false
		}
		return true
	}

	mux.HandleFunc("/idea", func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		writeMu.Lock()
		_, err := st.Create("idea", title, templateFor("idea", title), nil)
		writeMu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	mux.HandleFunc("/promote/", func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		writeMu.Lock()
		ns, err := promoteIdea(st, strings.TrimPrefix(r.URL.Path, "/promote/"), r.FormValue("type"), false)
		writeMu.Unlock()
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			render(w, "error", `<p style="color:red">`+html.EscapeString(err.Error())+`</p><p><a href="/">back to inbox</a></p>`)
			return
		}
		http.Redirect(w, r, "/edit/"+ns.ID(), http.StatusSeeOther)
	})

	mux.HandleFunc("/discard/", func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		writeMu.Lock()
		err := discardIdea(st, strings.TrimPrefix(r.URL.Path, "/discard/"))
		writeMu.Unlock()
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			render(w, "error", `<p style="color:red">`+html.EscapeString(err.Error())+`</p><p><a href="/">back to inbox</a></p>`)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// archive: read-only list of archived specs (moved to specs/archive/)
	mux.HandleFunc("/archive", func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString(`<h1>Archive</h1>`)
		var rows []Spec
		for _, s := range st.Specs() {
			if s.Archived {
				rows = append(rows, s)
			}
		}
		if len(rows) == 0 {
			b.WriteString(`<p>Nothing archived. Finished specs can be archived with <code>spec archive &lt;id&gt;</code>.</p>`)
			render(w, "archive", b.String())
			return
		}
		b.WriteString(`<p class="muted">Archived specs are kept for history and still searchable. Restore one with <code>spec unarchive &lt;id&gt;</code>.</p>
<table class="inbox"><thead><tr><th>id</th><th>title</th><th>type</th><th>status</th><th>archived</th></tr></thead><tbody>`)
		for _, s := range rows {
			id := html.EscapeString(s.ID())
			fmt.Fprintf(&b, `<tr><td><a href="/spec/%s">%s</a></td><td>%s</td><td>%s</td><td>%s</td><td class="muted">%s</td></tr>`,
				id, id, html.EscapeString(s.Title()), html.EscapeString(s.Type()), html.EscapeString(s.Status()),
				html.EscapeString(metaGet(s.Meta, "archived", "")))
		}
		b.WriteString(`</tbody></table><p><a href="/board">back to board</a></p>`)
		render(w, "archive", b.String())
	})

	mux.HandleFunc("/board", func(w http.ResponseWriter, r *http.Request) {
		// finished specs (done_statuses) are hidden unless ?all=1
		showAll := r.URL.Query().Get("all") == "1"
		// group specs by status, columns ordered by configured statuses;
		// ideas live in the inbox and archived specs in /archive — never here
		var specsAll []Spec
		archived := 0
		for _, s := range st.Specs() {
			switch {
			case s.Archived:
				archived++
			case !s.IsIdea():
				specsAll = append(specsAll, s)
			}
		}
		byStatus := map[string][]Spec{}
		var extra []string // statuses present but not in cfg.Statuses
		for _, s := range specsAll {
			byStatus[s.Status()] = append(byStatus[s.Status()], s)
		}
		var cols []string
		hidden := 0
		inCfg := map[string]bool{}
		addCol := func(s string) {
			if len(byStatus[s]) == 0 {
				return
			}
			if cfg.isDone(s) && !showAll {
				hidden += len(byStatus[s])
				return
			}
			cols = append(cols, s)
		}
		for _, s := range statuses {
			inCfg[s] = true
			addCol(s)
		}
		for s := range byStatus {
			if !inCfg[s] {
				extra = append(extra, s)
			}
		}
		sort.Strings(extra)
		for _, s := range extra {
			addCol(s)
		}

		archiveLink := ""
		if archived > 0 {
			archiveLink = fmt.Sprintf(`<p class="toggle muted">%d archived — <a href="/archive">view archive</a></p>`, archived)
		}
		var b strings.Builder
		b.WriteString(`<h1>Specifications</h1>`)
		if len(specsAll) == 0 {
			b.WriteString(`<p>No active specs — <a href="/new">create one</a>, or promote an idea from the <a href="/">inbox</a>.</p>` + archiveLink)
			render(w, "board", b.String())
			return
		}
		toggle := ""
		switch {
		case hidden > 0:
			toggle = fmt.Sprintf(`<p class="toggle">%d finished spec(s) hidden — <a href="%s?all=1">show</a></p>`, hidden, r.URL.Path)
		case showAll:
			toggle = fmt.Sprintf(`<p class="toggle"><a href="%s">hide finished</a></p>`, r.URL.Path)
		}
		toggle += archiveLink
		if len(cols) == 0 {
			b.WriteString(`<p>Nothing in progress.</p>` + toggle)
			render(w, "board", b.String())
			return
		}
		b.WriteString(`<div class="board">`)
		for _, status := range cols {
			specs := byStatus[status]
			fmt.Fprintf(&b, `<div class="col"><h2>%s <span class="count">%d</span></h2>`,
				html.EscapeString(status), len(specs))
			for _, s := range specs {
				fmt.Fprintf(&b, `<div class="card"><a href="/spec/%s">%s</a><div class="title">%s</div><div class="type">%s</div></div>`,
					s.ID(), html.EscapeString(s.ID()), html.EscapeString(s.Title()),
					html.EscapeString(s.Type()))
			}
			b.WriteString("</div>")
		}
		b.WriteString("</div>" + toggle)
		render(w, "board", b.String())
	})

	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if !checkToken(r) {
				http.Error(w, "bad token", http.StatusForbidden)
				return
			}
			specType := r.FormValue("type")
			title := strings.TrimSpace(r.FormValue("title"))
			if !validTypes[specType] || title == "" {
				render(w, "error", "<p>Invalid type or empty title.</p>")
				return
			}
			writeMu.Lock()
			s, err := st.Create(specType, title, templateFor(specType, title), nil)
			writeMu.Unlock()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/spec/"+s.ID(), http.StatusSeeOther)
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, `<h1>New spec</h1><form method="post">
<input type="hidden" name="token" value="%s">
<p><select name="type">`, token)
		// idea first and selected: not knowing the type yet is the common case
		for _, t := range []string{"idea", "project", "feature", "adr"} {
			sel := ""
			if t == "idea" {
				sel = " selected"
			}
			fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, t, sel, t)
		}
		b.WriteString(`</select> <input name="title" placeholder="Title" size="50" required>
<button>Create</button></p></form>
<p class="muted">Not sure what type it is? Keep <strong>idea</strong> — it goes to the inbox, and you can promote it to a project, feature, or adr later. See <a href="/help">help</a> for choosing a type.</p>`)
		render(w, "new spec", b.String())
	})

	mux.HandleFunc("/spec/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/spec/")
		s := st.Get(id)
		if s == nil {
			http.NotFound(w, r)
			return
		}
		var b strings.Builder
		archivedNote := ""
		if s.Archived {
			archivedNote = fmt.Sprintf(` · <a href="/archive">archived</a> %s`, html.EscapeString(metaGet(s.Meta, "archived", "")))
		}
		fmt.Fprintf(&b, "<h1>%s — %s</h1><p>type: %s · status: %s%s · <a href=\"/edit/%s\">edit</a></p>",
			html.EscapeString(s.ID()), html.EscapeString(s.Title()),
			html.EscapeString(s.Type()), html.EscapeString(s.Status()),
			archivedNote, s.ID())
		b.WriteString(mdBlock(s.Body))
		render(w, s.ID(), b.String())
	})

	mux.HandleFunc("/readme", func(w http.ResponseWriter, r *http.Request) {
		readme := readmeExcerpt(st.Root, 0)
		if readme == "" {
			render(w, "readme", "<h1>README</h1><p>No README found in repository root.</p>")
			return
		}
		render(w, "readme", "<h1>README</h1>"+mdBlock(readme))
	})

	mux.HandleFunc("/agents", func(w http.ResponseWriter, r *http.Request) {
		doc, name := agentsExcerpt(st.Root)
		if doc == "" {
			render(w, "agents", "<h1>Agent instructions</h1><p>No AGENTS.md or CLAUDE.md found in repository root.</p>")
			return
		}
		render(w, "agents", "<h1>"+name+"</h1>"+mdBlock(doc))
	})

	mux.HandleFunc("/help", func(w http.ResponseWriter, r *http.Request) {
		doc := helpMarkdown + "\n## Defined facts\n\n"
		facts := loadFacts(st.Root)
		if len(facts) == 0 {
			doc += "No facts defined (`spec fact set <key> <value>`).\n"
		} else {
			for _, f := range facts {
				doc += fmt.Sprintf("- `{fact:%s}` — %s\n", f.Key, f.Value)
			}
		}
		render(w, "help", mdBlock(doc))
	})

	renderEdit := func(w http.ResponseWriter, s Spec, errMsg string) {
		var b strings.Builder
		if errMsg != "" {
			fmt.Fprintf(&b, `<p style="color:red">%s</p>`, html.EscapeString(errMsg))
		}
		fmt.Fprintf(&b, `<h1>Edit %s — %s</h1>
<form method="post" action="/edit/%s"><input type="hidden" name="token" value="%s">
<p>type: <strong>%s</strong> <small title="the type sets the id prefix, directory, and required sections">(fixed — to change the type, create a new spec)</small>
· status: <select name="status">`, s.ID(), html.EscapeString(s.Title()), s.ID(), token, html.EscapeString(s.Type()))
		for _, status := range cfg.statusesFor(s.Type()) {
			sel := ""
			if s.Status() == status {
				sel = " selected"
			}
			fmt.Fprintf(&b, `<option%s>%s</option>`, sel, status)
		}
		fmt.Fprintf(&b, `</select></p>
<link rel="stylesheet" href="/assets/fontawesome/css/font-awesome.min.css">
<link rel="stylesheet" href="/assets/easymde/easymde.min.css">
<style>
.CodeMirror{background:#1e1e1e;color:#d4d4d4;border-color:#333}
.CodeMirror-cursor{border-left-color:#d4d4d4}
.CodeMirror-gutters{background:#1e1e1e;border-right-color:#333}
.CodeMirror-linenumber{color:#666}
.editor-toolbar{background:#2d2d2d;border-color:#333}
.editor-toolbar button{color:#d4d4d4!important}
.editor-toolbar button:hover,.editor-toolbar button.active{background:#3d3d3d;border-color:#555}
.editor-preview,.editor-preview-side{background:#1e1e1e;color:#d4d4d4}
.editor-preview *,.editor-preview-side *{color:#d4d4d4!important;background:transparent!important}
.editor-statusbar{color:#999}
.CodeMirror-selected{background:#334!important}
.cm-header,.cm-strong{color:#569cd6!important}
.cm-link,.cm-url{color:#4ec9b0!important}
.cm-quote,.cm-comment{color:#6a9955!important}
.CodeMirror-fullscreen,.editor-preview-fullscreen,.editor-preview-active,.editor-preview-active-side{background:#1e1e1e!important;color:#d4d4d4!important;border-color:#333}
.editor-preview-fullscreen *,.editor-preview-active *,.editor-preview-active-side *{color:#d4d4d4!important;background:transparent!important}
</style>
<textarea id="body-editor" name="body" rows="30">%s</textarea>
<p><button>Save</button> <a href="/spec/%s">cancel</a></p></form>
<script src="/assets/easymde/easymde.min.js"></script>
<script>new EasyMDE({element: document.getElementById('body-editor'),
autoDownloadFontAwesome: false, spellChecker: false, status: ['lines','words','cursor']});</script>`,
			html.EscapeString(s.Body), s.ID())
		render(w, "edit "+s.ID(), b.String())
	}

	mux.HandleFunc("/edit/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/edit/")
		s := st.Get(id)
		if s == nil {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			renderEdit(w, *s, "")
			return
		}
		if r.Method != http.MethodPost || !checkToken(r) {
			http.Error(w, "bad token", http.StatusForbidden)
			return
		}
		// the type is fixed at creation (it sets id prefix and directory), so
		// any submitted "type" is ignored and the current one is kept
		status := r.FormValue("status")
		body := r.FormValue("body")
		if !cfg.validStatusFor(s.Type(), status) {
			renderEdit(w, *s, "invalid status")
			return
		}
		// section check: same required sections validate enforces
		updated := Spec{Path: s.Path, Meta: s.Meta, Body: body}
		for _, sec := range requiredSections[s.Type()] {
			found := false
			for _, have := range updated.Sections() {
				if strings.EqualFold(have, sec) {
					found = true
				}
			}
			if !found {
				renderEdit(w, updated, fmt.Sprintf("missing required section %q — not saved", sec))
				return
			}
		}
		meta := map[string][]string{}
		for k, v := range s.Meta {
			meta[k] = v
		}
		meta["status"] = []string{status}
		writeMu.Lock()
		err := writeFileAtomic(s.Path, []byte(dumpFrontmatter(meta, body)), 0o644)
		writeMu.Unlock()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/spec/"+id, http.StatusSeeOther)
	})
	return mux
}

func cmdWeb(out, errOut io.Writer, host string, port int) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	if port == 0 {
		port = cfg.WebPort
	}
	if host == "" {
		host = cfg.WebHost
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	fmt.Fprintf(out, "Serving on http://%s:%d\n", host, port)
	return http.ListenAndServe(addr, webHandler(st, cfg, webToken()))
}
