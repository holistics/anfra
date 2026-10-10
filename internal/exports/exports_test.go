package exports

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
)

// newFiles is exports' files in a temp directory of the test's own.
func newFiles(t *testing.T, ttl time.Duration) *Files {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	fs := New(ttl)
	t.Cleanup(func() { _ = fs.Close() })
	return fs
}

// finish writes body as an export in s, and answers its link.
func finish(t *testing.T, s command.ExportStore, body string) string {
	t.Helper()
	e, err := s.Create(context.Background(), "sales.csv", "text/csv; charset=utf-8; header=present")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(e, body); err != nil {
		t.Fatal(err)
	}
	link, _, err := e.Done(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return link
}

// A link under a server's URL names its export by a long token; while the link
// lives, Open is its file, its name and its content type. An unknown token is
// not one.
func TestServedLink(t *testing.T) {
	fs := newFiles(t, time.Hour)
	link := finish(t, Store{Files: fs, ServedAt: "http://127.0.0.1:7878/exports/download"}, "a,b\n1,2\n")
	rest, ok := strings.CutPrefix(link, "http://127.0.0.1:7878/exports/download/")
	token, name, _ := strings.Cut(rest, "/")
	if !ok || len(token) < 20 || name != "sales.csv" {
		t.Fatalf("link = %s, want one below /exports/download/: a long token, then the name", link)
	}
	f, ok := fs.Open(token)
	if !ok {
		t.Fatal("the link's export does not open")
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	if string(b) != "a,b\n1,2\n" || f.Name != "sales.csv" || f.ContentType != "text/csv; charset=utf-8; header=present" {
		t.Errorf("opened %q %q %q", b, f.Name, f.ContentType)
	}
	if _, ok := fs.Open("nosuch"); ok {
		t.Error("an unknown token opened")
	}
}

// Without a server, a link is the file's own.
func TestFileLink(t *testing.T) {
	fs := newFiles(t, time.Hour)
	link := finish(t, Store{Files: fs}, "x\n")
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "file" {
		t.Fatalf("link = %s, want file://", link)
	}
	if b, err := os.ReadFile(u.Path); err != nil || string(b) != "x\n" {
		t.Errorf("the file at the link: %q %v", b, err)
	}
	if !strings.HasPrefix(u.Path, Dir()) {
		t.Errorf("the file is at %s, outside %s", u.Path, Dir())
	}
}

// An expired link does not open, and a sweep removes its file; a live one
// stays, even when its file looks older than a link's life (the clock jumped),
// since this process knows its expiry.
func TestExpiry(t *testing.T) {
	fs := newFiles(t, time.Hour)
	u, _ := url.Parse(finish(t, Store{Files: fs}, "x\n"))
	token := filepath.Base(u.Path)

	old := time.Now().Add(-3 * time.Hour)
	_ = os.Chtimes(u.Path, old, old)
	fs.Sweep(time.Now())
	if _, ok := fs.Open(token); !ok {
		t.Fatal("a live link was swept for its file's time")
	}

	fs.Sweep(time.Now().Add(2 * time.Hour))
	if _, err := os.Stat(u.Path); !os.IsNotExist(err) {
		t.Error("a sweep left an expired export's file")
	}
	if _, ok := fs.Open(token); ok {
		t.Error("an expired export opened")
	}
}

// A server sweeps on a ticker, until it stops.
func TestSweepEvery(t *testing.T) {
	fs := newFiles(t, time.Millisecond)
	u, _ := url.Parse(finish(t, Store{Files: fs}, "x\n"))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { fs.SweepEvery(ctx, 5*time.Millisecond); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(u.Path); os.IsNotExist(err) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no sweep removed the expired export")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
}

// The folder is the user's own, so one user's never keeps another's out.
func TestDirIsPerUser(t *testing.T) {
	if uid := os.Getuid(); uid >= 0 && !strings.HasSuffix(Dir(), "anfra-exports-"+strconv.Itoa(uid)) {
		t.Errorf("Dir() = %s, want the user's own", Dir())
	}
}

// An aborted export leaves nothing; closing the store removes what it made.
func TestAbortAndClose(t *testing.T) {
	fs := newFiles(t, time.Hour)
	e, err := Store{Files: fs}.Create(context.Background(), "a.csv", "text/csv")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(e, "partial")
	if err := e.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := entries(t); n != 0 {
		t.Errorf("%d exports left after an abort", n)
	}

	finish(t, Store{Files: fs}, "x\n")
	finish(t, Store{Files: fs}, "y\n")
	if err := fs.Close(); err != nil {
		t.Fatal(err)
	}
	if n := entries(t); n != 0 {
		t.Errorf("%d exports left after Close", n)
	}
}

// A new store removes what an earlier process left past its links' life, and
// nothing a live link may still reach.
func TestSweep(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, age := range map[string]time.Duration{"old": 3 * time.Hour, "fresh": 10 * time.Minute} {
		file := filepath.Join(Dir(), name)
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		then := time.Now().Add(-age)
		_ = os.Chtimes(file, then, then)
	}
	fs := New(time.Hour)
	defer fs.Close()
	finish(t, Store{Files: fs}, "x\n") // the first export sweeps
	if _, err := os.Stat(filepath.Join(Dir(), "old")); !os.IsNotExist(err) {
		t.Error("an export past its life was left")
	}
	if _, err := os.Stat(filepath.Join(Dir(), "fresh")); err != nil {
		t.Errorf("an export a link may still reach was removed: %v", err)
	}
}

func entries(t *testing.T) int {
	t.Helper()
	es, err := os.ReadDir(Dir())
	if err != nil {
		t.Fatal(err)
	}
	return len(es)
}

// A name is kept as given, in any script, and never touches the disk: whatever
// it holds, the file is the token's.
func TestFilenameIsNotOnDisk(t *testing.T) {
	fs := newFiles(t, time.Hour)
	e, err := Store{Files: fs, ServedAt: "http://127.0.0.1:7878/exports/download"}.Create(context.Background(), "Báo cáo/../q1.csv", "text/csv")
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := e.Done(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(link, "/B%C3%A1o%20c%C3%A1o%2F..%2Fq1.csv") {
		t.Errorf("link = %s, want the name as one escaped segment", link)
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(link, "http://127.0.0.1:7878/exports/download/"), "/")
	f, ok := fs.Open(token)
	if !ok {
		t.Fatal("the export does not open")
	}
	defer f.Close()
	if f.Name != "Báo cáo/../q1.csv" {
		t.Errorf("name = %q, want it as given", f.Name)
	}
	es, _ := os.ReadDir(Dir())
	if len(es) != 1 || es[0].Name() != token {
		t.Errorf("files on disk: %v, want the one, named by its token", es)
	}
}

// Making exports' files touches nothing on disk; the first export makes the
// folder.
func TestNewTouchesNothing(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	fs := New(time.Hour)
	defer fs.Close()
	if _, err := os.Stat(Dir()); !os.IsNotExist(err) {
		t.Error("New made the exports folder")
	}
	finish(t, Store{Files: fs}, "x\n")
	if _, err := os.Stat(Dir()); err != nil {
		t.Errorf("the first export made no folder: %v", err)
	}
}

// A folder removed while the process runs (a cleaner of old temp files) is made
// again by the next export.
func TestRemovedDirIsMadeAgain(t *testing.T) {
	fs := newFiles(t, time.Hour)
	finish(t, Store{Files: fs}, "x\n")
	if err := os.RemoveAll(Dir()); err != nil {
		t.Fatal(err)
	}
	finish(t, Store{Files: fs}, "y\n")
}
