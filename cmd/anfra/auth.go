package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/shared/apperr"
)

// authLoginResult is auth login's result: the org signed in to.
type authLoginResult struct {
	Message string `json:"message" doc:"the outcome, in a sentence"`
	OrgURL  string `json:"org_url" doc:"the org signed in to"`
}

// authLogoutResult is auth logout's result: the org signed out of, here and
// on its server.
type authLogoutResult struct {
	Message                   string   `json:"message" doc:"the outcome, in a sentence"`
	OrgURL                    string   `json:"org_url" doc:"the org signed out of"`
	LocalRemoved              bool     `json:"local_removed" doc:"whether no login for the org is saved here anymore"`
	RemoteRevocationConfirmed bool     `json:"remote_revocation_confirmed" doc:"whether the org's server revoked the login"`
	Hints                     []string `json:"hints,omitempty" doc:"what to do next, if any"`
}

func newAuthCmd() *cobra.Command {
	auth := &cobra.Command{Use: "auth", Short: "Manage organization authentication"}
	// --json and --non-interactive are the root's: the one sets the answer's
	// format, the other whether to ask (newAuthTerminal).
	terminalFor := func(cmd *cobra.Command) *authTerminal {
		t := newAuthTerminal(cmd.InOrStdin(), cmd.ErrOrStderr())
		t.machine = structuredAsked()
		return t
	}
	auth.AddCommand(&cobra.Command{
		Use: "login [<org URL>]", Short: "Authenticate with an organization",
		Long: "Authenticate through your browser. Choose an organization or enter its URL.\n\nSupply an organization URL when using --non-interactive or without a\nterminal. A server's address alone, such as https://bi.example.com, works too:\nyou choose the organization in the browser. Browser approval is required. It\nsaves the org's login on this machine, and never changes the repo's publish\ndestination.\n\n" +
			"Its result is message, the outcome, and org_url, the org signed in to.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t := terminalFor(cmd)
			var address string
			if len(args) != 0 {
				address = args[0]
			} else {
				var err error
				address, err = t.loginAddress(cmd.Context(), "Authenticate with an organization")
				if err != nil {
					return err
				}
			}
			org, err := t.loginOrg(cmd.Context(), address)
			if err != nil {
				return err
			}
			return writeResult(cmd, authLoginResult{Message: fmt.Sprintf("Signed in to %s.", org), OrgURL: org.String()})
		},
	})
	auth.AddCommand(&cobra.Command{
		Use: "logout [<org URL>]", Short: "Sign out of an organization",
		Long: "Choose a saved organization to sign out of, or supply its URL.\n\nThis manages authentication on this machine and ignores the repo's linked\norganization. An organization URL is required without an interactive terminal.\n\n" +
			"Its result is message, the outcome; org_url; local_removed, whether the login saved here is\n" +
			"gone; and remote_revocation_confirmed, whether the server revoked it. When the server couldn't,\n" +
			"it fails with command_failed, whose details are that result.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t := terminalFor(cmd)
			org, err := logoutOrg(cmd.Context(), args, t)
			if err != nil {
				return err
			}
			var result cliauth.LogoutResult
			err = t.activity(cmd.Context(), "Signing out of "+org.String(), func(ctx context.Context, _, _ func(string)) error {
				var logoutErr error
				result, logoutErr = (cliauth.Manager{}).Logout(ctx, org)
				return logoutErr
			})
			res := authLogoutResult{OrgURL: org.String(), LocalRemoved: result.LocalRemoved, RemoteRevocationConfirmed: result.RemoteRevocationConfirmed}
			switch {
			case err != nil && result.LocalRemoved && !result.RemoteRevocationConfirmed:
				// Gone from here, maybe not from the server: a credential left live.
				res.Message = fmt.Sprintf("Signed out of %s on this machine, but couldn't revoke the login on the server.", org)
				res.Hints = []string{fmt.Sprintf("Revoke it at %s/account/connected-apps.", org.Origin)}
				return commandFailedWith(res.Message, res)
			case err != nil:
				return err
			case result.RemoteRevocationConfirmed:
				res.Message = fmt.Sprintf("Signed out of %s.", org)
			default:
				res.Message = fmt.Sprintf("No saved login for %s: nothing to sign out of.", org)
			}
			return writeResult(cmd, res)
		},
	})
	auth.AddCommand(&cobra.Command{
		Use: "status", Short: "Show the organizations signed in to on this machine",
		Long: "Show the organizations signed in to on this machine, and whether the current repo's is one\n" +
			"of them. It reads what's saved here and asks no server: a login revoked on its server still\n" +
			"shows as valid.\n\n" +
			"Its result opens with message, the outcome. orgs lists each org with a saved login, and the\n" +
			"current repo's without one: its name and url, saved_login (valid, invalid when it can't be\n" +
			"used, or missing), and current_repo, whether the current repo publishes to it. hints, last,\n" +
			"are what to do next.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := authStatus(cmd.Context(), ".")
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	})
	for _, c := range auth.Commands() {
		switch c.Name() {
		case "login":
			resultIs[authLoginResult](c)
		case "logout":
			resultIs[authLogoutResult](c)
		case "status":
			resultIs[authStatusResult](c)
		}
		markPrettyOutput(c)
		if c.Name() == "login" || c.Name() == "logout" {
			markInteractable(c) // which org, which login
		}
	}
	return auth
}

