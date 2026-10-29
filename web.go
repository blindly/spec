package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func webToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func page(w http.ResponseWriter, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<html><head><title>%s</title>
<link rel="stylesheet" href="/assets/water/water.min.css">
<style>nav a{margin-right:1em}textarea{font-family:monospace}</style></head><body>
<nav><a href="/">specs</a><a href="/new">new spec</a></nav>%s</body></html>`,
		html.EscapeString(title), body)
}

//go:embed assets/easymde assets/fontawesome assets/water
var assetsFS embed.FS

// webHandler builds the mux; extracted from cmdWeb so tests can use httptest.
func webHandler(st Store, cfg Config, token string) http.Handler {
	statuses := cfg.Statuses
	var writeMu sync.Mutex

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
		var b strings.Builder
		b.WriteString("<h1>Specifications</h1><ul>")
		for _, s := range st.Specs() {
			rel, _ := filepath.Rel(st.Root, s.Path)
			fmt.Fprintf(&b, `<li><a href="/spec/%s">%s</a> — %s (%s, %s)</li>`,
				s.ID(), html.EscapeString(s.ID()), html.EscapeString(s.Title()),
				html.EscapeString(s.Type()), html.EscapeString(s.Status()))
			_ = rel
		}
		b.WriteString("</ul>")
		page(w, "spec", b.String())
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
				page(w, "error", "<p>Invalid type or empty title.</p>")
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
		page(w, "new spec", b.String())
	})

	mux.HandleFunc("/spec/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/spec/")
		s := st.Get(id)
		if s == nil {
			http.NotFound(w, r)
			return
		}
		var b strings.Builder
		fmt.Fprintf(&b, "<h1>%s — %s</h1><p>type: %s · status: %s · <a href=\"/edit/%s\">edit</a></p><pre>%s</pre>",
			html.EscapeString(s.ID()), html.EscapeString(s.Title()),
			html.EscapeString(s.Type()), html.EscapeString(s.Status()),
			s.ID(), html.EscapeString(s.Body))
		page(w, s.ID(), b.String())
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
<textarea id="body-editor" name="body" rows="30">%s</textarea>
<p><button>Save</button> <a href="/spec/%s">cancel</a></p></form>
<script src="/assets/easymde/easymde.min.js"></script>
<script>new EasyMDE({element: document.getElementById('body-editor'),
autoDownloadFontAwesome: false, spellChecker: false, status: ['lines','words','cursor']});</script>`,
			html.EscapeString(s.Body), s.ID())
		page(w, "edit "+s.ID(), b.String())
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
			err = os.Rename(tmp, s.Path)
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

func cmdWeb(out, errOut io.Writer, port int) error {
	st, err := mustStore()
	if err != nil {
		return err
	}
	cfg := loadConfig(st.Root)
	if port == 0 {
		port = cfg.WebPort
	}
	addr := fmt.Sprintf(":%d", port)
	fmt.Fprintln(out, "Serving on http://localhost"+addr)
	return http.ListenAndServe(addr, webHandler(st, cfg, webToken()))
}
