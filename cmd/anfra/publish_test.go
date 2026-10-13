package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/remoteapi"
)

// write creates files under dir, path to content.
func write(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}
	cmd := exec.Command("git", append(base, args...)...) //nolint:gosec // G204: a test's own git, on its own folder
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

// A publish carries the repo's .aml files and Data Apps, wherever they are in
// the folder, and nothing else: not .anfra/ (credentials), not a .env, notes or
// build output beside the repo, not dot-folders nor node_modules. .gitignore
// plays no part, in git or not.
func TestSelectFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{
		"models/orders.aml":               "o",
		"datasets/sales.aml":              "d",
		"nested/deep/x.AML":               "x",
		"apps/dash/index.html":            "<html>",
		"apps/dash/data/rows.json":        "[]",
		"apps/dash/node_modules/lib/i.js": "js",
		".anfra/data_sources.yml":         "password: hunter2",
		".anfra/config.yml":               "remote:\n  url: https://bi.example.com/o/acme\n",
		".env":                            "TOKEN=x",
		".gitignore":                      "models/\n", // says nothing about publishing
		".git/config":                     "[core]",
		".hidden/draft.aml":               "h",
		"README.md":                       "readme",
		"build/out.js":                    "b",
	})
	got, leftOut, err := selectFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"apps/dash/data/rows.json", "apps/dash/index.html", "datasets/sales.aml", "models/orders.aml", "nested/deep/x.AML"}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v\nwant    %v", got, want)
	}
	// Counted: README.md, build/out.js, .gitignore and .env at the root; pruned
	// folders (.anfra, .git, .hidden, node_modules) are not walked.
	if leftOut != 4 {
		t.Errorf("left out %d, want 4", leftOut)
	}
}

// A publish's default message is HEAD's commit, in a git repo, marked when the
// repo has changes not in it; outside git, none.
func TestGitMessage(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	if got := gitMessage(context.Background(), dir); got != "" {
		t.Errorf("outside git: %q, want none", got)
	}
	write(t, dir, map[string]string{"a.aml": "a"})
	git(t, dir, "init", "-q")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "Add orders")
	if got := gitMessage(context.Background(), dir); !strings.HasSuffix(got, " Add orders") {
		t.Errorf("clean: %q, want HEAD's short id and subject", got)
	}
	write(t, dir, map[string]string{"a.aml": "a2"})
	if got := gitMessage(context.Background(), dir); !strings.HasSuffix(got, " Add orders (with uncommitted changes)") {
		t.Errorf("dirty: %q", got)
	}
}

// A symlink among the repo's files is refused by name, never skipped (which
// would delete it on the server) nor followed out of the repo; one elsewhere in
// the folder is no concern of a publish.
func TestSelectFilesRefusesSymlinks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a"})
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "models", "link.aml")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	_, _, err := selectFiles(dir)
	if err == nil || !strings.Contains(err.Error(), "models/link.aml (a symlink)") || strings.Contains(err.Error(), "notes.txt") {
		t.Errorf("err = %v, want the repo's symlink refused by name, and only it", err)
	}
}

// remote is a fake server speaking the publish protocol, in memory.
type remote struct {
	mu       sync.Mutex
	blobs    map[string][]byte
	deployed []remoteapi.File
	revision int
	by       *remoteapi.Person
	limits   remoteapi.Limits
	uploads  int  // create_blobs calls
	forget   bool // drop every blob once, as housekeeping would
	cookie   string
}

