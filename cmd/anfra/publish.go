package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/publish"
)

func newPublishCmd() *cobra.Command {
	var o publish.Options
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish this repo to its org: its files, as they are now, become the published version",
		Long: "Publish this repo's files to its org, as they are in this folder now, committed\n" +
			"in git or not. They replace what is published there, all of it: a file not here is deleted there.\n" +
			"They are validated here first, then on the server, which publishes nothing that fails.\n\n" +
			"Which files: the repo's own, its .aml files and its Data Apps (everything under apps/), wherever they\n" +
			"are in the folder, except under a dot-folder or node_modules. Nothing else is published: not .anfra/\n" +
			"(data_sources.yml holds credentials), nor a .env, notes or build output beside the repo. .gitignore does\n" +
			"not apply: it says what belongs in git, not what is published. --dry-run lists the files and stops.\n\n" +
			"Where to: the org's address on its server, https://<server>/o/<org>, kept in .anfra/config.yml\n" +
			"(org.url) and committed, so everyone publishes to the same org. --set-org sets it, and publishes there.\n" +
			"Without a destination, an interactive terminal offers stored logins or new authentication.\n" +
			"Non-interactive publishing requires a destination and matching anfra auth login credentials.\n" +
			"--yes skips overwrite confirmation only. --dry-run never authenticates or writes configuration.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			terminal := newAuthTerminal(cmd.InOrStdin(), cmd.ErrOrStderr())
			ask := publish.Prompt(func(string, ...string) (int, bool) { return 0, false })
			var selectOrg func(context.Context) (cloudapi.Org, error)
			if terminal.interactive {
				ask = stdinPrompt(terminal.in, terminal.out)
				selectOrg = terminal.selectPublishOrg
			}
			return publish.Run(cmd.Context(), ".", o, publish.Env{
				Out:        cmd.OutOrStdout(),
				ErrOut:     cmd.ErrOrStderr(),
				Ask:        ask,
				Validate:   validateHere,
				Credential: (cliauth.Manager{}).Authorize,
				SelectOrg:  selectOrg,
				UserAgent:  "anfra/" + meta.Version,
			})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.Message, "message", "m", "", "what this publish is, for the org's history of the repo (default: in a git repo, HEAD's commit)")
	f.StringVar(&o.SetOrg, "set-org", "", "set the org the repo publishes to, by its address, https://<server>/o/<org> (.anfra/config.yml org.url), and publish there")
	f.BoolVar(&o.DryRun, "dry-run", false, "list the files that would be published, and stop")
	f.BoolVar(&o.CreateDataSources, "create-data-sources", false, "add to the org the data sources this repo uses (.anfra/data_sources.yml) and the org lacks, without asking (needed without a terminal); their connections are sent to the server. DuckDB ones, files on this machine, are never sent")
	f.BoolVarP(&o.Yes, "yes", "y", false, "publish without asking, though the repo was last published elsewhere (needed without a terminal)")
	return cmd
}

// stdinPrompt asks on out and reads answers from in: a choice's first letter or
// whole name, or nothing for the default. One reader for every question, so an
// answer read ahead is not lost to the next one.
func stdinPrompt(in io.Reader, out io.Writer) publish.Prompt {
	var lines *bufio.Reader
	return func(question string, choices ...string) (int, bool) {
		if f, isFile := in.(*os.File); isFile {
			if stat, err := f.Stat(); err != nil || stat.Mode()&os.ModeCharDevice == 0 {
				return 0, false
			}
		}
		if lines == nil {
			lines = bufio.NewReader(in)
		}
		fallback := len(choices) - 1
		labels := make([]string, len(choices))
		for i, c := range choices {
			labels[i] = "[" + c[:1] + "]" + c[1:]
		}
		fmt.Fprintf(out, "%s %s (default: %s) ", question, strings.Join(labels, " / "), choices[fallback])
		line, _ := lines.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		for i, c := range choices {
			if answer != "" && (answer == c || answer == c[:1]) {
				return i, true
			}
		}
		return fallback, true
	}
}

// validateHere runs validate on this repo before a publish, as `anfra validate`
// does; files that fail are shown, and refuse the publish.
func validateHere(ctx context.Context, errOut io.Writer) error {
	c, ok := app.Find("validate")
	if !ok {
		return errors.New("this anfra has no validate command")
	}
	body, err := invoke(ctx, c, map[string]any{})
	if err != nil {
		return err
	}
	if valid, err := c.Valid(body); err != nil || valid {
		return err
	}
	_ = renderTo(body, "application/json", errOut)
	return errors.New("the repo has errors: fix them and publish again")
}
