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
	"os"
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
footer{margin-top:2em;font-size:.85em;color:var(--text-muted,#777)}</style></head><body>
<nav><span class="proj">%s</span><a href="/">specs</a><a href="/new">new spec</a><a href="/readme">readme</a><a href="/agents">agents</a><a href="/help">help</a></nav>%s
<footer>spec %s</footer></body></html>`,
		html.EscapeString(projName), html.EscapeString(title), accent, accent,
		accent, html.EscapeString(projName), body, html.EscapeString(version))
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

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// group specs by status, columns ordered by configured statuses
		byStatus := map[string][]Spec{}
		var extra []string // statuses present but not in cfg.Statuses
		for _, s := range st.Specs() {
			byStatus[s.Status()] = append(byStatus[s.Status()], s)
		}
		var cols []string
		inCfg := map[string]bool{}
		for _, s := range statuses {
			inCfg[s] = true
			cols = append(cols, s)
		}
		for s := range byStatus {
			if !inCfg[s] {
				extra = append(extra, s)
			}
		}
		sort.Strings(extra)
		cols = append(cols, extra...)

		var b strings.Builder
		b.WriteString(`<h1>Specifications</h1><div class="board">`)
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
		b.WriteString("</div>")
		render(w, "spec", b.String())
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
		for _, t := range []string{"project", "feature", "adr"} {
			fmt.Fprintf(&b, `<option value="%s">%s</option>`, t, t)
		}
		b.WriteString(`</select> <input name="title" placeholder="Title" size="50" required>
<button>Create</button></p></form>`)
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
		fmt.Fprintf(&b, "<h1>%s — %s</h1><p>type: %s · status: %s · <a href=\"/edit/%s\">edit</a></p>",
			html.EscapeString(s.ID()), html.EscapeString(s.Title()),
			html.EscapeString(s.Type()), html.EscapeString(s.Status()),
			s.ID())
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
<p>type: <select name="type">`, s.ID(), html.EscapeString(s.Title()), s.ID(), token)
		for _, t := range []string{"project", "feature", "adr", "imported"} {
			sel := ""
			if s.Type() == t {
				sel = " selected"
			}
			fmt.Fprintf(&b, `<option%s>%s</option>`, sel, t)
		}
		b.WriteString(`</select> status: <select name="status">`)
		for _, status := range statuses {
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
		specType := r.FormValue("type")
		status := r.FormValue("status")
		body := r.FormValue("body")
		validType := specType == "imported" || validTypes[specType]
		validStatus := cfg.validStatus(status)
		if !validType || !validStatus {
			renderEdit(w, *s, "invalid type or status")
			return
		}
		// section check: same required sections validate enforces
		updated := Spec{Path: s.Path, Meta: s.Meta, Body: body}
		for _, sec := range requiredSections[specType] {
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
		meta["type"] = []string{specType}
		writeMu.Lock()
		tmp := s.Path + ".tmp"
		err := os.WriteFile(tmp, []byte(dumpFrontmatter(meta, body)), 0o644)
		if err == nil {
			err = replaceFile(tmp, s.Path)
		}
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
