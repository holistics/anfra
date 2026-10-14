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

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/repoformat"
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
		".anfra/config.yml":               "org:\n  url: https://bi.example.com/o/acme\n",
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

// server is a fake anfra-cloud serving the publish ops, in memory.
type server struct {
	mu       sync.Mutex
	blobs    map[string][]byte
	deployed []cloudapi.File
	revision int
	by       *cloudapi.Person
	limits   cloudapi.Limits
	uploads  int  // create_blobs calls
	forget   bool // drop every blob once, as housekeeping would
	cookie   string
	// The org's data sources, name to type, and what it was sent for each one
	// created; listing or creating refused when set.
	sources      map[string]string
	created      map[string]map[string]any
	forbidList   bool
	forbidCreate bool
	unreachable  map[string]string // a data source's check failing, with this reason
	tested       int               // data_sources.test calls
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	case cloudapi.OpStatus:
		reply(cloudapi.StatusOut{Revision: rev(), PublishedBy: s.by, PublishedAt: time.Now(), Publish: cloudapi.PublishSettings{Limits: s.limits}})
	case cloudapi.OpListMissingBlobs:
		var in cloudapi.ListMissingBlobsIn
		_ = jsonkit.UnmarshalRead(r.Body, &in)
		out := cloudapi.ListMissingBlobsOut{OIDs: []string{}}
		for _, oid := range in.OIDs {
			if _, ok := s.blobs[oid]; !ok {
				out.OIDs = append(out.OIDs, oid)
			}
		}
		reply(out)
	case cloudapi.OpCreateBlobs:
		var in cloudapi.CreateBlobsIn
		_ = jsonkit.UnmarshalRead(r.Body, &in)
		s.uploads++
		out := cloudapi.CreateBlobsOut{}
		for _, b := range in.Blobs {
			s.blobs[repoformat.BlobID(b)] = b
			out.OIDs = append(out.OIDs, repoformat.BlobID(b))
		}
		reply(out)
	case cloudapi.OpPublish:
		var in cloudapi.PublishIn
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
			_ = jsonkit.MarshalWrite(w, apperr.Envelope{Error: apperr.Response{Code: "blobs_missing", Message: "m", Details: cloudapi.MissingBlobs{OIDs: missing}}})
			return
		}
		if slices.Equal(in.Files, s.deployed) {
			reply(cloudapi.PublishOut{Revision: rev()})
			return
		}
		replaced := &cloudapi.Replaced{Revision: rev(), PublishedBy: s.by}
		s.deployed, s.revision, s.by = in.Files, s.revision+1, &cloudapi.Person{ID: "u1", Email: "ada@example.com"}
		reply(cloudapi.PublishOut{Revision: rev(), Changed: true, Replaced: replaced})
	case cloudapi.OpListDataSources:
		if s.forbidList {
			refuse(w, http.StatusForbidden, "forbidden")
			return
		}
		out := cloudapi.ListDataSourcesOut{DataSources: []cloudapi.DataSource{}}
		for name, typ := range s.sources {
			out.DataSources = append(out.DataSources, cloudapi.DataSource{Name: name, Type: typ})
		}
		reply(out)
	case cloudapi.OpCreateDataSource:
		if s.forbidCreate {
			refuse(w, http.StatusForbidden, "forbidden")
			return
		}
		var in cloudapi.CreateDataSourceIn
		_ = jsonkit.UnmarshalRead(r.Body, &in)
		s.sources[in.Name], s.created[in.Name] = in.Type, in.Connection
		reply(cloudapi.DataSource{Name: in.Name, Type: in.Type})
	case cloudapi.OpTestDataSource:
		if s.forbidCreate { // checking a connection is the admin's, as adding one is
			refuse(w, http.StatusForbidden, "forbidden")
			return
		}
		s.tested++
		var in cloudapi.TestDataSourceIn
		_ = jsonkit.UnmarshalRead(r.Body, &in)
		reason, failing := s.unreachable[in.Name]
		reply(cloudapi.TestDataSourceOut{OK: !failing, Message: reason})
	default:
		http.NotFound(w, r)
	}
}

