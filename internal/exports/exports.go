// Package exports is anfra's own command.ExportStore: an export's file in the
// system's temp directory, reached by a link that expires. `anfra serve` serves
// its links over HTTP (Open); the CLI without a server reads its file directly.
// A platform brings its own store. See docs/designs/export-store.md.
package exports

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/holistics/anfra/internal/command"
)

// Files are exports' files, one per export, named by a token no one can guess,
// each kept until its link expires and a sweep removes it (Sweep). A process
// has one; each Store writes to it. The name a download saves a file as is kept
// beside its link, never on disk, so it is never a path to check.
type Files struct {
	ttl   time.Duration
	swept sync.Once // at the first export

	mu    sync.Mutex
	links map[string]link // token -> a finished export
}

type link struct {
	path, filename, contentType string
	expiresAt                   time.Time
}

// Dir is where exports' files go: a folder in the system's temp directory, so
// TMPDIR moves it, shared by every anfra process of a user (a server per repo,
// CLI runs). It is the user's own (anfra-exports-<uid>), so on a machine others
// share, one user's folder never keeps another's exports out; on Windows, which
// has no uid and whose temp directory is a user's own, it is anfra-exports.
func Dir() string {
	name := "anfra-exports"
	if uid := os.Getuid(); uid >= 0 {
		name += "-" + strconv.Itoa(uid)
	}
	return filepath.Join(os.TempDir(), name)
}

// New is exports' files, whose links live for ttl. It touches nothing on disk:
// the first export makes Dir, and sweeps it.
func New(ttl time.Duration) *Files {
	return &Files{ttl: ttl, links: map[string]link{}}
}

// prepare makes Dir for an export, and sweeps it at the first. Dir is made
// every time, not once: something may remove it while a server runs (a cleaner
// of old files in the temp directory), and the next export makes it again.
func (fs *Files) prepare() error {
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return fmt.Errorf("create the exports folder %s: %w (set TMPDIR to put it elsewhere)", Dir(), err)
	}
	fs.swept.Do(func() { fs.Sweep(time.Now()) })
	return nil
}

// Sweep removes what no link can reach any more, as of now:
//   - this process's exports whose links have expired, judged by their own
//     expiry, so a jump of the system clock never removes a live one;
//   - any other file in Dir last written more than a link's life ago: another
//     process's (a server that crashed, a link the CLI printed), whose expiry
//     is not known here. A link's life starts when its file is finished, and an
//     export is shorter than a link's life (its timeout), so the file of an
//     export still being written is never that old.
func (fs *Files) Sweep(now time.Time) {
	fs.mu.Lock()
	var expired []string
	for token, l := range fs.links {
		if now.After(l.expiresAt) {
			expired = append(expired, token)
		}
	}
	fs.mu.Unlock()
	for _, token := range expired {
		fs.remove(token)
	}

	entries, err := os.ReadDir(Dir())
	if err != nil {
		return
	}
	cutoff := now.Add(-fs.ttl)
	for _, e := range entries {
		fs.mu.Lock()
		_, ours := fs.links[e.Name()]
		fs.mu.Unlock()
		if info, err := e.Info(); err == nil && !ours && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(Dir(), e.Name()))
		}
	}
}

// SweepEvery sweeps every interval until ctx ends: a server's, which outlives
// many links.
func (fs *Files) SweepEvery(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			fs.Sweep(now)
		}
	}
}

// Store is the command.ExportStore of exports written to Files, linked where
// they are served: below ServedAt, its token then its name, when a server
// serves them (Open; the name is for a client that saves a file under a link's
// last part, as curl -O does, and decides nothing); to the files themselves,
// file:// links, when ServedAt is "".
type Store struct {
	Files    *Files
	ServedAt string
}

// Create starts an export's file, named by a new token.
func (s Store) Create(_ context.Context, filename, contentType string) (command.Export, error) {
	if err := s.Files.prepare(); err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(Dir(), token), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create the export's file: %w (set TMPDIR to put exports elsewhere)", err)
	}
	return &export{File: f, store: s, token: token, filename: filename, contentType: contentType}, nil
}

type export struct {
	*os.File
	store                        Store
	token, filename, contentType string
}

// Done closes the file and makes its link, which a sweep removes once it has
// expired.
func (e *export) Done(context.Context) (string, time.Time, error) {
	if err := e.Close(); err != nil {
		_ = os.Remove(e.Name())
		return "", time.Time{}, err
	}
	fs := e.store.Files
	expiresAt := time.Now().Add(fs.ttl)
	fs.mu.Lock()
	fs.links[e.token] = link{path: e.Name(), filename: e.filename, contentType: e.contentType, expiresAt: expiresAt}
	fs.mu.Unlock()
	if e.store.ServedAt == "" {
		return (&url.URL{Scheme: "file", Path: e.Name()}).String(), expiresAt, nil
	}
	return e.store.ServedAt + "/" + e.token + "/" + url.PathEscape(e.filename), expiresAt, nil
}

// Abort discards the file.
func (e *export) Abort(context.Context) error {
	_ = e.Close()
	return os.Remove(e.Name())
}

// File is a finished export's file, opened to serve: its content, the name a
// download saves it as, and its content type.
type File struct {
	*os.File
	Name, ContentType string
}

// Open is the file of the export token names, while its link lives. ok is false
// for a token unknown here, or a link that has expired: the same to a caller,
// since a token is all it takes to have the file.
func (fs *Files) Open(token string) (f File, ok bool) {
	fs.mu.Lock()
	l, ok := fs.links[token]
	fs.mu.Unlock()
	if !ok || time.Now().After(l.expiresAt) {
		return File{}, false
	}
	file, err := os.Open(l.path)
	if err != nil {
		return File{}, false // removed as it expired
	}
	return File{File: file, Name: l.filename, ContentType: l.contentType}, true
}

// remove forgets an export's link and removes its file.
func (fs *Files) remove(token string) {
	fs.mu.Lock()
	l, ok := fs.links[token]
	delete(fs.links, token)
	fs.mu.Unlock()
	if ok {
		_ = os.Remove(l.path)
	}
}

// Close removes every export written to fs, when a server stops. The CLI does
// not: it removes the file it writes out, and leaves one whose link it printed
// for its link's life.
func (fs *Files) Close() error {
	fs.mu.Lock()
	tokens := make([]string, 0, len(fs.links))
	for t := range fs.links {
		tokens = append(tokens, t)
	}
	fs.mu.Unlock()
	for _, t := range tokens {
		fs.remove(t)
	}
	return nil
}

// newToken is 128 random bits, URL-safe: a link no one can guess.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("make an export's token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
