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
				return nil, clierr.Precondition("Can't read this repo's settings: %v.", err)
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
	Message string          `json:"message" doc:"the outcome, in a sentence"`
	Orgs    []authStatusOrg `json:"orgs" doc:"each org with a saved login, and the current repo's org"`
	Hints   []string        `json:"hints,omitempty" doc:"what to do next, if any"`
}

// authStatusOrg is an org with a saved login, or the current repo's without
// one.
type authStatusOrg struct {
	Name        string `json:"name,omitempty" doc:"the org's name, from its saved login. Absent when no login is saved."`
	URL         string `json:"url" doc:"the org's address"`
	SavedLogin  string `json:"saved_login" doc:"the state of the org's saved login. Possible values: \"valid\", \"invalid\", \"missing\". \"invalid\" means it can't be used, for example because it has expired. \"missing\" means no login is saved for the org." pretty:",tone=valid:good invalid:warn missing:warn"`
	CurrentRepo bool   `json:"current_repo" doc:"whether the current repo publishes to this org"`
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
			res.Hints = append(res.Hints, fmt.Sprintf("Run `anfra auth login %s` to sign in to %s again.", o.URL, o.Name))
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
