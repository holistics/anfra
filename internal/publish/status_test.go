package publish

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func status(t *testing.T, dir string) StatusResult {
	t.Helper()
	env, _, _ := testEnv(nil)
	res, err := Status(context.Background(), dir, env)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func paths(cs []FileChange) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Path+" "+c.Change)
	}
	return out
}

// Without an org, status says so, and how to set one.
func TestStatusWithoutOrg(t *testing.T) {
	dir := newRepo(t, map[string]string{"a.aml": "a"})
	res := status(t, dir)
	if res.Org != nil || !strings.Contains(res.Message, "no org") || len(res.Hints) == 0 {
		t.Errorf("%+v", res)
	}
}

// Status compares the folder with what it last published and what the org
// has now, and changes nothing.
func TestStatus(t *testing.T) {
	s, url := newServer(t)
	dir := newRepo(t, map[string]string{"a.aml": "a\n", "b.aml": "b\n"})
	if _, _, err := run(t, dir, Options{SetOrg: url}, never(t)); err != nil {
		t.Fatal(err)
	}
	res := status(t, dir)
	if len(res.Incoming)+len(res.Unpublished) != 0 || !strings.HasPrefix(res.Message, "Everything here is published") {
		t.Errorf("just published: %+v", res)
	}

	write(t, dir, map[string]string{"c.aml": "c\n", "b.aml": "b2\n"})
	res = status(t, dir)
	if got := paths(res.Unpublished); !slices.Equal(got, []string{"b.aml updated", "c.aml added"}) || !strings.Contains(res.Message, "2 files here aren't published yet") ||
		!slices.Contains(res.Hints, "Run `anfra publish` to publish them.") {
		t.Errorf("with work of its own: %v %+v", got, res)
	}

	s.publishAs("bob@example.com", map[string]string{"a.aml": "a\n", "b.aml": "bob\n", "r.aml": "r\n"})
	res = status(t, dir)
	if got := paths(res.Incoming); !slices.Equal(got, []string{"b.aml merged", "r.aml added"}) {
		t.Errorf("incoming = %v", got)
	}
	if !strings.Contains(res.Message, "bob@example.com published to") || res.Base == "" || res.PublishedBy != "bob@example.com" ||
		!slices.ContainsFunc(res.Hints, func(h string) bool { return strings.HasPrefix(h, "b.aml changed both here and on the org") }) {
		t.Errorf("behind: %+v", res)
	}
	if content, _ := os.ReadFile(filepath.Join(dir, "b.aml")); string(content) != "b2\n" {
		t.Errorf("status changed b.aml to %q", content)
	}
}

// With no record of an earlier sync, files only one side has, or that
// differ, are compared from nothing.
func TestStatusWithoutBase(t *testing.T) {
	s, url := newServer(t)
	s.publishAs("bob@example.com", map[string]string{"a.aml": "a\n", "b.aml": "bob\n"})
	dir := newRepo(t, map[string]string{"a.aml": "a\n", "b.aml": "ada\n", "c.aml": "c\n"})
	write(t, dir, map[string]string{".anfra/config.yml": "org:\n  url: " + url + "\n"})
	res := status(t, dir)
	if res.Base != "" || !strings.Contains(res.Message, "no record") ||
		!slices.Equal(paths(res.Incoming), []string{"b.aml merged"}) || !slices.Equal(paths(res.Unpublished), []string{"b.aml added", "c.aml added"}) {
		t.Errorf("%v %v %+v", paths(res.Incoming), paths(res.Unpublished), res)
	}
}

// A pull's unresolved conflicts are listed, and the folder is compared from
// that pull's revision.
func TestStatusConflicts(t *testing.T) {
	s, url := newServer(t)
	dir := newRepo(t, map[string]string{"a.aml": "a\n"})
	if _, _, err := run(t, dir, Options{SetOrg: url}, never(t)); err != nil {
		t.Fatal(err)
	}
	s.publishAs("bob@example.com", map[string]string{"a.aml": "bob\n"})
	write(t, dir, map[string]string{"a.aml": "ada\n"})
	mustPull(t, dir, PullOptions{})
	res := status(t, dir)
	if len(res.Conflicts["content"]) != 1 || len(res.Incoming) != 0 || !strings.Contains(res.Message, "1 conflict from the last pull is left to resolve") ||
		!slices.Contains(res.Hints, "To undo the last pull, run `anfra pull --abort`.") {
		t.Errorf("%+v", res)
	}
}
