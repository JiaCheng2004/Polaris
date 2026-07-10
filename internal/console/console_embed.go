//go:build console

package console

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// dist holds the built console SPA. `make build-console` compiles web/console and
// copies its dist/ here before `go build -tags console`. A committed placeholder
// index.html keeps this build compilable even without that copy.
//
//go:embed all:dist
var dist embed.FS

// Enabled reports whether this build embeds the console UI.
func Enabled() bool { return true }

// Handler serves the embedded SPA with client-side-routing fallback: known asset
// paths are served directly; anything else returns index.html so deep links work.
func Handler() (http.Handler, bool) {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	fileServer := http.FileServerFS(root)
	index, err := fs.ReadFile(root, "index.html")
	if err != nil {
		return nil, false
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path (collapses `//`, resolves `..`) before checking the FS.
		upath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if upath == "" || upath == "." {
			serveIndex(w, index)
			return
		}
		if _, statErr := fs.Stat(root, upath); statErr != nil {
			serveIndex(w, index) // unknown path → SPA entry point (client-side routing)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
	return h, true
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(index)
}
