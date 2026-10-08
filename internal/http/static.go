package api

import (
	"net/http"
	"os"
	"path/filepath"
)

func staticHandler() http.Handler {
	dist := filepath.Join("web", "dist")
	info, err := os.Stat(dist)
	if err != nil || !info.IsDir() {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}
	files := http.FileServer(http.Dir(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean("/" + r.URL.Path)
		target := filepath.Join(dist, clean)
		if st, err := os.Stat(target); err == nil && !st.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dist, "index.html"))
	})
}
