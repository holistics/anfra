package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/repoconfig"
)

// statusRepoOrg reads the nearest config without crossing a Git, home or filesystem boundary.
// This ancestor lookup is intentionally not used by publish or logout.
func statusRepoOrg(dir string) (*string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(home); resolveErr == nil {
		home = resolved
	}
	for {
		_, err := os.Stat(repoconfig.Path(dir))
		if err == nil {
			config, err := repoconfig.Read(dir)
			if err != nil {
				return nil, clierr.Precondition("Can't read a valid repo configuration from %q: check its permissions and YAML syntax.", repoconfig.Path(dir))
			}
			if config.Org.URL == "" {
				return nil, nil
			}
			org, err := cloudapi.ParseOrg(config.Org.URL)
			if err != nil {
				return nil, clierr.Precondition("org.url in %q isn't an organization URL such as https://<server>/o/<org>.", repoconfig.Path(dir))
			}
			canonical := org.String()
			return &canonical, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		_, gitErr := os.Lstat(filepath.Join(dir, ".git"))
		if gitErr != nil && !errors.Is(gitErr, os.ErrNotExist) {
			return nil, gitErr
		}
		parent := filepath.Dir(dir)
		if gitErr == nil || dir == home || parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// authStatusResult is auth status's result: the logins saved on this machine,
// read offline (docs/usage/publishing.md).
type authStatusResult struct {
	// Message is the outcome, in a sentence.
	Message string          `json:"message"`
	Orgs    []authStatusOrg `json:"orgs"`
	// Hints are what to do next; absent when nothing is.
	Hints []string `json:"hints,omitempty"`
}

// authStatusOrg is an org with a saved login, or the current repo's without
// one.
type authStatusOrg struct {
	// Name is the org's, as its login saved it; absent without a login.
	Name string `json:"name,omitempty"`
	URL  string `json:"url"`
	// SavedLogin is valid, invalid (expired, an interrupted refresh, a
	// damaged entry: it can't be used), or missing: none is saved.
	SavedLogin string `json:"saved_login" pretty:",tone=valid:good invalid:warn missing:warn"`
	// CurrentRepo: the current repo publishes to this org.
	CurrentRepo bool `json:"current_repo"`
}

// authStatus is the logins saved on this machine, and whether the repo at dir
// publishes to one of them. It asks no server.
func authStatus(ctx context.Context, dir string) (authStatusResult, error) {
	repoURL, err := statusRepoOrg(dir)
	if err != nil {
		return authStatusResult{}, err
	}
	logins, err := (cliauth.Manager{}).StoredLogins(ctx)
	if err != nil {
		return authStatusResult{}, err
	}
	res := authStatusResult{Orgs: make([]authStatusOrg, 0, len(logins)+1)}
	var repoOrg *authStatusOrg
	valid := 0
	for _, login := range logins {
		o := authStatusOrg{Name: cliui.SafeText(login.Name), URL: login.Org.String(), SavedLogin: "valid"}
		if login.LocalState == "sign_in_again" {
			o.SavedLogin = "invalid"
			res.Hints = append(res.Hints, fmt.Sprintf("Run `anfra auth login %s` to sign in to %s again: its saved login can't be used.", o.URL, o.Name))
		} else {
			valid++
		}
		o.CurrentRepo = repoURL != nil && *repoURL == o.URL
		res.Orgs = append(res.Orgs, o)
		if o.CurrentRepo {
			repoOrg = &res.Orgs[len(res.Orgs)-1]
		}
	}
	if repoURL != nil && repoOrg == nil {
		res.Orgs = append(res.Orgs, authStatusOrg{URL: *repoURL, SavedLogin: "missing", CurrentRepo: true})
		repoOrg = &res.Orgs[len(res.Orgs)-1]
	}
	if repoOrg != nil && repoOrg.SavedLogin == "missing" {
		res.Hints = append(res.Hints, fmt.Sprintf("Run `anfra auth login %s` to sign in to the current repo's org.", repoOrg.URL))
	}

	signedIn := "Not signed in to any org"
	if valid > 0 {
		signedIn = fmt.Sprintf("Signed in to %d %s", valid, plural(valid, "org", "orgs"))
	}
	switch {
	case repoOrg == nil:
		res.Message = signedIn + "."
	case repoOrg.SavedLogin == "valid":
		res.Message = fmt.Sprintf("%s, including the current repo's, %s.", signedIn, repoOrg.Name)
	case valid == 0:
		res.Message = fmt.Sprintf("%s, the current repo's (%s) included.", signedIn, repoOrg.URL)
	default:
		res.Message = fmt.Sprintf("%s, but not the current repo's, %s.", signedIn, repoOrg.URL)
	}
	if len(logins) == 0 && repoOrg == nil {
		res.Hints = append(res.Hints, "Run `anfra auth login` to sign in.")
	}
	return res, nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
