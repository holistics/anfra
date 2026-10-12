package anfranode

import (
	"context"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
)

// RefTo names a repo as anfra-node reads it: a local repo by its directory, a
// managed repo by its revision, each with its id, never both.
func TestRefTo(t *testing.T) {
	local := repo.Local{Dir: "/repos/acme", ID: "acme-1a2b3c4d"}.Repo()
	rev, err := repo.Managed("0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b",
		"0b6f1c2e-8d4a-4f3b-9a1e-2c3d4e5f6a7b@0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if got := RefTo(local); got != (RepoRef{RepoID: "acme-1a2b3c4d", RepoPath: "/repos/acme"}) || got.check() != nil {
		t.Errorf("local: %+v, %v", got, got.check())
	}
	if got := RefTo(rev); got.RepoPath != "" || got.Revision == "" || got.RepoID == "" || got.check() != nil {
		t.Errorf("revision: %+v, %v", got, got.check())
	}
}

// A request that names neither or both, or no id, is refused before it leaves the
// process: with a message naming the mistake, not a connection error.
func TestRepoRefIsCheckedBeforeSending(t *testing.T) {
	c := NewClientHTTP("http://127.0.0.1:1") // nothing listens: reaching it would fail differently
	for name, ref := range map[string]RepoRef{
		"no id":     {RepoPath: "/repos/acme"},
		"neither":   {RepoID: "acme"},
		"both":      {RepoID: "acme", RepoPath: "/repos/acme", Revision: "acme@0123456789abcdef0123456789abcdef01234567"},
		"zero repo": RefTo(repo.Repo{}),
	} {
		_, err := c.ValidateAML(context.Background(), ValidateAMLRequest{RepoRef: ref})
		if err == nil || !strings.Contains(err.Error(), "anfra-node: ") || strings.Contains(err.Error(), "connect") {
			t.Errorf("%s: %v, want a refusal before sending", name, err)
		}
	}
}
