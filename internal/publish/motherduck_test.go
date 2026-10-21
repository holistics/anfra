package publish

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/motherduck"
)

// fakeMotherDuck is a MotherDuck account, recording what is done to it.
type fakeMotherDuck struct {
	installed, signedIn bool
	installedAfter      int // looks for the CLI that find none, before it is installed
	finds               int
	databases           map[string]bool
	tokenNames          map[string]bool // the account's tokens, by name
	uploadErr           error
	lockedBy            string // the first upload finds the file held by this program
	tokenErr            error

	calls   []string
	uploads map[string][]byte // database → the content uploaded
	tokens  []motherduck.Token
	revoked []string
}

func newFakeMotherDuck() *fakeMotherDuck {
	return &fakeMotherDuck{installed: true, signedIn: true, databases: map[string]bool{}, tokenNames: map[string]bool{}, uploads: map[string][]byte{}}
}

func (f *fakeMotherDuck) Find(context.Context) (string, bool) {
	f.calls = append(f.calls, "find")
	f.finds++
	return "v1.5.6", f.installed || (f.installedAfter > 0 && f.finds > f.installedAfter)
}

func (f *fakeMotherDuck) Status(context.Context) (motherduck.Status, error) {
	f.calls = append(f.calls, "status")
	if !f.signedIn {
		return motherduck.Status{}, nil
	}
	return motherduck.Status{Authenticated: true, User: "ada", Organization: "ada's"}, nil
}

func (f *fakeMotherDuck) Login(context.Context) error {
	f.calls = append(f.calls, "login")
	f.signedIn = true
	return nil
}

func (f *fakeMotherDuck) Session(context.Context) (string, []string, error) {
	f.calls = append(f.calls, "session")
	names := make([]string, 0, len(f.databases))
	for n := range f.databases {
		names = append(names, n)
	}
	return "session-token", names, nil
}

func (f *fakeMotherDuck) DatabaseExists(_ context.Context, name string) (bool, error) {
	f.calls = append(f.calls, "exists "+name)
	return f.databases[name], nil
}

