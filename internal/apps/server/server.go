// Package server is the demo's HTTP surface: the Shell, its API, Data Apps, and live events.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/holistics/anfra/internal/apps/assets"
	"github.com/holistics/anfra/internal/apps/dataapps"
	"github.com/holistics/anfra/internal/apps/descriptors"
	"github.com/holistics/anfra/internal/apps/dispatch"
	"github.com/holistics/anfra/internal/apps/queries"
	"github.com/holistics/anfra/internal/apps/repofiles"
)

// Server holds the demo's state: anfra, the Data Folder, its Datasets and AML problems, and the
// Shells listening for events.
type Server struct {
	Anfra      dispatch.Caller
	DataFolder string
	SDKBundle  string
	Shell      fs.FS

	mu       sync.RWMutex
	datasets map[string]descriptors.Dataset
	problems []queries.Problem

	subsMu sync.Mutex
	subs   map[chan string]struct{}

	rebuild sync.Mutex
}

// New builds the Server's starting state: the Datasets and the AML's problems.
func New(ctx context.Context, a dispatch.Caller, dataFolder, sdkBundle string, shell fs.FS) (*Server, error) {
	s := &Server{Anfra: a, DataFolder: dataFolder, SDKBundle: sdkBundle, Shell: shell, subs: map[chan string]struct{}{}}
	s.revalidate(ctx)
	datasets, err := descriptors.Build(ctx, a)
	if err != nil {
		return nil, fmt.Errorf("Couldn't read the Data Folder's datasets from anfra: %v", err)
	}
	s.datasets = datasets
	return s, nil
}

func (s *Server) broadcast(event any) {
	b, _ := json.Marshal(event)
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- string(b):
		default: // a stalled listener misses an event rather than blocking everyone
		}
	}
}

func (s *Server) revalidate(ctx context.Context) {
	problems, err := queries.Validate(ctx, s.Anfra)
	if err != nil {
		problems = []queries.Problem{{Message: fmt.Sprintf("anfra couldn't validate the AML: %v", err)}}
	}
	s.mu.Lock()
	s.problems = problems
	s.mu.Unlock()
	s.broadcast(map[string]any{"type": "validation", "problems": problems})
}

// OnChange reacts to the Data Folder changing: Data Apps refresh the Shell's tree (and reload the
// running one); AML re-validates and rebuilds the Datasets, one rebuild at a time.
func (s *Server) OnChange(change repofiles.Change) {
	if len(change.Apps) > 0 {
		s.broadcast(map[string]any{"type": "apps", "paths": change.Apps})
	}
	if !change.AML {
		return
	}
	go func() {
		s.rebuild.Lock()
		defer s.rebuild.Unlock()
		ctx := context.Background()
		s.revalidate(ctx)
		datasets, err := descriptors.Build(ctx, s.Anfra)
		if err != nil {
			// Keep serving the last good descriptors; the banner says why the AML is broken.
			log.Printf("anfra-demo: couldn't rebuild the datasets: %v", err)
			return
		}
		s.mu.Lock()
		s.datasets = datasets
		s.mu.Unlock()
		s.broadcast(map[string]any{"type": "datasets"})
	}()
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func queryError(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]string{"name": "QueryError", "message": message}})
}

