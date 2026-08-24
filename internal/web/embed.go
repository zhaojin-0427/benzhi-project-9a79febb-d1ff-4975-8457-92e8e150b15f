package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed workbench.html app.js style.css
var assets embed.FS

func Handler() http.Handler {
	static, _ := fs.Sub(assets, ".")
	files := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			r.URL.Path = "/workbench.html"
		}
		files.ServeHTTP(w, r)
	})
}
