package appserve

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// The appserve frontend, built by Vite (`pnpm build:web`) into dist/ before `go build`. A build
// output, not committed: the placeholder keeps the pattern valid without it.
//
//go:embed all:dist
var dist embed.FS

// frontend is the built frontend, or false when this binary was built without it.
func frontend() (fs.FS, bool) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}

const notBuilt = `<!doctype html>
<meta charset="utf-8">
<title>Data Apps</title>
<p>This anfra was built without its Data App pages. From the anfra repository, run
<code>pnpm build:web</code>, then build anfra again.</p>
`

// page answers every page URL with the frontend's index.html: the frontend decides what the path
// names. No other site may frame it. Under a dev frontend, it redirects to the same page there.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if dev := s.opts.DevFrontendURL; dev != "" {
		http.Redirect(w, r, strings.TrimSuffix(dev, "/")+r.URL.RequestURI(), http.StatusTemporaryRedirect)
		return
	}
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.Header().Set("Cache-Control", "no-store")
	if s.frontend == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(notBuilt))
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	s.frontend.ServeHTTP(w, r2)
}

// asset serves the frontend's built files, below /appserve/ (Vite's base).
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	if s.frontend == nil {
		http.NotFound(w, r)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path = strings.TrimPrefix(r.URL.Path, "/appserve")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	s.frontend.ServeHTTP(w, r2)
}
