package app

import (
	"context"
	"errors"
	"testing"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/repo"
)

// A host must allow local repos for a command to run on one: every command, the
// ones added later included, is refused on a local repo without AllowLocalRepo,
// before it does anything. A platform serving managed repos therefore cannot
// run one on a directory by mistake.
func TestEveryCommandRefusesADisallowedLocalRepo(t *testing.T) {
	local := repo.Resolve(t.TempDir()).Repo()
	for _, c := range Commands {
		cc := command.CommandContext{Repo: local, DataPerms: dataperm.Ungoverned()}
		_, err := Invoke(context.Background(), cc, c.Name(), []byte(`{}`))
		if !errors.Is(err, errcode.LocalRepoNotAllowed) {
			t.Errorf("%s on a disallowed local repo: %v, want local_repo_not_allowed", c.Name(), err)
		}
	}
}

// The guard is about local repos only: allowed, a local repo runs, and a managed
// repo needs no allowance. (Neither has sidecars here, so a command that needs
// one stops there instead: past the guard.)
func TestTheGuardLetsAllowedAndManagedReposThrough(t *testing.T) {
	for name, cc := range map[string]command.CommandContext{
		"allowed local": {Repo: repo.Resolve(t.TempDir()).Repo(), AllowLocalRepo: true, DataPerms: dataperm.Ungoverned()},
		"managed":       {Repo: managedRepo(t), DataPerms: dataperm.Ungoverned()},
	} {
		if _, err := Invoke(context.Background(), cc, "validate", []byte(`{}`)); errors.Is(err, errcode.LocalRepoNotAllowed) || !errors.Is(err, errcode.SidecarUnavailable) {
			t.Errorf("%s: %v, want past the guard, to sidecar_unavailable", name, err)
		}
	}
}

// Undecided data permissions are still the first refusal: the guard does not hide
// the other host mistake.
func TestDataPermsAreCheckedFirst(t *testing.T) {
	cc := command.CommandContext{Repo: repo.Resolve(t.TempDir()).Repo()}
	if _, err := Invoke(context.Background(), cc, "validate", []byte(`{}`)); !errors.Is(err, errcode.DataPermsMissing) {
		t.Errorf("got %v, want data_perms_missing", err)
	}
}