func (f *fakeMotherDuck) CreateDatabaseFrom(_ context.Context, name, path string) error {
	f.calls = append(f.calls, "create "+name)
	if f.lockedBy != "" {
		holder := f.lockedBy
		f.lockedBy = ""
		return &motherduck.QueryError{Message: `IO Error: Could not set lock on file "` + path + `": Conflicting lock is held in ` + holder + ` (PID 7) by user a.`}
	}
	if f.databases[name] {
		return &motherduck.QueryError{Message: `Binder Error: database with name "` + name + `" already exists`}
	}
	if f.uploadErr != nil {
		return f.uploadErr
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	f.databases[name], f.uploads[name] = true, content
	return nil
}

func (f *fakeMotherDuck) CreateReadToken(_ context.Context, session, username, name, _ string) (motherduck.Token, error) {
	f.calls = append(f.calls, "token "+name)
	if session != "session-token" || username != "ada" {
		return motherduck.Token{}, errors.New("not the signed-in user's")
	}
	if f.tokenErr != nil {
		return motherduck.Token{}, f.tokenErr
	}
	if f.tokenNames[name] {
		return motherduck.Token{}, &motherduck.APIError{Status: 409, Message: "A token with that name already exists"}
	}
	f.tokenNames[name] = true
	t := motherduck.Token{ID: "t1", Name: name, Secret: "org-read-token"} //nolint:gosec // G101: a fake
	f.tokens = append(f.tokens, t)
	return t, nil
}

func (f *fakeMotherDuck) RevokeToken(_ context.Context, _, _, id string) error {
	f.calls = append(f.calls, "revoke "+id)
	f.revoked = append(f.revoked, id)
	return nil
}

// duckSources has two data sources reading one file, by its path and through a
// symlink, and one in memory, which is no file to copy.
const duckSources = `data_sources:
  alias:
    type: duckdb
    connection:
      path: link.duckdb?access_mode=read_only
  local:
    type: duckdb
    connection:
      path: data/n.duckdb
  mem:
    type: duckdb
    connection:
      path: ":memory:"
`

func duckRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", ".anfra/data_sources.yml": duckSources, "data/n.duckdb": "DUCK"})
	if err := os.Symlink(filepath.Join(dir, "data", "n.duckdb"), filepath.Join(dir, "link.duckdb")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runWith(t *testing.T, dir string, o Options, ask Prompt, md MotherDuck, typed ...string) (string, error) {
	t.Helper()
	env, _, errOut := testEnv(ask)
	env.MotherDuck = md
	env.Input = func(question string) (string, bool) {
		if len(typed) == 0 {
			t.Fatalf("asked to type %q, with nothing left to type", question)
		}
		answer := typed[0]
		typed = typed[1:]
		return answer, true
	}
	err := Run(context.Background(), dir, o, env)
	return errOut.String(), err
}

// One file is uploaded once, as a new database beside one of its name, which is
// left as it is; each data source reading it is created on the org on the copy,
// with a read-only token made for the org, never the person's own.
func TestCopiesADuckDBFileToMotherDuck(t *testing.T) {
	s, url := newServer(t)
	dir := duckRepo(t)
	md := newFakeMotherDuck()
	md.databases["n"] = true
	ask := &script{t: t, answers: []string{"yes"}}
	errOut, err := runWith(t, dir, Options{SetOrg: url}, ask.ask, md)
	if err != nil {
		t.Fatal(err)
	}
	// Each call connects to MotherDuck anew, which takes seconds: one look at
	// the account, one for the token and the database names, one upload.
	if !slices.Equal(md.calls, []string{"find", "status", "session", "token anfra acme", "create n_2"}) {
		t.Errorf("calls %v", md.calls)
	}
	// One question: the upload, naming the account it goes to.
	if len(ask.asked) != 1 || ask.asked[0] != "Upload link.duckdb (4 bytes) to your MotherDuck account ada (ada's), and give the anfra org read-only access to it?" {
		t.Errorf("asked %q", ask.asked)
	}
	if string(md.uploads["n_2"]) != "DUCK" || len(md.uploads) != 1 {
		t.Errorf("uploads %v", md.uploads)
	}
	for _, name := range []string{"alias", "local"} {
		got := s.created[name]
		if s.sources[name] != "motherduck" || got["token"] != "org-read-token" || got["dbname"] != "n_2" {
			t.Errorf("%s: %s, %v", name, s.sources[name], got)
		}
	}
	if _, ok := s.created["mem"]; ok {
		t.Error("an in-memory database was created on the org")
	}
	if len(md.tokens) != 1 || len(md.revoked) != 0 {
		t.Errorf("tokens %v, revoked %v", md.tokens, md.revoked)
	}
	for _, want := range []string{"A database named n already exists in your MotherDuck account, so this one will be named n_2", "Uploaded link.duckdb to MotherDuck as database n_2", "Queries on the models using mem fail"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("not said %q: %s", want, errOut)
		}
	}
	// The org's token is shown once, to keep; the person's own, never.
	if strings.Contains(errOut, "session-token") || strings.Count(errOut, "org-read-token") != 1 || !strings.Contains(errOut, "It is shown only once") {
		t.Errorf("tokens said: %s", errOut)
	}
}

// Declined, nothing is done but looking at who the CLI is signed in as, which
// the question names; without a terminal, there is no offer.
func TestDeclinedCopyTouchesNothing(t *testing.T) {
	s, url := newServer(t)
	dir := duckRepo(t)
	md := newFakeMotherDuck()
	errOut, err := runWith(t, dir, Options{SetOrg: url}, (&script{t: t, answers: []string{"no"}}).ask, md)
	if err != nil || !slices.Equal(md.calls, []string{"find", "status"}) || len(s.created) != 0 || !strings.Contains(errOut, "load its data into a warehouse") {
		t.Errorf("err %v, calls %v, created %v; said %s", err, md.calls, s.created, errOut)
	}
	errOut, err = runWith(t, dir, Options{SetOrg: url}, never(t), nil)
	if err != nil || len(s.created) != 0 || !strings.Contains(errOut, "Or run anfra publish in a terminal to upload it to MotherDuck") {
		t.Errorf("no terminal: err %v, created %v; said %s", err, s.created, errOut)
	}
}

// Without the CLI, it waits for it to be installed; signed out, it signs in,
// once agreed, and the account signed in to is not asked about again.
func TestCopyHelpsInstallAndSignIn(t *testing.T) {
	s, url := newServer(t)
	dir := duckRepo(t)
	md := newFakeMotherDuck()
	md.installed, md.installedAfter, md.signedIn = false, 2, false // installed while asked to: the look before the question, and the first after, find none
	ask := &script{t: t, answers: []string{"yes", "yes", "yes"}}   // upload, installed, sign in
	errOut, err := runWith(t, dir, Options{SetOrg: url}, ask.ask, md)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(md.calls, "login") || s.sources["local"] != "motherduck" {
		t.Errorf("calls %v, sources %v", md.calls, s.sources)
	}
	if !strings.Contains(errOut, "install.motherduck.com") || !strings.Contains(errOut, "Signed in to MotherDuck as ada (ada's)") || len(ask.asked) != 3 {
		t.Errorf("said %s", errOut)
	}

	// Not installed after all: this data source is not uploaded, and nothing
	// but looking for the CLI is done.
	s, url = newServer(t)
	md = newFakeMotherDuck()
	md.installed = false
	ask = &script{t: t, answers: []string{"yes", "no"}}
	if _, err := runWith(t, dir, Options{SetOrg: url}, ask.ask, md); err != nil || len(s.created) != 0 || slices.ContainsFunc(md.calls, func(c string) bool { return c != "find" }) {
		t.Errorf("err %v, created %v, calls %v", err, s.created, md.calls)
	}
}

// An upload MotherDuck refuses makes no data source, and the token made for it
// is revoked; a database it left is said, never removed. One whose name was
// taken meanwhile is not called partial: it is not this upload's.
func TestFailedUploadIsSaidNotUndone(t *testing.T) {
	s, url := newServer(t)
	dir := duckRepo(t)
	md := newFakeMotherDuck()
	md.uploadErr = &motherduck.QueryError{Message: "Out of storage"}
	errOut, err := runWith(t, dir, Options{SetOrg: url}, (&script{t: t, answers: []string{"yes"}}).ask, md)
	if err != nil || len(s.created) != 0 || !strings.Contains(errOut, "Out of storage") {
		t.Errorf("err %v, created %v; said %s", err, s.created, errOut)
	}
	if !slices.Equal(md.revoked, []string{"t1"}) {
		t.Errorf("the unused token not revoked: %v", md.revoked)
	}

	md = newFakeMotherDuck()
	md.uploadErr = &motherduck.QueryError{Message: "Binder Error: database with name \"n\" already exists"}
	errOut, _ = runWith(t, dir, Options{SetOrg: url}, (&script{t: t, answers: []string{"yes"}}).ask, md)
	if !strings.Contains(errOut, "created in the meantime") || strings.Contains(errOut, "partly created") {
		t.Errorf("said %s", errOut)
	}
}

// The org's token is revoked when no data source came to use it: here, the
// server cannot reach the copy and the person skips it.
func TestUnusedTokenIsRevoked(t *testing.T) {
	s, url := newServer(t)
	s.unreachable = map[string]string{"alias": "no", "local": "no"}
	dir := duckRepo(t)
	md := newFakeMotherDuck()
	ask := &script{t: t, answers: []string{"yes", "skip creating this data source", "skip creating this data source"}}
	errOut, err := runWith(t, dir, Options{SetOrg: url}, ask.ask, md)
	if err != nil || len(s.created) != 0 || !slices.Equal(md.revoked, []string{"t1"}) || !strings.Contains(errOut, "Revoked the unused MotherDuck token") {
		t.Errorf("err %v, created %v, revoked %v; said %s", err, s.created, md.revoked, errOut)
	}
	if !md.databases["n"] {
		t.Error("the uploaded database is gone")
	}
}

// A file anfra serve holds is uploaded from a copy; one another program holds is
// not uploaded at all, since a copy of a file being written may be torn, and the
// token made for it is revoked.
func TestHeldFile(t *testing.T) {
	s, url := newServer(t)
	dir := duckRepo(t)
	md := newFakeMotherDuck()
	md.lockedBy = "/home/a/.anfra/sidecars/canal-query-1a2b"
	errOut, err := runWith(t, dir, Options{SetOrg: url}, (&script{t: t, answers: []string{"yes"}}).ask, md)
	if err != nil || string(md.uploads["n"]) != "DUCK" || s.sources["local"] != "motherduck" || !strings.Contains(errOut, "a temporary copy of it is uploaded instead") {
		t.Errorf("err %v, uploads %v, sources %v; said %s", err, md.uploads, s.sources, errOut)
	}

	s, url = newServer(t)
	md = newFakeMotherDuck()
	md.lockedBy = "/usr/bin/python3"
	errOut, err = runWith(t, dir, Options{SetOrg: url}, (&script{t: t, answers: []string{"yes"}}).ask, md)
	if err != nil || len(md.uploads) != 0 || !slices.Equal(md.revoked, []string{"t1"}) || len(s.created) != 0 || !strings.Contains(errOut, "open in another program (/usr/bin/python3, PID 7)") {
		t.Errorf("err %v, uploads %v, revoked %v; said %s", err, md.uploads, md.revoked, errOut)
	}
}

// A file that changes while it is copied is not uploaded.
func TestCopyForUploadRefusesAChangingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "n.duckdb")
	write(t, dir, map[string]string{"n.duckdb": "DUCK", "n.duckdb.wal": "LOG"})
	copied, cleanup, err := copyForUpload(path)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(copied); string(b) != "DUCK" {
		t.Errorf("copy %q", b)
	}
	if b, _ := os.ReadFile(copied + ".wal"); string(b) != "LOG" {
		t.Errorf("log %q", b)
	}
	cleanup()
	if _, err := os.Stat(filepath.Dir(copied)); !os.IsNotExist(err) {
		t.Errorf("not cleaned up: %v", err)
	}
}