// defaultCloudURL is optionally set by the build with -X main.defaultCloudURL:
// the servers bare `anfra auth login` offers, as cloudapi.ParseCloudServers
// reads them.
var defaultCloudURL string

// defaultCloudSupportEmail is optionally set by the build with
// -X main.defaultCloudSupportEmail: whom login tells users of the build's
// servers to email for another region.
var defaultCloudSupportEmail string

// loginSupportEmail is defaultCloudSupportEmail when login offers the build's
// servers. A runtime ANFRA_CLOUD_URL replaces them, and the build's support
// doesn't cover those.
func loginSupportEmail() string {
	if _, set := os.LookupEnv("ANFRA_CLOUD_URL"); set || defaultCloudURL == "" {
		return ""
	}
	return defaultCloudSupportEmail
}

// configuredCloudServers are the servers bare `anfra auth login` offers: the
// runtime ANFRA_CLOUD_URL's when it's set, even to nothing, or else the build's.
func configuredCloudServers() ([]cloudapi.CloudServer, error) {
	raw, set := os.LookupEnv("ANFRA_CLOUD_URL")
	if !set {
		raw = defaultCloudURL
	}
	servers, err := cloudapi.ParseCloudServers(raw)
	if err != nil {
		return nil, refused("ANFRA_CLOUD_URL", "invalid", "ANFRA_CLOUD_URL: "+err.Error())
	}
	return servers, nil
}

func loginServer(address string) (origin, hint string, err error) {
	if address == "" {
		var servers []cloudapi.CloudServer
		if servers, err = configuredCloudServers(); err != nil {
			return "", "", err
		}
		var origins []string
		for _, server := range servers {
			if server.Origin != "" {
				origins = append(origins, server.Origin)
			}
		}
		switch len(origins) {
		case 0:
			return "", "", refused("org URL", "required", "Provide an organization URL: anfra auth login https://<server>/o/<org>.")
		case 1:
			return origins[0], "", nil
		default:
			return "", "", refused("org URL", "required", fmt.Sprintf("Provide an organization URL, such as anfra auth login %s/o/<org>. There's more than one server to sign in to (%s), so anfra can't choose one for you.", origins[0], strings.Join(origins, ", ")))
		}
	}
	// A server's address alone works: its consent page chooses the org.
	origin, hint, err = cloudapi.ParseAddress(address)
	if err != nil {
		return "", "", refused("org URL", "invalid", err.Error())
	}
	return origin, hint, nil
}

// openAuthBrowser passes the authorization URL directly to the platform launcher,
// never a shell, terminal diagnostic or telemetry event.
func openAuthBrowser(ctx context.Context, raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", raw)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", raw)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", raw)
	}
	return cmd.Run()
}

type authTerminal struct {
	in                   io.Reader
	out                  io.Writer
	interactive, machine bool
}

