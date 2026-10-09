package appserve

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/holistics/anfra/shared/jsonkit"
)

// repo is a repo with Data Apps: two at the top, one in a folder, plus what the tree leaves out.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"apps/sales.html":           "<html><head><title> Sales &amp; Returns </title></head></html>",
		"apps/untitled.html":        "<html></html>",
		"apps/team/overview.html":   "<title>Team</title>",
		"apps/team/logo.png":        "png",
		"apps/empty/notes.txt":      "not a Data App",
		"apps/.hidden/private.html": "<title>Private</title>",
		"apps/.env":                 "HIDDEN=1",
		"models/orders.model.aml":   "Model orders {}",
		".anfra/data_sources.yml":   "data_sources: {}",
	}
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// The tree: folders first, then Data Apps, each alphabetical; a Data App labelled by its <title>,
// else its file name; folders without Data Apps, and dot-entries, left out.
func TestCatalog(t *testing.T) {
	got, _ := jsonkit.Marshal(Catalog(repo(t)))
	want := `[{"kind":"folder","path":"team","name":"team","children":[{"kind":"app","path":"team/overview.html","label":"Team"}]},` +
		`{"kind":"app","path":"sales.html","label":"Sales & Returns"},{"kind":"app","path":"untitled.html","label":"untitled.html"}]`
	if string(got) != want {
		t.Errorf("catalog =\n  %s\nwant\n  %s", got, want)
	}
	if got := Catalog(t.TempDir()); got == nil || len(got) != 0 {
		t.Errorf("no apps/: %#v, want an empty tree", got)
	}
}

// A file under apps/ is served sandboxed, never sniffed; nothing outside apps/, nothing hidden.
func TestFiles(t *testing.T) {
	dir := repo(t)
	if err := os.Symlink(filepath.Join(dir, ".anfra", "data_sources.yml"), filepath.Join(dir, "apps", "leak.html")); err != nil {
		t.Fatal(err)
	}
	s := New(Options{RepoDir: dir})

	w := get(s, "/appserve/files/team/overview.html")
	if w.Code != http.StatusOK || w.Body.String() != "<title>Team</title>" {
		t.Fatalf("a definition: %d %q", w.Code, w.Body)
	}
	if csp := w.Header().Get("Content-Security-Policy"); csp != "sandbox allow-scripts" {
		t.Errorf("CSP %q: opened on its own, a definition must not run as the API's origin", csp)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("not nosniff")
	}
	if w := get(s, "/appserve/files/team/logo.png"); w.Code != http.StatusOK {
		t.Errorf("a file a definition refers to: %d", w.Code)
	}
	// A repo reached through a symlink (macOS's /var, say) serves its files all the same.
	link := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if w := get(New(Options{RepoDir: link}), "/appserve/files/sales.html"); w.Code != http.StatusOK {
		t.Errorf("through a symlinked repo: %d", w.Code)
	}
	for _, path := range []string{
		"/appserve/files/../.anfra/data_sources.yml",
		"/appserve/files/%2e%2e/.anfra/data_sources.yml",
		"/appserve/files/.env",
		"/appserve/files/.hidden/private.html",
		"/appserve/files/leak.html",
		"/appserve/files/team",
		"/appserve/files/nosuch.html",
	} {
		if w := get(s, path); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, w.Code)
		}
	}
}

