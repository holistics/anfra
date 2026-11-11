package publish

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/publish/syncstate"
	"github.com/holistics/anfra/shared/apperr"
)

// newRepo is an anfra repo folder, `anfra init` having made its .anfra/, with
// files written into it.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".anfra"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, files)
	return dir
}

func pull(t *testing.T, dir string, o PullOptions) (PullResult, error) {
	t.Helper()
	env, _, _ := testEnv(nil)
	return Pull(context.Background(), dir, o, env)
}

func mustPull(t *testing.T, dir string, o PullOptions) PullResult {
	t.Helper()
	res, err := pull(t, dir, o)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func read(t *testing.T, dir, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, path))
	if errors.Is(err, os.ErrNotExist) {
		return "<absent>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func syncOf(t *testing.T, dir, url string) *syncstate.Org {
	t.Helper()
	s, err := syncstate.For(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	return s[url]
}

func codeOf(err error) apperr.Code { return apperr.From(err).Code }

// A folder fresh from `anfra init` gets every file the org has, and its
// revision as the base, with nothing to conflict.
func TestPullIntoFreshFolder(t *testing.T) {
	srv, url := newServer(t)
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "a\n", "apps/x/index.html": "<html>\n"})
	dir := newRepo(t, nil)
	res := mustPull(t, dir, PullOptions{SetOrg: url})
	if read(t, dir, "models/a.aml") != "a\n" || read(t, dir, "apps/x/index.html") != "<html>\n" {
		t.Errorf("files not pulled")
	}
	if len(res.Conflicts) != 0 || len(res.Changes) != 2 || res.PublishedBy != "bob@acme.com" {
		t.Errorf("%+v", res)
	}
	// A fresh folder had nothing that could differ: no word about it.
	if strings.Contains(strings.Join(res.Hints, " "), "no record") {
		t.Errorf("hints %v", res.Hints)
	}
	if s := syncOf(t, dir, url); s == nil || s.Base != srv.rev() || s.Pending != nil {
		t.Errorf("sync state %+v", s)
	}
	// Pulling again: up to date.
	if res := mustPull(t, dir, PullOptions{}); !strings.Contains(res.Message, "already has everything") {
		t.Errorf("%s", res.Message)
	}
}

// After a publish from here, others' changes merge in: added, updated,
// deleted and merged, and this folder's own edits kept.
func TestPullMergesChanges(t *testing.T) {
	srv, url := newServer(t)
	dir := newRepo(t, map[string]string{
		"models/a.aml": "a\n", "models/c.aml": "c\n", "models/d.aml": "d\n", "models/m.aml": "1\n2\n3\n4\n5\n",
	})
	if _, _, err := run(t, dir, Options{SetOrg: url}, nil); err != nil {
		t.Fatal(err)
	}
	srv.publishAs("bob@acme.com", map[string]string{
		"models/a.aml": "A\n", "models/b.aml": "b\n", "models/d.aml": "d\n", "models/m.aml": "1\n2\n3\n4\nFIVE\n",
	}) // a updated, b added, c deleted, m's last line changed
	write(t, dir, map[string]string{"models/d.aml": "D here\n", "models/m.aml": "ONE\n2\n3\n4\n5\n"})
	res := mustPull(t, dir, PullOptions{})
	want := map[string]string{"models/a.aml": "A\n", "models/b.aml": "b\n", "models/c.aml": "<absent>",
		"models/d.aml": "D here\n", "models/m.aml": "ONE\n2\n3\n4\nFIVE\n"}
	for p, w := range want {
		if got := read(t, dir, p); got != w {
			t.Errorf("%s = %q, want %q", p, got, w)
		}
	}
	changes := map[string]string{}
	for _, c := range res.Changes {
		changes[c.Path] = c.Change
	}
	if changes["models/a.aml"] != "updated" || changes["models/b.aml"] != "added" || changes["models/c.aml"] != "deleted" || changes["models/m.aml"] != "merged" {
		t.Errorf("changes %v", changes)
	}
	if res.Base == "" || len(res.Conflicts) != 0 {
		t.Errorf("%+v", res)
	}
}