// The account the CLI is signed in to is named in the upload question. Quitting
// ends the publish, for the person to switch accounts, which anfra leaves to
// them; nothing is uploaded, and nobody signed out.
func TestUploadQuestionNamesTheAccount(t *testing.T) {
	s, url := newServer(t)
	dir := duckRepo(t)
	md := newFakeMotherDuck()
	ask := &script{t: t, answers: []string{"quit"}}
	_, err := runWith(t, dir, Options{SetOrg: url}, ask.ask, md)
	if !errors.Is(err, errSwitchAccount) || len(md.uploads) != 0 || len(s.created) != 0 || len(s.deployed) != 0 {
		t.Errorf("err %v, uploads %v, created %v, deployed %v", err, md.uploads, s.created, s.deployed)
	}
	if !slices.Equal(md.calls, []string{"find", "status"}) {
		t.Errorf("more than looking: %v", md.calls)
	}

	// The CLI installed meanwhile, signed in already: that account was not named
	// when the upload was agreed to, so it is asked again, naming it.
	md = newFakeMotherDuck()
	md.installed, md.installedAfter = false, 2
	ask = &script{t: t, answers: []string{"yes", "yes", "no"}} // upload, installed, upload to ada's: no
	if _, err := runWith(t, dir, Options{SetOrg: url}, ask.ask, md); err != nil || len(md.uploads) != 0 || len(s.created) != 0 {
		t.Errorf("err %v, uploads %v, created %v", err, md.uploads, s.created)
	}
	if len(ask.asked) != 3 || !strings.Contains(ask.asked[0], "to MotherDuck,") || !strings.Contains(ask.asked[2], "to your MotherDuck account ada (ada's),") {
		t.Errorf("asked %q", ask.asked)
	}
}

