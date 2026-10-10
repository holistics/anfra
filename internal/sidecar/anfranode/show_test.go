package anfranode

import (
	"testing"

	"github.com/holistics/anfra/shared/jsonkit"
)

// A shown object encodes within the answer's encoder, so the answer's encoding
// reaches inside it: a repo with no datasets has [], not null.
func TestShowObjectEncodesAsTheAnswer(t *testing.T) {
	b, err := jsonkit.Marshal(ShowResult{Object: ShowObject{Repo: &ShowRepo{Kind: "repo"}}})
	if want := `{"object":{"kind":"repo","datasets":[]},"diagnostics":[]}`; err != nil || string(b) != want {
		t.Errorf("got %s %v, want %s", b, err, want)
	}
}