func (s *remote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, err := r.Cookie("anfra_session"); err == nil {
		s.cookie = c.Value
	}
	op, ok := strings.CutPrefix(r.URL.Path, "/api/o/acme/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	reply := func(v any) { w.Header().Set("Content-Type", "application/json"); _ = jsonkit.MarshalWrite(w, v) }
	rev := func() string { return "repo@" + strings.Repeat(string(rune('0'+s.revision)), 40) }
	switch op {
	case remoteapi.StatusOp.Name:
		reply(remoteapi.StatusOut{Revision: rev(), PublishedBy: s.by, PublishedAt: time.Now(), Publish: remoteapi.PublishSettings{Limits: s.limits}})
	case remoteapi.ListMissingBlobsOp.Name:
		var in remoteapi.ListMissingBlobsIn
		_ = jsonkit.UnmarshalRead(r.Body, &in)
		out := remoteapi.ListMissingBlobsOut{OIDs: []string{}}
		for _, oid := range in.OIDs {
			if _, ok := s.blobs[oid]; !ok {
				out.OIDs = append(out.OIDs, oid)
			}
		}
		reply(out)
	case remoteapi.CreateBlobsOp.Name:
		var in remoteapi.CreateBlobsIn
		_ = jsonkit.UnmarshalRead(r.Body, &in)
		s.uploads++
		out := remoteapi.CreateBlobsOut{}
		for _, b := range in.Blobs {
			s.blobs[remoteapi.BlobID(b)] = b
			out.OIDs = append(out.OIDs, remoteapi.BlobID(b))
		}
		reply(out)
	case remoteapi.PublishOp.Name:
		var in remoteapi.PublishIn
		_ = jsonkit.UnmarshalRead(r.Body, &in)
		if s.forget {
			s.blobs, s.forget = map[string][]byte{}, false
		}
		var missing []string
		for _, f := range in.Files {
			if _, ok := s.blobs[f.OID]; !ok {
				missing = append(missing, f.OID)
			}
		}
		if len(missing) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = jsonkit.MarshalWrite(w, apperr.Envelope{Error: apperr.Response{Code: "blobs_missing", Message: "m", Details: remoteapi.MissingBlobs{OIDs: missing}}})
			return
		}
		if slices.Equal(in.Files, s.deployed) {
			reply(remoteapi.PublishOut{Revision: rev()})
			return
		}
		replaced := &remoteapi.Replaced{Revision: rev(), PublishedBy: s.by}
		s.deployed, s.revision, s.by = in.Files, s.revision+1, &remoteapi.Person{ID: "u1", Email: "ada@example.com"}
		reply(remoteapi.PublishOut{Revision: rev(), Changed: true, Replaced: replaced})
	default:
		http.NotFound(w, r)
	}
}

func newRemote(t *testing.T) (*remote, string) {
	t.Helper()
	s := &remote{blobs: map[string][]byte{}, limits: remoteapi.Limits{Files: 100, UploadBlobs: 1000, UploadBytes: 1 << 20}}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv(sessionEnv, "dev-session")
	stubValidation(t, nil)
	return s, srv.URL + "/o/acme"
}

// stubValidation makes a publish's local validation answer err, without
// sidecars.
func stubValidation(t *testing.T, err error) {
	t.Helper()
	was := validateRepo
	validateRepo = func(context.Context, io.Writer) error { return err }
	t.Cleanup(func() { validateRepo = was })
}

// Files that fail validation here are not published: nothing is uploaded.
func TestPublishValidatesFirst(t *testing.T) {
	s, url := newRemote(t)
	stubValidation(t, errors.New("the repo has errors: fix them and publish again"))
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "Model {"})
	if _, _, err := run(t, dir, publishOptions{setRemote: url}, never(t)); err == nil || !strings.Contains(err.Error(), "has errors") {
		t.Errorf("err = %v, want the validation's refusal", err)
	}
	if s.uploads != 0 || s.deployed != nil {
		t.Errorf("%d uploads, deployed %v: want nothing sent", s.uploads, s.deployed)
	}
}

func run(t *testing.T, dir string, o publishOptions, ask prompt) (string, string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := publish(context.Background(), dir, o, &out, &errOut, ask)
	return out.String(), errOut.String(), err
}

func never(t *testing.T) prompt {
	return func(q string) (bool, bool) { t.Errorf("asked %q", q); return false, true }
}