// jwt is a token as MotherDuck's look, with these claims.
func jwt(claims string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"HS256"}`)) + "." + enc([]byte(claims)) + "." + enc([]byte("sig"))
}

// The org's token name taken by an earlier publish's: nothing is replaced or
// revoked, and the person chooses the next number, a name, or a token they
// kept, which must be read-only. Skipped, nothing is uploaded.
func TestOrgTokenNameTaken(t *testing.T) {
	cases := []struct {
		name    string
		answers []string
		typed   []string
		want    string // the org's token, or "" for none
		made    []string
	}{
		{"next number", []string{"yes", `create new token named "anfra acme 2"`}, nil, "org-read-token", []string{"anfra acme 2"}},
		{"named", []string{"yes", "name a new token"}, []string{"mine"}, "org-read-token", []string{"mine"}},
		{"pasted", []string{"yes", "paste a read-only token"},
			[]string{jwt(`{"tokenType":"read_scaling","readOnly":true}`)}, jwt(`{"tokenType":"read_scaling","readOnly":true}`), nil},
		{"pasted, writing, then skipped", []string{"yes", "paste a read-only token", "skip uploading these data sources"},
			[]string{jwt(`{"tokenType":"read_write","readOnly":false}`)}, "", nil},
		{"skipped", []string{"yes", "skip uploading these data sources"}, nil, "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, url := newServer(t)
			dir := duckRepo(t)
			md := newFakeMotherDuck()
			md.tokenNames["anfra acme"] = true
			ask := &script{t: t, answers: tc.answers}
			errOut, err := runWith(t, dir, Options{SetOrg: url}, ask.ask, md, tc.typed...)
			if err != nil {
				t.Fatal(err)
			}
			if got := s.created["local"]["token"]; tc.want != "" && got != tc.want {
				t.Errorf("the org's token %v, want %q", got, tc.want)
			}
			if tc.want == "" && (len(md.uploads) != 0 || len(s.created) != 0) {
				t.Errorf("no token, but uploaded %v, created %v", md.uploads, s.created)
			}
			var made []string
			for _, tok := range md.tokens {
				made = append(made, tok.Name)
			}
			if !slices.Equal(made, tc.made) || len(md.revoked) != 0 {
				t.Errorf("made %v, revoked %v", made, md.revoked)
			}
			if !strings.Contains(errOut, `A token named "anfra acme" already exists`) || !slices.Contains(ask.asked, "Which token should data sources alias, local use?") {
				t.Errorf("asked %q; said %s", ask.asked, errOut)
			}
		})
	}
}

// twoDuckFiles is a repo of two DuckDB files, each a data source.
func twoDuckFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", "a.duckdb": "A", "b.duckdb": "B", ".anfra/data_sources.yml": `data_sources:
  a:
    type: duckdb
    connection:
      path: a.duckdb
  b:
    type: duckdb
    connection:
      path: b.duckdb