func newServer(t *testing.T) (*server, string) {
	t.Helper()
	s := &server{blobs: map[string][]byte{}, sources: map[string]string{}, created: map[string]map[string]any{}, limits: cloudapi.Limits{Files: 100, UploadBlobs: 1000, UploadBytes: 1 << 20}}
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
	s, url := newServer(t)
	stubValidation(t, errors.New("the repo has errors: fix them and publish again"))
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "Model {"})
	if _, _, err := run(t, dir, publishOptions{setOrg: url}, never(t)); err == nil || !strings.Contains(err.Error(), "has errors") {
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
	return func(q string, choices ...string) (int, bool) { t.Errorf("asked %q", q); return len(choices) - 1, true }
}

// noTerminal is a prompt nobody can answer.
func noTerminal(string, ...string) (int, bool) { return 0, false }

// script answers questions in turn, each by its choice's name, and records
// them: what a person at a terminal would.
type script struct {
	t       *testing.T
	answers []string
	asked   []string
}

func (s *script) ask(question string, choices ...string) (int, bool) {
	s.t.Helper()
	s.asked = append(s.asked, question)
	if len(s.answers) == 0 {
		s.t.Fatalf("asked %q, with no answer left", question)
	}
	answer := s.answers[0]
	s.answers = s.answers[1:]
	i := slices.Index(choices, answer)
	if i < 0 {
		s.t.Fatalf("asked %q with %v; the answer %q is not among them", question, choices, answer)
	}
	return i, true
}

// A publish uploads what the server lacks, then publishes every file; publishing
// again uploads nothing and changes nothing; an edit uploads the one file.
func TestPublish(t *testing.T) {
	s, url := newServer(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"models/a.aml": "a", "models/b.aml": "b", "copy.aml": "a", ".anfra/data_sources.yml": "secret"})
	o := publishOptions{setOrg: url, message: "first"}

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

// Uploads are batched within the server's limits, not the client's.
func TestPublishBatchesUploads(t *testing.T) {
	s, url := newServer(t)
	s.limits.UploadBlobs = 2
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "1", "b.aml": "2", "c.aml": "3", "d.aml": "4", "e.aml": "5"})
	if _, _, err := run(t, dir, publishOptions{setOrg: url}, never(t)); err != nil {
		t.Fatal(err)
	}
	if s.uploads != 3 {
		t.Errorf("%d upload calls for 5 blobs at 2 a call, want 3", s.uploads)
	}
}

