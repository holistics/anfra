package command

import (
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
)

// A local repo's log is on its reader's machine, so a failure names the
// command that shows it; a managed repo's host keeps its logs, so it names
// nothing.
func TestLogHint(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	if got := WithHint("It failed.", LogHint(repo.Resolve(t.TempDir()).Repo())); got != "It failed. Run `anfra logs` to see what failed." {
		t.Errorf("a local repo: %q", got)
	}
	managed, err := repo.Managed("r1", "acme/models@"+strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	if got := WithHint("It failed.", LogHint(managed)); got != "It failed." {
		t.Errorf("a managed repo: %q", got)
	}
}