// A publish uploads what the remote lacks, then deploys every file; publishing
// again uploads nothing and changes nothing; an edit uploads the one file.
func TestPublish(t *testing.T) {
	s, url := newRemote(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a", "models/b.aml": "b", "copy.aml": "a", ".anfra/data_sources.yml": "secret"})
	o := publishOptions{setRemote: url, message: "first"}

	out, _, err := run(t, dir, o, never(t))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Published repo@") || s.cookie != "dev-session" {
		t.Errorf("out = %q, cookie %q", out, s.cookie)
	}
	paths := func() (ps []string) {
		for _, f := range s.deployed {
			ps = append(ps, f.Path)
		}
		return ps
	}
	if want := []string{"copy.aml", "models/a.aml", "models/b.aml"}; !slices.Equal(paths(), want) || len(s.blobs) != 2 {
		t.Errorf("deployed %v with %d blobs; want %v, and the shared content once", paths(), len(s.blobs), want)
	}

	uploads := s.uploads
	if out, _, err = run(t, dir, o, never(t)); err != nil || !strings.Contains(out, "Nothing changed") || s.uploads != uploads {
		t.Errorf("again: %q, %v, %d uploads; want nothing changed, nothing uploaded", out, err, s.uploads-uploads)
	}

	write(t, dir, map[string]string{"models/b.aml": "b2"})
	if _, _, err = run(t, dir, o, never(t)); err != nil {
		t.Fatal(err)
	}
	if s.uploads != uploads+1 || len(s.blobs) != 3 {
		t.Errorf("an edit: %d upload calls, %d blobs; want one more call, one more blob", s.uploads-uploads, len(s.blobs))
	}
}

// Uploads are batched within the remote's limits, not the client's.
func TestPublishBatchesUploads(t *testing.T) {
	s, url := newRemote(t)
	s.limits.UploadBlobs = 2
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "1", "b.aml": "2", "c.aml": "3", "d.aml": "4", "e.aml": "5"})
	if _, _, err := run(t, dir, publishOptions{setRemote: url}, never(t)); err != nil {
		t.Fatal(err)
	}
	if s.uploads != 3 {
		t.Errorf("%d upload calls for 5 blobs at 2 a call, want 3", s.uploads)
	}
}

// Content collected between upload and publish is uploaded again, once.
func TestPublishUploadsAgainWhatWasCollected(t *testing.T) {
	s, url := newRemote(t)
	s.forget = true
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	if _, _, err := run(t, dir, publishOptions{setRemote: url}, never(t)); err != nil {
		t.Fatal(err)
	}
	if s.uploads != 2 || len(s.deployed) != 1 {
		t.Errorf("%d uploads, deployed %v; want the content uploaded again and published", s.uploads, s.deployed)
	}
}

// A deployed revision that changed since this folder last published is replaced
// only once confirmed: asked at a terminal, --yes without one. One this folder
// published is replaced without asking, as are the org's starting files.
func TestPublishConfirmsReplacingChanges(t *testing.T) {
	s, url := newRemote(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	o := publishOptions{setRemote: url}
	if _, _, err := run(t, dir, o, never(t)); err != nil {
		t.Fatalf("over the starting files: %v", err)
	}

	// Someone publishes elsewhere.
	s.by, s.revision = &remoteapi.Person{ID: "u2", Email: "bob@example.com"}, s.revision+1
	write(t, dir, map[string]string{"a.aml": "a2"})
	var asked string
	_, errOut, err := run(t, dir, o, func(q string) (bool, bool) { asked = q; return false, true })
	if err == nil || asked == "" || !strings.Contains(errOut, "bob@example.com") || !strings.Contains(errOut, "This repo was last published by bob@example.com") {
		t.Fatalf("declined: err %v, asked %q, said %q", err, asked, errOut)
	}
	if _, _, err := run(t, dir, o, func(string) (bool, bool) { return false, false }); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("without a terminal: %v, want a refusal naming --yes", err)
	}
	deployed := len(s.deployed)
	if _, _, err := run(t, dir, publishOptions{setRemote: url, yes: true}, never(t)); err != nil {
		t.Fatalf("--yes: %v", err)
	}
	if got := s.blobs[s.deployed[0].OID]; string(got) != "a2" || deployed != 1 {
		t.Errorf("deployed %q, want a2", got)
	}

	// Published from here: no question.
	write(t, dir, map[string]string{"a.aml": "a3"})
	if _, _, err := run(t, dir, o, never(t)); err != nil {
		t.Errorf("replacing what this folder published: %v", err)
	}
}