// newAuthTerminal is a command's terminal, to ask on when the run is
// interactive. A command that asks through it is marked interactable
// (markInteractable), for its --help.
func newAuthTerminal(in io.Reader, out io.Writer) *authTerminal {
	return &authTerminal{in: in, out: out, interactive: !output.nonInteractive && cliui.CanInteract(in, out)}
}

func loginChoices(orgs []cliauth.StoredLogin) []cliui.OrgChoice {
	choices := make([]cliui.OrgChoice, len(orgs))
	for i, org := range orgs {
		choices[i] = cliui.OrgChoice{Name: org.Name, URL: org.Org.String()}
	}
	return choices
}

func (t *authTerminal) loginAddress(ctx context.Context, title string) (string, error) {
	if !t.interactive {
		return "", refused("org URL", "required", "Provide an organization URL: anfra auth login https://<server>/o/<org>.")
	}
	servers, err := configuredCloudServers()
	if err != nil {
		return "", err
	}
	_, address, err := cliui.PickOrg(ctx, t.in, t.out, title, nil, &cliui.CloudChoices{Servers: servers, SupportEmail: loginSupportEmail()}, true)
	return address, err
}

func (t *authTerminal) activity(ctx context.Context, label string, work func(context.Context, func(string), func(string)) error) error {
	if t.interactive {
		return cliui.RunActivity(ctx, t.in, t.out, label, work)
	}
	var mu sync.Mutex
	printMessage := func(message string) {
		mu.Lock()
		defer mu.Unlock()
		if !t.machine {
			fmt.Fprintln(t.out, cliui.SafeText(message))
		}
	}
	return work(ctx, printMessage, printMessage)
}

func (t *authTerminal) loginOrg(ctx context.Context, address string) (cloudapi.Org, error) {
	origin, hint, err := loginServer(address)
	if err != nil {
		return cloudapi.Org{}, err
	}
	var org cloudapi.Org
	err = t.activity(ctx, string(cliauth.LoginConnecting), func(ctx context.Context, progress, printMessage func(string)) error {
		var loginErr error
		manager := cliauth.Manager{LoginProgress: func(stage cliauth.LoginStage) { progress(string(stage)) }}
		org, loginErr = manager.Login(ctx, origin, hint, func(ctx context.Context, address string) error {
			if t.machine {
				// The link isn't printed here, so it stays out of logs and
				// transcripts.
				err := openAuthBrowser(ctx, address)
				if err != nil && ctx.Err() == nil {
					return clierr.Precondition("Couldn't open a browser to approve the authentication. Run `anfra auth login` in a terminal (without %s), which prints a link to open instead.", structuredFlag())
				}
				return err
			}
			authorizationURL, err := url.Parse(address)
			if err != nil {
				return fmt.Errorf("could not parse authorization URL: %w", err)
			}
			printMessage("Callback URL: " + authorizationURL.Query().Get("redirect_uri"))
			printMessage("Opening browser to authenticate…")
			printMessage("If the browser didn't open, visit: " + address)
			if err := openAuthBrowser(ctx, address); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				printMessage("Could not open your browser. Open the URL above to continue authenticating.")
			}
			return nil
		})
		return loginErr
	})
	if errors.Is(err, context.Canceled) {
		return org, apperr.New(clierr.Interrupted, "Authentication canceled.")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return org, clierr.Precondition("Authentication timed out before it was approved in the browser. Try again.")
	}
	return org, err
}

func (t *authTerminal) pickStored(ctx context.Context, orgs []cliauth.StoredLogin) (cloudapi.Org, error) {
	index, _, err := cliui.PickOrg(ctx, t.in, t.out, "Choose an organization to sign out", loginChoices(orgs), nil, false)
	if err != nil {
		return cloudapi.Org{}, err
	}
	return orgs[index].Org, nil
}

func logoutOrg(ctx context.Context, args []string, terminal *authTerminal) (cloudapi.Org, error) {
	if len(args) != 0 {
		org, err := cloudapi.ParseOrgAddress(args[0])
		if err != nil {
			return cloudapi.Org{}, refused("org URL", "invalid", err.Error())
		}
		return org, nil
	}
	if !terminal.interactive {
		return cloudapi.Org{}, refused("org URL", "required", "Provide an organization URL: anfra auth logout https://<server>/o/<org>.")
	}
	orgs, err := (cliauth.Manager{}).StoredLogins(ctx)
	if err != nil {
		return cloudapi.Org{}, err
	}
	if len(orgs) == 0 {
		return cloudapi.Org{}, apperr.New(apperr.NotFound, "No saved login to sign out of.")
	}
	return terminal.pickStored(ctx, orgs)
}

