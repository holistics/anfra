package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/hostedorigin"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/shared/jsonkit"
)

func newAuthCmd() *cobra.Command {
	var machine, noInteractive bool
	auth := &cobra.Command{Use: "auth", Short: "Manage organization authentication"}
	auth.PersistentFlags().BoolVar(&machine, "json", false, "print results as JSON without prompts")
	auth.PersistentFlags().BoolVar(&noInteractive, "no-interactive", false, "disable prompts and animations")
	terminalFor := func(cmd *cobra.Command) *authTerminal {
		t := newAuthTerminal(cmd.InOrStdin(), cmd.ErrOrStderr())
		t.interactive = t.interactive && !machine && !noInteractive
		t.machine = machine
		return t
	}
	auth.AddCommand(&cobra.Command{
		Use: "login [<org URL>]", Short: "Authenticate with an organization",
		Long: "Authenticate through your browser. Choose an organization or enter its URL.\n\nSupply an organization URL when using --json, --no-interactive, or a\nnon-interactive terminal. Browser approval is required.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t := terminalFor(cmd)
			var address string
			if len(args) != 0 {
				address = args[0]
			} else {
				var err error
				address, err = t.loginAddress(cmd.Context())
				if err != nil {
					return err
				}
			}
			org, err := t.loginOrg(cmd.Context(), address)
			if err != nil {
				return err
			}
			if machine {
				return jsonkit.MarshalWrite(cmd.OutOrStdout(), struct {
					OrgURL                 string `json:"org_url"`
					RepoDestinationChanged bool   `json:"repo_destination_changed"`
				}{OrgURL: org.String()})
			}
			styles := cliui.TerminalStyles{}
			if t.interactive {
				styles = cliui.NewTerminalStyles(cmd.OutOrStdout())
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s.\n", styles.Success.Render("Authenticated with"), org)
			return err
		},
	})
	auth.AddCommand(&cobra.Command{
		Use: "logout [<org URL>]", Short: "Sign out of an organization",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t := terminalFor(cmd)
			org, err := logoutOrg(cmd.Context(), ".", args, t)
			if err != nil {
				return err
			}
			var result cliauth.LogoutResult
			err = t.activity(cmd.Context(), "Signing out of "+org.String(), func(ctx context.Context, _, _ func(string)) error {
				var logoutErr error
				result, logoutErr = (cliauth.Manager{}).Logout(ctx, org)
				return logoutErr
			})
			if machine && (err == nil || result.LocalRemoved) {
				if writeErr := jsonkit.MarshalWrite(cmd.OutOrStdout(), result); writeErr != nil {
					return writeErr
				}
			}
			if err != nil {
				return err
			}
			if !machine {
				styles := cliui.TerminalStyles{}
				if t.interactive {
					styles = cliui.NewTerminalStyles(cmd.OutOrStdout())
				}
				if result.RemoteRevocationConfirmed {
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s.\n", styles.Success.Render("Signed out of"), org)
				} else {
					_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s.\n", styles.Warning.Render("No saved authentication for"), org)
				}
			}
			return err
		},
	})
	auth.AddCommand(&cobra.Command{
		Use: "status", Short: "Show authenticated organizations and this repo's publish destination",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printAuthStatus(cmd.Context(), ".", cmd.OutOrStdout(), machine, noInteractive)
		},
	})
	return auth
}

func loginServer(address string) (origin, hint string, err error) {
	if address == "" {
		origin, err = hostedorigin.HostedOrigin()
		return origin, "", err
	}
	org, err := cloudapi.ParseOrg(address)
	return org.Origin, org.Slug, err
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

func newAuthTerminal(in io.Reader, out io.Writer) *authTerminal {
	return &authTerminal{in: in, out: out, interactive: cliui.CanInteract(in, out)}
}

func loginChoices(orgs []cliauth.StoredLogin) []cliui.OrgChoice {
	choices := make([]cliui.OrgChoice, len(orgs))
	for i, org := range orgs {
		choices[i] = cliui.OrgChoice{Name: org.Name, URL: org.Org.String()}
	}
	return choices
}

func (t *authTerminal) loginAddress(ctx context.Context) (string, error) {
	if !t.interactive {
		return "", errors.New("provide an organization URL: anfra auth login https://<server>/o/<org>")
	}
	_, address, err := cliui.PickOrg(ctx, t.in, t.out, "Authenticate with an organization", nil, true, true)
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
				return openAuthBrowser(ctx, address)
			}
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
		return org, errors.New("authentication canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return org, errors.New("authentication timed out; run anfra auth login again and complete approval in the browser")
	}
	return org, err
}

func (t *authTerminal) pickStored(ctx context.Context, orgs []cliauth.StoredLogin) (cloudapi.Org, error) {
	index, _, err := cliui.PickOrg(ctx, t.in, t.out, "Choose an organization to sign out", loginChoices(orgs), false, false)
	if err != nil {
		return cloudapi.Org{}, err
	}
	return orgs[index].Org, nil
}

func logoutOrg(ctx context.Context, dir string, args []string, terminal *authTerminal) (cloudapi.Org, error) {
	if len(args) != 0 {
		return cloudapi.ParseOrg(args[0])
	}
	config, err := repoconfig.Read(dir)
	if err != nil {
		return cloudapi.Org{}, err
	}
	if config.Org.URL != "" {
		return cloudapi.ParseOrg(config.Org.URL)
	}
	if !terminal.interactive {
		return cloudapi.Org{}, errors.New("provide an organization URL: anfra auth logout https://<server>/o/<org>")
	}
	orgs, err := (cliauth.Manager{}).StoredLogins(ctx)
	if err != nil {
		return cloudapi.Org{}, err
	}
	if len(orgs) == 0 {
		return cloudapi.Org{}, errors.New("no saved authentication to sign out of; run anfra auth status to check this repo's publish destination")
	}
	return terminal.pickStored(ctx, orgs)
}

func (t *authTerminal) selectPublishOrg(ctx context.Context) (cloudapi.Org, error) {
	orgs, err := (cliauth.Manager{}).StoredLogins(ctx)
	if err != nil {
		return cloudapi.Org{}, err
	}
	choices := loginChoices(orgs)
	choices = append(choices, cliui.OrgChoice{Name: "Authenticate with another organization"})
	index, _, err := cliui.PickOrg(ctx, t.in, t.out, "Choose where to publish", choices, false, false)
	if err != nil {
		return cloudapi.Org{}, err
	}
	if index < len(orgs) {
		return orgs[index].Org, nil
	}
	address, err := t.loginAddress(ctx)
	if err != nil {
		return cloudapi.Org{}, err
	}
	org, err := t.loginOrg(ctx, address)
	if err != nil {
		return cloudapi.Org{}, err
	}
	_, _, err = cliui.PickOrg(ctx, t.in, t.out, "Use this organization for publishing?", []cliui.OrgChoice{{Name: org.Slug, URL: org.String()}}, false, false)
	return org, err
}
