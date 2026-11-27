package publish

import (
	"strings"
	"testing"
)

// A folder saved on Windows (CRLF) pulls like one saved on a Mac: files
// unedited here update, a change on both sides merges, and each file keeps
// its own line ending; a new file is LF, a binary file as it is.
func TestPullLineEndings(t *testing.T) {
	srv, url := newServer(t)
	png := "PNG\x00\r\n1"
	dir := newRepo(t, map[string]string{"models/m.aml": "1\n2\n3\n4\n5\n", "models/b.aml": "b\n", "apps/x/logo.png": png})
	if _, _, err := run(t, dir, Options{SetOrg: url}, nil); err != nil {
		t.Fatal(err)
	}
	// Saved on Windows: CRLF, m's first line edited, b only re-saved.
	write(t, dir, map[string]string{"models/m.aml": "ONE\r\n2\r\n3\r\n4\r\n5\r\n", "models/b.aml": "b\r\n"})
	srv.publishAs("bob@acme.com", map[string]string{
		"models/m.aml": "1\n2\n3\n4\nFIVE\n", "models/b.aml": "B\n", "models/c.aml": "c\n", "apps/x/logo.png": png + "2",
	})
	res := mustPull(t, dir, PullOptions{})
	if len(res.Conflicts) != 0 {
		t.Fatalf("conflicts %+v", res.Conflicts)
	}
	for p, want := range map[string]string{
		"models/m.aml":    "ONE\r\n2\r\n3\r\n4\r\nFIVE\r\n", // merged, still CRLF
		"models/b.aml":    "B\r\n",                          // updated, still CRLF
		"models/c.aml":    "c\n",                            // new: as the org has it
		"apps/x/logo.png": png + "2",                        // binary: as it is
	} {
		if got := read(t, dir, p); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
}

// A conflict in a CRLF file gets CRLF markers, counts as resolved once they're
// gone, and --abort puts the file back byte for byte.
func TestPullLineEndingsConflict(t *testing.T) {
	srv, url := newServer(t)
	dir := newRepo(t, map[string]string{"models/a.aml": "x\ny\n"})
	if _, _, err := run(t, dir, Options{SetOrg: url}, nil); err != nil {
		t.Fatal(err)
	}
	srv.publishAs("bob@acme.com", map[string]string{"models/a.aml": "x\nORG\n"})
	write(t, dir, map[string]string{"models/a.aml": "x\r\nLOCAL\r\n"})

	res := mustPull(t, dir, PullOptions{})
	got := read(t, dir, "models/a.aml")
	if len(res.Conflicts) != 1 || !strings.Contains(got, "<<<<<<<") || strings.Count(got, "\n") != strings.Count(got, "\r\n") {
		t.Fatalf("conflicts %d, file %q: want one conflict, its markers in CRLF", len(res.Conflicts), got)
	}
	mustPull(t, dir, PullOptions{Abort: true})
	if got := read(t, dir, "models/a.aml"); got != "x\r\nLOCAL\r\n" {
		t.Errorf("after --abort %q, want the CRLF file back as it was", got)
	}

	mustPull(t, dir, PullOptions{})
	write(t, dir, map[string]string{"models/a.aml": "x\r\nBOTH\r\n"})
	if _, _, err := run(t, dir, Options{}, nil); err != nil {
		t.Errorf("publish after resolving the CRLF file: %v", err)
	}
}
