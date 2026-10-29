package main

import (
	"fmt"
	"html"
	"net/http"
	"path/filepath"
	"strings"
)

func cmdWeb(port int) error {
	st := mustStore()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><head><title>spec</title></head><body><h1>Specifications</h1><ul>")
		for _, s := range st.Specs() {
			rel, _ := filepath.Rel(st.Root, s.Path)
			fmt.Fprintf(w, `<li><a href="/spec/%s">%s</a> — %s (%s, %s)</li>`,
				s.ID(), html.EscapeString(s.ID()), html.EscapeString(s.Title()),
				html.EscapeString(s.Type()), html.EscapeString(s.Status()))
			_ = rel
		}
		fmt.Fprint(w, "</ul></body></html>")
	})
	mux.HandleFunc("/spec/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/spec/")
		s := st.Get(id)
		if s == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><head><title>%s</title></head><body>", html.EscapeString(s.ID()))
		fmt.Fprintf(w, "<h1>%s — %s</h1><p>type: %s · status: %s</p><pre>%s</pre></body></html>",
			html.EscapeString(s.ID()), html.EscapeString(s.Title()),
			html.EscapeString(s.Type()), html.EscapeString(s.Status()),
			html.EscapeString(s.Body))
	})
	addr := fmt.Sprintf(":%d", port)
	fmt.Println("Serving on http://localhost" + addr)
	return http.ListenAndServe(addr, mux)
}
