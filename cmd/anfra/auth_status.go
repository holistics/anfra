package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/shared/jsonkit"
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
				return nil, errors.New("auth status: cannot read valid .anfra/config.yml")
			}
			if config.Org.URL == "" {
				return nil, nil
			}
			org, err := cloudapi.ParseOrg(config.Org.URL)
			if err != nil {
				return nil, errors.New("auth status: invalid .anfra/config.yml org.url")
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

type authStatusOrg struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	RepoActive bool   `json:"repo_active"`
	LocalState string `json:"local_state"`
}

func printAuthStatus(ctx context.Context, dir string, out io.Writer, machine, noInteractive bool) error {
	repoURL, err := statusRepoOrg(dir)
	if err != nil {
		return err
	}
	logins, err := (cliauth.Manager{}).StoredLogins(ctx)
	if err != nil {
		return err
	}
	result := struct {
		Orgs          []authStatusOrg `json:"orgs"`
		RepoOrgURL    *string         `json:"repo_org_url"`
		RemoteChecked bool            `json:"remote_checked"`
	}{Orgs: make([]authStatusOrg, 0, len(logins)), RepoOrgURL: repoURL}
	found := false
	for _, login := range logins {
		active := repoURL != nil && *repoURL == login.Org.String()
		found = found || active
		result.Orgs = append(result.Orgs, authStatusOrg{Name: cliui.SafeText(login.Name), URL: login.Org.String(), RepoActive: active, LocalState: login.LocalState})
	}
	if machine {
		return jsonkit.MarshalWrite(out, result)
	}
	styles := cliui.TerminalStyles{}
	if !noInteractive {
		styles = cliui.NewTerminalStyles(out)
	}
	fmt.Fprintln(out, styles.Heading.Render("Authenticated organizations"))
	fmt.Fprintln(out)
	if len(result.Orgs) == 0 {
		_, err = fmt.Fprintln(out, "No authenticated organizations. Run anfra auth login <org URL> to sign in.")
	}
	for i, org := range result.Orgs {
		marker := ""
		if org.RepoActive {
			marker = "  " + styles.Accent.Render("Publish destination")
		}
		if org.LocalState == "sign_in_again" {
			marker += "  " + styles.Warning.Render("Sign in again")
		}
		fmt.Fprintf(out, "  %d. %s  %s%s\n", i+1, styles.Heading.Render(org.Name), styles.Muted.Render(org.URL), marker)
		if org.LocalState == "sign_in_again" {
			fmt.Fprintf(out, "    anfra auth login %s\n", org.URL)
		}
	}
	if repoURL != nil && !found {
		_, err = fmt.Fprintf(out, "\nRepo destination: %s  %s\nRun anfra auth login %s\n", *repoURL, styles.Warning.Render("Not signed in"), *repoURL)
	}
	return err
}