`})
	return dir
}

// Yes to all uploads every DuckDB file still to come without asking again, to
// the account the question named; the last file is not offered it.
func TestUploadYesToAll(t *testing.T) {
	s, url := newServer(t)
	md := newFakeMotherDuck()
	ask := &script{t: t, answers: []string{"all"}}
	if _, err := runWith(t, twoDuckFiles(t), Options{SetOrg: url}, ask.ask, md); err != nil {
		t.Fatal(err)
	}
	if len(ask.asked) != 1 || string(md.uploads["a"]) != "A" || string(md.uploads["b"]) != "B" || s.sources["a"] != "motherduck" || s.sources["b"] != "motherduck" {
		t.Errorf("asked %q, uploads %v, sources %v", ask.asked, md.uploads, s.sources)
	}

	// Asked about each, the last has no "all": nothing remains.
	_, url = newServer(t)
	ask = &script{t: t, answers: []string{"yes", "yes"}}
	if _, err := runWith(t, twoDuckFiles(t), Options{SetOrg: url}, ask.ask, newFakeMotherDuck()); err != nil {
		t.Fatal(err)
	}
	hasAll := func(choices []string) bool {
		return slices.ContainsFunc(choices, func(c string) bool { return strings.HasPrefix(c, "all") })
	}
	if len(ask.choices) != 2 || !hasAll(ask.choices[0]) || hasAll(ask.choices[1]) {
		t.Errorf("choices %q", ask.choices)
	}
}

// Yes to all before an account was named still asks once about the account
// it turns out to be; no there is no to the rest too. One signed in to in this
// publish was chosen there, and is not asked about.
func TestYesToAllNeedsANamedAccount(t *testing.T) {
	s, url := newServer(t)
	md := newFakeMotherDuck()
	md.installed, md.installedAfter = false, 2
	ask := &script{t: t, answers: []string{"all", "yes", "no", "no"}} // all, installed, upload a to ada's: no; b: no
	if _, err := runWith(t, twoDuckFiles(t), Options{SetOrg: url}, ask.ask, md); err != nil {
		t.Fatal(err)
	}
	if len(md.uploads) != 0 || len(s.created) != 0 || len(ask.asked) != 4 { // b asked again: no to all is gone
		t.Errorf("asked %q, uploads %v, created %v", ask.asked, md.uploads, s.created)
	}

	s, url = newServer(t)
	md = newFakeMotherDuck()
	md.signedIn = false
	ask = &script{t: t, answers: []string{"all", "yes"}} // all, sign in
	if _, err := runWith(t, twoDuckFiles(t), Options{SetOrg: url}, ask.ask, md); err != nil {
		t.Fatal(err)
	}
	if len(md.uploads) != 2 || len(ask.asked) != 2 || s.sources["a"] != "motherduck" || s.sources["b"] != "motherduck" {
		t.Errorf("asked %q, uploads %v, sources %v", ask.asked, md.uploads, s.sources)
	}
}
