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
				return nil, fmt.Errorf("cannot read valid repo configuration from %q; check file permissions and YAML syntax", repoconfig.Path(dir))
			}
			if config.Org.URL == "" {
				return nil, nil
			}
			org, err := cloudapi.ParseOrg(config.Org.URL)
			if err != nil {
				return nil, fmt.Errorf("invalid org.url in %q; use an organization URL such as https://<server>/o/<org>", repoconfig.Path(dir))
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
		if repoURL != nil {
			_, err = fmt.Fprintln(out, "No authenticated organizations.")
		} else {
			_, err = fmt.Fprintf(out, "No authenticated organizations. Run %s to authenticate.\n", styles.Code.Render("anfra auth login"))
		}
	}
	numberWidth := len(fmt.Sprint(len(result.Orgs)))
	indent := fmt.Sprintf("%*s", numberWidth+4, "")
	for i, org := range result.Orgs {
		if i > 0 {
			fmt.Fprintln(out)
		}
		line := fmt.Sprintf("  %*d. %s", numberWidth, i+1, styles.Heading.Render(org.Name))
		if org.RepoActive {
			line = styles.Selection.Render(fmt.Sprintf("* %*d. %s  (current org of this repo)", numberWidth, i+1, org.Name))
		}
		fmt.Fprintln(out, line)
		fmt.Fprintln(out, indent+styles.Muted.Render(org.URL))
		if org.LocalState == "sign_in_again" {
			fmt.Fprintf(out, "%s%s %s %s\n", indent, styles.Warning.Render("Authenticate again:"), styles.Code.Render("anfra auth login"), styles.Link.Render(org.URL))
		}
	}
	if repoURL != nil && !found {
		_, err = fmt.Fprintf(out, "\nThis repo is configured to publish to %s  %s\nRun %s to authenticate\n", styles.Link.Render(*repoURL), styles.Warning.Render("Not authenticated"), styles.Code.Render(fmt.Sprintf("anfra auth login %s", *repoURL)))
	}
	return err
}