// A conflict leaves markers, keeps the pull pending, and refuses another pull
// until the markers are gone; then the pull's revision is the base.
func TestPullConflictThenResolveByEditing(t *testing.T) {
	srv, url := newServer(t)
	dir := newRepo(t, map[string]string{"models/a.aml": "x\ny\nz\n"})
	if _, _, err := run(t, dir, Options{SetOrg: url}, nil); err != nil {
		t.Fatal(err)
	}
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "x\nY-org\nz\n"})
	write(t, dir, map[string]string{"models/a.aml": "x\nY-local\nz\n"})
	res := mustPull(t, dir, PullOptions{})
	if got := res.Conflicts["content"]; len(got) != 1 || !slices.Equal(got[0].Paths, []string{"models/a.aml"}) {
		t.Fatalf("conflicts %+v", res.Conflicts)
	}
	if res.HowToResolve["content"] == "" || !strings.Contains(res.Message, "1 conflict to resolve") {
		t.Errorf("%+v", res)
	}
	if a := read(t, dir, "models/a.aml"); !strings.Contains(a, "<<<<<<< local\n") || !strings.Contains(a, ">>>>>>> org (bob@acme.com, ") {
		t.Errorf("no markers:\n%s", a)
	}
	if _, err := pull(t, dir, PullOptions{}); codeOf(err) != clierr.ConflictsUnresolved.Code() {
		t.Errorf("pulling again: %v", err)
	}
	write(t, dir, map[string]string{"models/a.aml": "x\nY-both\nz\n"})
	if res := mustPull(t, dir, PullOptions{}); !strings.Contains(res.Message, "already has everything") {
		t.Errorf("after resolving: %s", res.Message)
	}
	if s := syncOf(t, dir, url); s.Base != srv.rev() || s.Pending != nil {
		t.Errorf("sync state %+v", s)
	}
}

// --take keeps one side of whole files; a binary conflict has no markers, and
// --take is how it's resolved.
func TestPullTake(t *testing.T) {
	srv, url := newServer(t)
	dir := newRepo(t, map[string]string{"models/a.aml": "x\ny\n", "apps/logo.png": "\x00png0"})
	if _, _, err := run(t, dir, Options{SetOrg: url}, nil); err != nil {
		t.Fatal(err)
	}
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "x\nORG\n", "apps/logo.png": "\x00png-org"})
	write(t, dir, map[string]string{"models/a.aml": "x\nLOCAL\n", "apps/logo.png": "\x00png-local"})
	res := mustPull(t, dir, PullOptions{})
	if len(res.Conflicts["content"]) != 1 || len(res.Conflicts["binary"]) != 1 {
		t.Fatalf("conflicts %+v", res.Conflicts)
	}
	if read(t, dir, "apps/logo.png") != "\x00png-local" {
		t.Errorf("a binary conflict changed the file")
	}
	res = mustPull(t, dir, PullOptions{Take: "org", Paths: []string{"apps/logo.png"}})
	if read(t, dir, "apps/logo.png") != "\x00png-org" || len(res.Conflicts["content"]) != 1 {
		t.Errorf("take org: %q, %+v", read(t, dir, "apps/logo.png"), res)
	}
	if _, err := pull(t, dir, PullOptions{Take: "org", Paths: []string{"models/zzz.aml"}}); codeOf(err) != apperr.ValidationFailed.Code() {
		t.Errorf("a path with no conflict: %v", err)
	}
	res = mustPull(t, dir, PullOptions{Take: "local", Paths: []string{"models/a.aml"}})
	if read(t, dir, "models/a.aml") != "x\nLOCAL\n" || !strings.Contains(res.Message, "Every conflict is resolved") {
		t.Errorf("take local: %q, %s", read(t, dir, "models/a.aml"), res.Message)
	}
	if s := syncOf(t, dir, url); s.Base != srv.rev() || s.Pending != nil {
		t.Errorf("sync state %+v", s)
	}
}

// --abort puts back every file the pull changed, and forgets it.
func TestPullAbort(t *testing.T) {
	srv, url := newServer(t)
	dir := newRepo(t, map[string]string{"models/a.aml": "x\ny\n", "models/gone.aml": "g\n"})
	if _, _, err := run(t, dir, Options{SetOrg: url}, nil); err != nil {
		t.Fatal(err)
	}
	base := syncOf(t, dir, url).Base
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "x\nORG\n", "models/new.aml": "n\n"})
	write(t, dir, map[string]string{"models/a.aml": "x\nLOCAL\n"})
	mustPull(t, dir, PullOptions{})
	res := mustPull(t, dir, PullOptions{Abort: true})
	if read(t, dir, "models/a.aml") != "x\nLOCAL\n" || read(t, dir, "models/gone.aml") != "g\n" || read(t, dir, "models/new.aml") != "<absent>" {
		t.Errorf("not put back: %v", res)
	}
	if s := syncOf(t, dir, url); s.Base != base || s.Pending != nil {
		t.Errorf("sync state %+v", s)
	}
	if !strings.Contains(res.Message, "files it changed are back as they were") {
		t.Errorf("%s", res.Message)
	}
	if res := mustPull(t, dir, PullOptions{Abort: true}); !strings.Contains(res.Message, "no pull to undo") {
		t.Errorf("%s", res.Message)
	}
}