// Content collected between upload and publish is uploaded again, once.
func TestPublishUploadsAgainWhatWasCollected(t *testing.T) {
	s, url := newServer(t)
	s.forget = true
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	if _, _, err := run(t, dir, publishOptions{setOrg: url}, never(t)); err != nil {
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
	s, url := newServer(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	o := publishOptions{setOrg: url}
	if _, _, err := run(t, dir, o, never(t)); err != nil {
		t.Fatalf("over the starting files: %v", err)
	}

	// Someone publishes elsewhere.
	s.by, s.revision = &cloudapi.Person{ID: "u2", Email: "bob@example.com"}, s.revision+1
	write(t, dir, map[string]string{"a.aml": "a2"})
	var asked string
	_, errOut, err := run(t, dir, o, func(q string, _ ...string) (int, bool) { asked = q; return 1, true })
	if err == nil || asked == "" || !strings.Contains(errOut, "bob@example.com") || !strings.Contains(errOut, "This repo was last published by bob@example.com") {
		t.Fatalf("declined: err %v, asked %q, said %q", err, asked, errOut)
	}
	if _, _, err := run(t, dir, o, noTerminal); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Errorf("without a terminal: %v, want a refusal naming --yes", err)
	}
	deployed := len(s.deployed)
	if _, _, err := run(t, dir, publishOptions{setOrg: url, yes: true}, never(t)); err != nil {
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

// Without a credential, or an org, a publish says what to do.
func TestPublishSaysWhatIsMissing(t *testing.T) {
	_, url := newServer(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	if _, _, err := run(t, dir, publishOptions{}, never(t)); err == nil || !strings.Contains(err.Error(), "--set-org") {
		t.Errorf("no org: %v", err)
	}
	t.Setenv(sessionEnv, "")
	if _, _, err := run(t, dir, publishOptions{setOrg: url}, never(t)); err == nil || !strings.Contains(err.Error(), sessionEnv) {
		t.Errorf("no credential: %v", err)
	}
}

// --set-org records the org in .anfra/config.yml, where the next publish
// finds it; a dry run records nothing.
func TestSetOrg(t *testing.T) {
	s, url := newServer(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	if _, _, err := run(t, dir, publishOptions{setOrg: url, dryRun: true}, never(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(configPath(dir)); err == nil {
		t.Error("a dry run wrote .anfra/config.yml")
	}
	if _, _, err := run(t, dir, publishOptions{setOrg: url}, never(t)); err != nil {
		t.Fatal(err)
	}
	if r, ok, err := readOrg(dir); err != nil || !ok || r.String() != url {
		t.Errorf("org = %v %v %v, want %s", r, ok, err, url)
	}
	write(t, dir, map[string]string{"a.aml": "a2"})
	if _, _, err := run(t, dir, publishOptions{}, never(t)); err != nil || len(s.deployed) != 1 {
		t.Errorf("publishing to the recorded org: %v", err)
	}
}

// Setting a value changes its bytes and nothing else: comments, order, other
// keys and quoting survive; a missing section is appended; what anfra will not
// guess at is refused with the line to write.
func TestEditConfigString(t *testing.T) {
	const url = "https://bi.example.com/o/acme"
	for name, tc := range map[string]struct{ in, want string }{
		"no file":          {"", "# This repo's anfra settings: committed, and free of credentials.\norg:\n  url: " + url + "\n"},
		"comments only":    {"# mine\n", "# mine\norg:\n  url: " + url + "\n"},
		"no section":       {"apps:\n    dir: dash # four spaces\n", "apps:\n    dir: dash # four spaces\norg:\n  url: " + url + "\n"},
		"no final newline": {"apps:\n  dir: dash", "apps:\n  dir: dash\norg:\n  url: " + url + "\n"},
		"plain": {"# top\norg:\n  url: https://old.example.com/o/x # where\nother: 1\n",
			"# top\norg:\n  url: " + url + " # where\nother: 1\n"},
		"double quoted":   {"org:\n    url: \"https://old.example.com/o/x\\u0020\" # q\n", "org:\n    url: " + url + " # q\n"},
		"flow section":    {"org: {}\n", ""},
		"single quotes":   {"org:\n  url: 'https://o.example.com/o/it''s'\n  more: x\n", "org:\n  url: " + url + "\n  more: x\n"},
		"after unicode":   {"# café\norg:\n  url: https://old.example.com/o/x\n", "# café\norg:\n  url: " + url + "\n"},
		"crlf":            {"org:\r\n  url: https://old.example.com/o/x\r\n", "org:\r\n  url: " + url + "\r\n"},
		"section, no key": {"org:\n  other: 1\n", ""},
		"null value":      {"org:\n  url:\n", ""},
		"multi-line":      {"org:\n  url: >\n    https://old\n", ""},
		"flow top level":  {"{org: {url: x}}\n", ""},
		"not a mapping":   {"- a\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := editConfigString([]byte(tc.in), "org", "url", url)
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
				if err := checkConfigString(got, "org", "url", url); err != nil {
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
	if err := os.WriteFile(path, []byte("org:\n  other: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := setConfigString(path, "org", "url", "https://bi.example.com/o/acme")
	if err == nil || !strings.Contains(err.Error(), "url: https://bi.example.com/o/acme") {
		t.Errorf("err = %v, want the line to write by hand", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "org:\n  other: 1\n" {
		t.Errorf("a refused edit changed the file: %q", b)
	}

	if err := os.WriteFile(path+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := setConfigString(path, "org", "url", "https://bi.example.com/o/acme"); err == nil || !strings.Contains(err.Error(), ".lock exists") {
		t.Errorf("err = %v, want the held lock refused", err)
	}
}

func refuse(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonkit.MarshalWrite(w, apperr.Envelope{Error: apperr.Response{Code: code, Message: code}})
}