func (t *authTerminal) selectPublishOrg(ctx context.Context) (cloudapi.Org, error) {
	orgs, err := (cliauth.Manager{}).StoredLogins(ctx)
	if err != nil {
		return cloudapi.Org{}, err
	}
	title := "This repo isn't linked to an organization yet.\nAuthenticate to publish."
	if len(orgs) > 0 {
		choices := loginChoices(orgs)
		choices = append(choices, cliui.OrgChoice{Name: "Authenticate with another organization"})
		index, _, err := cliui.PickOrg(ctx, t.in, t.out, "Choose where to publish", choices, nil, false)
		if err != nil {
			return cloudapi.Org{}, err
		}
		if index < len(orgs) {
			return orgs[index].Org, nil
		}
		title = "Authenticate with an organization"
	}
	address, err := t.loginAddress(ctx, title)
	if err != nil {
		return cloudapi.Org{}, err
	}
	org, err := t.loginOrg(ctx, address)
	if err != nil {
		return cloudapi.Org{}, err
	}
	_, _, err = cliui.PickOrg(ctx, t.in, t.out, "Use this organization for publishing?", []cliui.OrgChoice{{Name: org.Slug, URL: org.String()}}, nil, false)
	return org, err
}

func (t *authTerminal) recoverPublishOrg(ctx context.Context, linked cloudapi.Org, cause error) (cloudapi.Org, error) {
	if !errors.Is(cause, cliauth.ErrAuthenticationRequired) {
		return cloudapi.Org{}, cause
	}
	orgs, err := (cliauth.Manager{}).StoredLogins(ctx)
	if err != nil {
		return cloudapi.Org{}, err
	}
	choices := []cliui.OrgChoice{{Name: "Authenticate with " + linked.Slug, URL: linked.String()}}
	alternatives := make([]cliauth.StoredLogin, 0, len(orgs))
	for _, login := range orgs {
		if login.Org == linked {
			continue
		}
		alternatives = append(alternatives, login)
	}
	choices = append(choices, cliui.OrgChoice{Name: "Switch to another organization"})
	title := fmt.Sprintf("This repo publishes to %s.\nYou're not authenticated with this organization.", linked.String())
	index, _, err := cliui.PickOrg(ctx, t.in, t.out, title, choices, nil, false)
	if err != nil {
		return cloudapi.Org{}, err
	}
	var selected cloudapi.Org
	if index == 0 {
		selected, err = t.loginOrg(ctx, linked.String())
	} else {
		storedIndex := len(alternatives)
		if len(alternatives) > 0 {
			choices = append(loginChoices(alternatives), cliui.OrgChoice{Name: "Authenticate with a different organization"})
			storedIndex, _, err = cliui.PickOrg(ctx, t.in, t.out, "Choose an organization to publish to", choices, nil, false)
			if err != nil {
				return cloudapi.Org{}, err
			}
		}
		if storedIndex < len(alternatives) {
			selected = alternatives[storedIndex].Org
		} else {
			var address string
			address, err = t.loginAddress(ctx, "Authenticate with an organization")
			if err == nil {
				selected, err = t.loginOrg(ctx, address)
			}
		}
	}
	if err != nil {
		return cloudapi.Org{}, err
	}
	if selected != linked {
		title = fmt.Sprintf("Change this repo's publish destination from %s to %s?\nThis updates org.url in .anfra/config.yml.", linked.String(), selected.String())
		index, _, err = cliui.PickOrg(ctx, t.in, t.out, title, []cliui.OrgChoice{{Name: "Cancel"}, {Name: "Change destination and publish"}}, nil, false)
		if err != nil {
			return cloudapi.Org{}, err
		}
		if index == 0 {
			return cloudapi.Org{}, context.Canceled
		}
	}
	return selected, nil
}
