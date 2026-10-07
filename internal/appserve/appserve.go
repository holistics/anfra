// Package appserve is anfra serve's Data App component, its backend: the pages a browser opens
// (/apps/<path>, the appserve frontend), the Data Apps under the repo's apps/ (their tree, and the
// files themselves), who reads them, and live reload. It reaches no data: a Data App's queries go
// to the core API, from the browser.
package appserve

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
)

// Options configure a Server.
type Options struct {
	// RepoDir is the repo whose Data Apps are served, under its apps/.
	RepoDir string
	// Watch turns live reload on: the repo is watched, and a change is an event.
	Watch  bool
	Logger *slog.Logger
}

// Server serves a repo's Data Apps. Mount it for /, /apps/ and /appserve/.
type Server struct {
	opts     Options
	frontend http.Handler // nil: this binary was built without the appserve frontend
	events   *broadcaster
}

// New is a Server for opts, with the frontend built into this binary. With opts.Watch, Watch
// starts live reload.
func New(opts Options) *Server {
	built, _ := frontend()
	return newServer(opts, built)
}

func newServer(opts Options, built fs.FS) *Server {
	s := &Server{opts: opts, events: newBroadcaster()}
	if built != nil {
		s.frontend = http.FileServerFS(built)
	}
	return s
}

// FrontendBuilt reports whether this binary carries the appserve frontend: without it, the pages
// say how to build it, and the backend's routes still work.
func (s *Server) FrontendBuilt() bool { return s.frontend != nil }

// Close ends every event stream, so a server can shut down without waiting on them.
func (s *Server) Close() { s.events.closeAll() }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case r.Method != http.MethodGet && r.Method != http.MethodHead:
		httpkit.WriteError(w, r, apperr.New(apperr.NotFound, "No such endpoint."))
	case p == "/" || p == "/apps" || strings.HasPrefix(p, "/apps/"):
		s.page(w, r)
	case strings.HasPrefix(p, "/appserve/assets/"):
		s.asset(w, r)
	case p == "/appserve/apps":
		writeJSON(w, Catalog(s.opts.RepoDir))
	case p == "/appserve/context":
		writeJSON(w, Context{Repo: RepoInfo{Name: filepath.Base(s.opts.RepoDir)}, Reader: LocalReader(), Watch: s.opts.Watch})
	case p == "/appserve/events" && s.opts.Watch:
		s.events.serve(w, r)
	case strings.HasPrefix(p, "/appserve/files/"):
		s.file(w, r, strings.TrimPrefix(p, "/appserve/files/"))
	default:
		httpkit.WriteError(w, r, apperr.New(apperr.NotFound, "No such endpoint."))
	}
}

// Context is what the frontend starts from: the repo, who reads it, and whether it live-reloads.
type Context struct {
	Repo   RepoInfo `json:"repo"`
	Reader Reader   `json:"reader"`
	Watch  bool     `json:"watch"`
}

type RepoInfo struct {
	Name string `json:"name"`
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(body)
}