// ServeHTTP routes by hand rather than with ServeMux, which cleans `..` out of a path by
// redirecting, and would turn /_anfra/data-apps/..%2F… into a Shell page instead of a 404.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	// Everything the demo owns is under the reserved namespace; any other path is a Data App URL, for the Shell.
	const reserved = "/" + repofiles.ReservedName
	if p == reserved || strings.HasPrefix(p, reserved+"/") {
		s.internal(w, r, strings.TrimPrefix(p, reserved))
		return
	}
	s.shell(w, r)
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, p string) {
	switch {
	case p == "/api/apps" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, repofiles.Catalog(s.DataFolder))
	case p == "/api/status" && r.Method == http.MethodGet:
		status := "down"
		if s.Anfra.Healthy(r.Context()) {
			status = "up"
		}
		s.mu.RLock()
		problems := s.problems
		s.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"anfra": status, "problems": problems, "dataFolder": filepath.Base(s.DataFolder)})
	case p == "/api/events" && r.Method == http.MethodGet:
		s.events(w, r)
	case p == "/api/query" && r.Method == http.MethodPost:
		s.query(w, r)
	case p == "/api/suggestions" && r.Method == http.MethodPost:
		s.suggestions(w, r)
	case strings.HasPrefix(p, "/data-apps/") && r.Method == http.MethodGet:
		s.dataApp(w, r, strings.TrimPrefix(p, "/data-apps/"))
	case strings.HasPrefix(p, "/api/"):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("No route for %s %s", r.Method, p)})
	default:
		s.shellFile(w, r, strings.TrimPrefix(p, "/"))
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ch := make(chan string, 16)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	defer func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
	}
}

// Close ends every event stream, so the server can shut down.
func (s *Server) Close() {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		delete(s.subs, ch)
	}
}

func (s *Server) query(w http.ResponseWriter, r *http.Request) {
	var req queries.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		queryError(w, "The query request isn't valid JSON.")
		return
	}
	// r.Context() ends when the Shell cancels the request, which cancels the anfra call too.
	result, err := queries.Run(r.Context(), s.Anfra, req)
	s.answer(w, result, err)
}

func (s *Server) suggestions(w http.ResponseWriter, r *http.Request) {
	var req queries.SuggestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		queryError(w, "The suggestions request isn't valid JSON.")
		return
	}
	s.mu.RLock()
	fieldType := descriptors.FieldType(s.datasets, req.Dataset, req.Model, req.Field)
	s.mu.RUnlock()
	// The field is spliced into AQL, so only a field the Dataset defines is accepted.
	if fieldType == "" {
		queryError(w, fmt.Sprintf("Unknown field %s.%s in dataset %s.", req.Model, req.Field, req.Dataset))
		return
	}
	values, err := queries.Suggest(r.Context(), s.Anfra, req, fieldType)
	s.answer(w, values, err)
}

func (s *Server) answer(w http.ResponseWriter, result any, err error) {
	var failure *queries.Failure
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, result)
	case errors.As(err, &failure):
		queryError(w, failure.Message)
	case dispatch.IsAbort(err):
		// The client is gone; there is no one to answer.
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func (s *Server) dataApp(w http.ResponseWriter, r *http.Request, appPath string) {
	file := repofiles.AppFile(s.DataFolder, appPath)
	if file == "" {
		http.Error(w, "No such Data App.", http.StatusNotFound)
		return
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		http.Error(w, "No such Data App.", http.StatusNotFound)
		return
	}
	s.mu.RLock()
	datasets := s.datasets
	s.mu.RUnlock()
	html := dataapps.Provision(string(raw), dataapps.Options{
		SDKBundle:   s.SDKBundle,
		Bootstrap:   assets.FrameBootstrap,
		Datasets:    datasets,
		ShellOrigin: shellOrigin(r),
	})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(html))
}

// shellOrigin is the origin the browser loaded the Shell from. Behind a TLS-terminating proxy the
// request arrives over plain HTTP, so the proxy's X-Forwarded-Proto wins over the connection's own scheme.
func shellOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(proto, ",")[0]))
	}
	return scheme + "://" + r.Host
}

// shell serves the Shell's page for a Data App URL. The Shell decides whether the URL names a Data App.
func (s *Server) shell(w http.ResponseWriter, r *http.Request) {
	s.shellFile(w, r, "index.html")
}

// shellFile serves a file of the built Shell, or a 404 when there is none. Nothing the Shell serves
// may be framed by another page.
func (s *Server) shellFile(w http.ResponseWriter, r *http.Request, name string) {
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if info, err := fs.Stat(s.Shell, name); err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	http.ServeFileFS(w, r, s.Shell, name)
}