// With no record of an earlier sync, files that differ conflict, and the
// result says why.
func TestPullWithNoBase(t *testing.T) {
	srv, url := newServer(t)
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "org\n", "models/b.aml": "b\n"})
	dir := newRepo(t, map[string]string{"models/a.aml": "local\n"})
	res := mustPull(t, dir, PullOptions{SetOrg: url})
	if got := res.Conflicts["add_add"]; len(got) != 1 || !strings.Contains(got[0].Message, "differs here and on the org") {
		t.Errorf("conflicts %+v", res.Conflicts)
	}
	if res.Base != "" || !strings.Contains(strings.Join(res.Hints, " "), "no record of this folder") {
		t.Errorf("%+v", res)
	}
	if read(t, dir, "models/b.aml") != "b\n" {
		t.Error("b not added")
	}
}

// A folder that already has every file the org has, unchanged, gets its
// revision as the base without writing anything.
func TestPullFolderAlreadyMatching(t *testing.T) {
	srv, url := newServer(t)
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "a\n"})
	dir := newRepo(t, map[string]string{"models/a.aml": "a\n", "models/mine.aml": "m\n"})
	res := mustPull(t, dir, PullOptions{SetOrg: url})
	if !strings.Contains(res.Message, "already has everything") || syncOf(t, dir, url).Base != srv.rev() {
		t.Errorf("%s", res.Message)
	}
}

func TestPullRefusals(t *testing.T) {
	_, url := newServer(t)
	if _, err := pull(t, t.TempDir(), PullOptions{SetOrg: url}); err == nil || !strings.Contains(err.Error(), "isn't an anfra repo") {
		t.Errorf("not a repo: %v", err)
	}
	// A pull stopped while writing refuses until it's undone.
	dir := newRepo(t, nil)
	if err := syncstate.For(dir).Save(syncstate.State{url: {Pending: &syncstate.Pending{Revision: "r", Stage: syncstate.Writing}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pull(t, dir, PullOptions{SetOrg: url}); codeOf(err) != clierr.PullInterrupted.Code() {
		t.Errorf("interrupted: %v", err)
	}
	if _, err := pull(t, dir, PullOptions{SetOrg: url, Abort: true}); err != nil {
		t.Errorf("abort: %v", err)
	}
}

// A pull names the data sources the repo uses that this folder lacks, with
// their types on the org.
func TestPullHintsMissingDataSources(t *testing.T) {
	s, url := newServer(t)
	s.sources["events"] = "clickhouse"
	s.publishAs("bob@example.com", map[string]string{
		"models/a.aml": "Model a {\n  data_source_name: 'demo'\n}\n",
		"models/b.aml": "Model b {\n  data_source_name: \"events\"\n}\n",
		"models/c.aml": "Model c {\n  data_source_name: 'logs'\n}\n",
	})
	dir := newRepo(t, map[string]string{".anfra/data_sources.yml": "data_sources:\n  demo:\n    type: postgresql\n"})
	res := mustPull(t, dir, PullOptions{SetOrg: url})
	want := "This repo uses data sources that aren't in .anfra/data_sources.yml yet: events (clickhouse) and logs. Add them, with their connections."
	if !slices.Contains(res.Hints, want) {
		t.Errorf("hints = %q", res.Hints)
	}
}

// A pull into a git working tree with uncommitted changes says how to keep
// them apart from the pull's.
func TestPullHintsUncommittedChanges(t *testing.T) {
	requireGit(t)
	s, url := newServer(t)
	s.publishAs("bob@example.com", map[string]string{"a.aml": "a\n", "r.aml": "r\n"})
	dir := newRepo(t, map[string]string{"a.aml": "a\n"})
	git(t, dir, "init", "-q")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "start")
	if res := mustPull(t, dir, PullOptions{SetOrg: url}); slices.Contains(res.Hints, uncommittedHint()) {
		t.Errorf("clean tree: %q", res.Hints)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "pulled")
	s.publishAs("bob@example.com", map[string]string{"a.aml": "a\n", "r.aml": "r2\n"})
	write(t, dir, map[string]string{"mine.aml": "m\n"})
	if res := mustPull(t, dir, PullOptions{}); !slices.Contains(res.Hints, uncommittedHint()) {
		t.Errorf("uncommitted: %q", res.Hints)
	}
}