// Every page URL is the frontend's index.html, which no other site may frame; without a frontend
// built in, a page says how to build one, and the backend's routes still work.
func TestPages(t *testing.T) {
	built := fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html><title>Data Apps</title>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	}
	s := newServer(Options{RepoDir: repo(t)}, built)
	for _, path := range []string{"/", "/apps", "/apps/sales", "/apps/team/overview"} {
		w := get(s, path)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<title>Data Apps</title>") {
			t.Errorf("%s: %d %q", path, w.Code, w.Body)
		}
		if w.Header().Get("Content-Security-Policy") != "frame-ancestors 'none'" {
			t.Errorf("%s: may be framed", path)
		}
	}
	if w := get(s, "/appserve/assets/app-1.js"); w.Code != http.StatusOK || w.Body.String() != "console.log(1)" {
		t.Errorf("an asset: %d %q", w.Code, w.Body)
	}

	none := newServer(Options{RepoDir: repo(t)}, nil)
	if w := get(none, "/apps/sales"); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "pnpm build:web") {
		t.Errorf("no frontend: %d %q", w.Code, w.Body)
	}
	if w := get(none, "/appserve/apps"); w.Code != http.StatusOK {
		t.Errorf("no frontend, the backend: %d", w.Code)
	}
}

// Under a dev frontend, a page is the same page on the dev server; the backend's routes stay here.
func TestDevFrontend(t *testing.T) {
	s := newServer(Options{RepoDir: repo(t), DevFrontendURL: "http://127.0.0.1:5173/"}, nil)
	if !s.FrontendBuilt() {
		t.Error("a dev frontend: not built")
	}
	for path, want := range map[string]string{
		"/":                    "http://127.0.0.1:5173/",
		"/apps/team/sales?x=1": "http://127.0.0.1:5173/apps/team/sales?x=1",
	} {
		if w := get(s, path); w.Code != http.StatusTemporaryRedirect || w.Header().Get("Location") != want {
			t.Errorf("%s: %d %q", path, w.Code, w.Header().Get("Location"))
		}
	}
	if w := get(s, "/appserve/apps"); w.Code != http.StatusOK {
		t.Errorf("the backend: %d", w.Code)
	}

	// Only the path and query are the request's: its host never reaches the destination.
	r := httptest.NewRequest(http.MethodGet, "http://evil.example/apps/sales", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if loc := w.Header().Get("Location"); loc != "http://127.0.0.1:5173/apps/sales" {
		t.Errorf("another host's request: %q", loc)
	}
}

func TestContext(t *testing.T) {
	t.Setenv("TZ", "Asia/Ho_Chi_Minh")
	dir := repo(t)
	var got Context
	if err := jsonkit.UnmarshalRead(get(New(Options{RepoDir: dir, Watch: true}), "/appserve/context").Body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Repo.Name != filepath.Base(dir) || !got.Watch || got.Reader.Timezone != "Asia/Ho_Chi_Minh" || !got.Reader.Permissions.CanViewGeneratedSQL {
		t.Errorf("context: %+v", got)
	}
}

func TestUnknownRoutes(t *testing.T) {
	s := New(Options{RepoDir: repo(t)})
	for _, path := range []string{"/appserve/nosuch", "/appserve/events"} { // events: no live reload
		if w := get(s, path); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/appserve/apps", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("POST: %d, want 404", w.Code)
	}
}

// An event stream carries what is published, as Server-Sent Events, and ends on Close.
func TestEvents(t *testing.T) {
	s := New(Options{RepoDir: repo(t), Watch: true})
	srv := httptest.NewServer(s)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/appserve/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	lines := bufio.NewScanner(resp.Body)
	lines.Scan() // ": connected"

	// The stream is registered once the comment is out; publish until it is heard.
	go func() {
		for range 20 {
			s.events.publish(Event{Type: "apps", Paths: []string{"sales.html"}})
			time.Sleep(20 * time.Millisecond)
		}
	}()
	for lines.Scan() {
		if line := lines.Text(); strings.HasPrefix(line, "data: ") {
			if line != `data: {"type":"apps","paths":["sales.html"]}` {
				t.Errorf("event %q", line)
			}
			break
		}
	}

	done := make(chan struct{})
	go func() {
		for lines.Scan() { //nolint:revive // drain until the stream ends
		}
		close(done)
	}()
	s.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("the stream outlived Close")
	}
}

func TestClassify(t *testing.T) {
	for rel, want := range map[string]struct {
		app     string
		aml, ok bool
	}{
		"apps/sales.html":         {"sales.html", false, true},
		"apps":                    {"", false, true},
		"models/orders.model.aml": {"", true, true},
		".anfra/data_sources.yml": {"", false, false},
		"node_modules/x/y.aml":    {"", false, false},
		"apps/.hidden/x.html":     {"", false, false},
		"README.md":               {"", false, false},
	} {
		app, aml, ok := classify(rel)
		if app != want.app || aml != want.aml || ok != want.ok {
			t.Errorf("%s: (%q, %v, %v), want %+v", rel, app, aml, ok, want)
		}
	}
}

// Watching a repo: a burst of changes is one event of each kind; a directory made later is
// watched too.
func TestWatch(t *testing.T) {
	dir := repo(t)
	s := New(Options{RepoDir: dir, Watch: true})
	got := make(chan Event, 8)
	ch := make(chan Event, 8)
	s.events.mu.Lock()
	s.events.subs[ch] = struct{}{}
	s.events.mu.Unlock()
	go func() {
		for e := range ch {
			got <- e
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.watch(ctx); err != nil {
		t.Fatal(err)
	}

	write := func(name, content string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("apps/sales.html", "<title>Sales 2</title>")
	write("apps/sales.html", "<title>Sales 3</title>")
	write("models/orders.model.aml", "Model orders { }")

	seen := map[string]Event{}
	deadline := time.After(3 * time.Second)
	for len(seen) < 2 {
		select {
		case e := <-got:
			seen[e.Type] = e
		case <-deadline:
			t.Fatalf("events: %+v", seen)
		}
	}
	if p := seen["apps"].Paths; len(p) != 1 || p[0] != "sales.html" {
		t.Errorf("apps event: %+v", seen["apps"])
	}

	write("apps/new/later.html", "<title>Later</title>")
	time.Sleep(300 * time.Millisecond) // the new directory is watched once its Create is seen
	write("apps/new/later.html", "<title>Later 2</title>")
	deadline = time.After(3 * time.Second)
	for {
		select {
		case e := <-got:
			for _, p := range e.Paths {
				if p == "new/later.html" {
					return
				}
			}
		case <-deadline:
			t.Fatal("a change in a directory made later was missed")
		}
	}
}

// Live reload watches the repo only while a Data App is open: the first event stream starts the
// watcher, which hears a change, and the last one to close stops it, as Close does on shutdown.
func TestLiveReloadWatchesOnlyWhileAStreamIsOpen(t *testing.T) {
	dir := repo(t)
	s := New(Options{RepoDir: dir, Watch: true})
	srv := httptest.NewServer(s)
	defer srv.Close()

	watching := func() bool {
		s.live.mu.Lock()
		defer s.live.mu.Unlock()
		return s.live.stop != nil
	}
	eventually := func(want bool, what string) {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); watching() != want; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
		}
	}
	if watching() {
		t.Fatal("watching with no stream open")
	}

	streamCtx, closeStream := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/appserve/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	eventually(true, "a stream opened, and nothing is watched")

	lines := bufio.NewScanner(resp.Body)
	lines.Scan() // ": connected"
	go func() {
		for range 20 {
			_ = os.WriteFile(filepath.Join(dir, "apps", "sales.html"), []byte("<title>Sales 2</title>"), 0o600)
			time.Sleep(50 * time.Millisecond)
		}
	}()
	for lines.Scan() {
		if line := lines.Text(); strings.HasPrefix(line, "data: ") {
			if line != `data: {"type":"apps","paths":["sales.html"]}` {
				t.Errorf("event %q", line)
			}
			break
		}
	}

	closeStream()
	eventually(false, "the last stream closed, and the repo is still watched")

	// On shutdown, Close ends the streams still open, and with them the watcher.
	again, err := http.Get(srv.URL + "/appserve/events")
	if err != nil {
		t.Fatal(err)
	}
	defer again.Body.Close()
	eventually(true, "a stream opened again, and nothing is watched")
	s.Close()
	eventually(false, "Close ended the streams, and the repo is still watched")
}