// Without a credential, or a remote, a publish says what to do.
func TestPublishSaysWhatIsMissing(t *testing.T) {
	_, url := newRemote(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	if _, _, err := run(t, dir, publishOptions{}, never(t)); err == nil || !strings.Contains(err.Error(), "--set-remote") {
		t.Errorf("no remote: %v", err)
	}
	t.Setenv(sessionEnv, "")
	if _, _, err := run(t, dir, publishOptions{setRemote: url}, never(t)); err == nil || !strings.Contains(err.Error(), sessionEnv) {
		t.Errorf("no credential: %v", err)
	}
}

// --set-remote records the remote in .anfra/config.yml, where the next publish
// finds it; a dry run records nothing.
func TestSetRemote(t *testing.T) {
	s, url := newRemote(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	if _, _, err := run(t, dir, publishOptions{setRemote: url, dryRun: true}, never(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(configPath(dir)); err == nil {
		t.Error("a dry run wrote .anfra/config.yml")
	}
	if _, _, err := run(t, dir, publishOptions{setRemote: url}, never(t)); err != nil {
		t.Fatal(err)
	}
	if r, ok, err := readRemote(dir); err != nil || !ok || r.String() != url {
		t.Errorf("remote = %v %v %v, want %s", r, ok, err, url)
	}
	write(t, dir, map[string]string{"a.aml": "a2"})
	if _, _, err := run(t, dir, publishOptions{}, never(t)); err != nil || len(s.deployed) != 1 {
		t.Errorf("publishing to the recorded remote: %v", err)
	}
}

// Setting a value changes its bytes and nothing else: comments, order, other
// keys and quoting survive; a missing section is appended; what anfra will not
// guess at is refused with the line to write.
func TestEditConfigString(t *testing.T) {
	const url = "https://bi.example.com/o/acme"
	for name, tc := range map[string]struct{ in, want string }{
		"no file":          {"", "# This repo's anfra settings: committed, and free of credentials.\nremote:\n  url: " + url + "\n"},
		"comments only":    {"# mine\n", "# mine\nremote:\n  url: " + url + "\n"},
		"no section":       {"apps:\n    dir: dash # four spaces\n", "apps:\n    dir: dash # four spaces\nremote:\n  url: " + url + "\n"},
		"no final newline": {"apps:\n  dir: dash", "apps:\n  dir: dash\nremote:\n  url: " + url + "\n"},
		"plain": {"# top\nremote:\n  url: https://old.example.com/o/x # where\nother: 1\n",
			"# top\nremote:\n  url: " + url + " # where\nother: 1\n"},
		"double quoted":   {"remote:\n    url: \"https://old.example.com/o/x\\u0020\" # q\n", "remote:\n    url: " + url + " # q\n"},
		"flow section":    {"remote: {}\n", ""},
		"single quotes":   {"remote:\n  url: 'https://o.example.com/o/it''s'\n  more: x\n", "remote:\n  url: " + url + "\n  more: x\n"},
		"after unicode":   {"# café\nremote:\n  url: https://old.example.com/o/x\n", "# café\nremote:\n  url: " + url + "\n"},
		"crlf":            {"remote:\r\n  url: https://old.example.com/o/x\r\n", "remote:\r\n  url: " + url + "\r\n"},
		"section, no key": {"remote:\n  other: 1\n", ""},
		"null value":      {"remote:\n  url:\n", ""},
		"multi-line":      {"remote:\n  url: >\n    https://old\n", ""},
		"flow top level":  {"{remote: {url: x}}\n", ""},
		"not a mapping":   {"- a\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := editConfigString([]byte(tc.in), "remote", "url", url)
			if tc.want == "" {
				if err == nil {
					t.Errorf("edited to %q, want a refusal", got)
				}
				return
			}
			if err != nil || string(got) != tc.want {
				t.Errorf("got %q, %v\nwant %q", got, err, tc.want)
			}
			if err == nil {
				if err := checkConfigString(got, "remote", "url", url); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

// A refused edit names the line to write, and changes nothing; a held lock
// refuses, as git's does.
func TestSetConfigStringRefusesSafely(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	if err := os.WriteFile(path, []byte("remote:\n  other: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := setConfigString(path, "remote", "url", "https://bi.example.com/o/acme")
	if err == nil || !strings.Contains(err.Error(), "url: https://bi.example.com/o/acme") {
		t.Errorf("err = %v, want the line to write by hand", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "remote:\n  other: 1\n" {
		t.Errorf("a refused edit changed the file: %q", b)
	}

	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := setConfigString(path, "remote", "url", "https://bi.example.com/o/acme"); err == nil || !strings.Contains(err.Error(), ".lock exists") {
		t.Errorf("err = %v, want the held lock refused", err)
	}
}
