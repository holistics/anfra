package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/motherduck"
	"github.com/holistics/anfra/internal/publish"
	"github.com/holistics/anfra/shared/jsonkit"
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
			"If the destination needs authentication, log in or choose another org here. Changing\n" +
			"the destination requires confirmation before updating .anfra/config.yml.\n" +
			"Non-interactive publishing requires a destination and matching anfra auth login credentials.\n" +
			"--yes skips overwrite confirmation only. --dry-run never authenticates or writes configuration.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			terminal := newAuthTerminal(cmd.InOrStdin(), cmd.ErrOrStderr())
			ask := publish.Prompt(func(string, ...string) (int, bool) { return 0, false })
			input := publish.Input(func(string) (string, bool) { return "", false })
			var selectOrg func(context.Context) (cloudapi.Org, error)
			var recoverOrg func(context.Context, cloudapi.Org, error) (cloudapi.Org, error)
			var md publish.MotherDuck
			if terminal.interactive {
				ask, input = stdinQuestions(cmd.Context(), terminal.in, terminal.out)
				md = &motherduck.Account{
					CLI: motherduck.CLI{Stdin: terminal.in, Stdout: terminal.out, Stderr: terminal.out},
					API: motherduck.API{UserAgent: "anfra/" + meta.Version},
				}
				selectOrg = terminal.selectPublishOrg
				recoverOrg = terminal.recoverPublishOrg
			}
			err := publish.Run(cmd.Context(), ".", o, publish.Env{
				Out:        cmd.OutOrStdout(),
				ErrOut:     cmd.ErrOrStderr(),
				Ask:        ask,
				Input:      input,
				Validate:   validateHere,
				Credential: (cliauth.Manager{}).Authorize,
				SelectOrg:  selectOrg,
				RecoverOrg: recoverOrg,
				UserAgent:  "anfra/" + meta.Version,
				MotherDuck: md,
			})
			if err != nil && cmd.Context().Err() != nil { // Ctrl-C, maybe at a question, which took its default
				return errors.New("interrupted: not published")
			}
			return err
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

// stdinQuestions asks on out and reads answers from in: a Prompt's, a choice's
// first letter, name or whole text, or nothing for the default; an Input's, a line of
// text. One reader for every question, so an answer read ahead is not lost to the
// next one. Ctrl-C, which cancels ctx, ends a question unanswered (a Prompt's
// with its default, the safe answer): the read itself cannot be interrupted, so
// it runs apart from the question.
func stdinQuestions(ctx context.Context, in io.Reader, out io.Writer) (publish.Prompt, publish.Input) {
	var lines chan string
	// line asks, and reads the answer; ok is false with nobody to answer, and
	// answered false when interrupted.
	line := func(ask func(*cliui.Printer)) (answer string, ok, answered bool) {
		if f, isFile := in.(*os.File); isFile {
			if stat, err := f.Stat(); err != nil || stat.Mode()&os.ModeCharDevice == 0 {
				return "", false, false
			}
		}
		if ctx.Err() != nil {
			return "", true, false
		}
		if lines == nil {
			lines = readLines(ctx, in)
		}
		ask(cliui.NewPrinter(out))
		select {
		case answer = <-lines: // closed at the input's end: ""
			return strings.TrimSpace(answer), true, true
		case <-ctx.Done():
			_, _ = io.WriteString(out, "\n")
			return "", true, false
		}
	}
	prompt := func(question string, choices ...string) (int, bool) {
		fallback := len(choices) - 1
		answer, ok, answered := line(func(p *cliui.Printer) { p.Question(question, choices...) })
		if !ok {
			return 0, false
		}
		answer = strings.ToLower(answer)
		for i, c := range choices {
			if answered && answer != "" && (answer == c || answer == c[:1] || answer == cliui.ChoiceName(c)) {
				return i, true
			}
		}
		return fallback, true
	}
	input := func(question string) (string, bool) {
		answer, ok, _ := line(func(p *cliui.Printer) { p.Input(question) })
		return answer, ok
	}
	return prompt, input
}

// readLines reads in's lines as they come, until its end or ctx's.
func readLines(ctx context.Context, in io.Reader) chan string {
	lines := make(chan string)
	go func() {
		defer close(lines)
		r := bufio.NewReader(in)
		for {
			line, err := r.ReadString('\n')
			if line != "" || err == nil {
				select {
				case lines <- line:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return lines
}

// validateHere runs validate on this repo before a publish, as `anfra validate`
// does; files that fail are shown, and refuse the publish. What in its Data Apps
// will not work in a host is shown too, and refuses nothing.
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
		if err == nil {
			appWarnings(body, errOut)
		}
		return err
	}
	_ = renderTo(body, "application/json", errOut)
	return errors.New("the repo has errors: fix them and publish again")
}

// appWarnings shows validate's Data App warnings, one line each.
func appWarnings(body []byte, errOut io.Writer) {
	var res struct {
		Reports []struct {
			Validator string `json:"validator"`
			FilePath  string `json:"filePath"`
			Message   string `json:"message"`
		} `json:"reports"`
	}
	if jsonkit.Unmarshal(body, &res) != nil {
		return
	}
	p := cliui.NewPrinter(errOut)
	for _, r := range res.Reports {
		if r.Validator == "data_app" {
			p.Warn("%s, %s", r.FilePath, r.Message)
		}
	}
}
