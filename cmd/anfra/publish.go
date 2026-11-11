package main

import (
	"bufio"
	"context"
	"encoding/json/jsontext"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/muesli/cancelreader"
	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/cliauth"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/envflag"
	"github.com/holistics/anfra/internal/logging"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/motherduck"
	"github.com/holistics/anfra/internal/publish"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/apperr"
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
			"not apply: it says what belongs in git, not what is published. `anfra publish check` lists them.\n\n" +
			"Where to: the org's address on its server, https://<server>/o/<org>, kept in .anfra/config.yml\n" +
			"(org.url) and committed, so everyone publishes to the same org. --set-org sets it, and publishes there.\n" +
			"Without a destination, an interactive terminal offers stored logins or new authentication.\n" +
			"If the destination needs authentication, log in or choose another org here. Changing\n" +
			"the destination requires confirmation before updating .anfra/config.yml.\n" +
			"Non-interactive publishing requires a destination and matching anfra auth login credentials.\n" +
			"--yes skips overwrite confirmation only.\n\n" +
			"Its result is the revision published, and what came of it. A repo that fails validation fails\n" +
			"with repo_invalid, whose details are validate's answer, as validation.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, former := range []string{"list", "dry-run"} {
				if cmd.Flags().Changed(former) {
					printWarning(cmd.ErrOrStderr(), "deprecated", "--"+former+" is deprecated: use `anfra publish check`.")
				}
			}
			terminal := newAuthTerminal(cmd.InOrStdin(), cmd.ErrOrStderr())
			// Progress and questions are for a person at a terminal; warnings,
			// documents, go to stderr whoever reads it.
			progress := io.Discard
			if terminal.interactive {
				progress = cmd.ErrOrStderr()
			}
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
			logger, closeLog := repoLog()
			defer closeLog()
			res, err := publish.Run(cmd.Context(), ".", o, publish.Env{
				ErrOut:     progress,
				Logger:     logger,
				Warn:       func(code, message string) { printWarning(cmd.ErrOrStderr(), code, message) },
				OrgName:    storedOrgName(cmd.Context()),
				Ask:        ask,
				Input:      input,
				Validate:   func(ctx context.Context) error { return validateHere(ctx, cmd.ErrOrStderr()) },
				Credential: (cliauth.Manager{}).Authorize,
				SelectOrg:  selectOrg,
				RecoverOrg: recoverOrg,
				UserAgent:  "anfra/" + meta.Version,
				MotherDuck: md,
			})
			if err != nil && cmd.Context().Err() != nil { // Ctrl-C, maybe at a question, which took its default
				return apperr.New(clierr.Interrupted, "Interrupted: not published.")
			}
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	markPrettyOutput(cmd)
	resultIs[publish.Result](cmd)
	markInteractable(cmd)
	f := cmd.Flags()
	f.StringVarP(&o.Message, "message", "m", "", "what this publish is, for the org's history of the repo (default: in a git repo, HEAD's commit)")
	f.StringVar(&o.SetOrg, "set-org", "", "set the org the repo publishes to, by its address, https://<server>/o/<org> (.anfra/config.yml org.url), and publish there")
	// --list and, before it, --dry-run were publish check's names: they still
	// work, warning (docs/designs/commands-and-api.md#deprecation-before-removal),
	// as a document, not as pflag's deprecation note: stderr is a stream of them.
	f.BoolVar(&o.Check, "list", false, "former name of `anfra publish check`")
	f.BoolVar(&o.Check, "dry-run", false, "former name of `anfra publish check`")
	_ = f.MarkHidden("list")
	_ = f.MarkHidden("dry-run")
	f.BoolVar(&o.CreateDataSources, "create-data-sources", false, "add to the org the data sources this repo uses (.anfra/data_sources.yml) and the org lacks, without asking (needed without a terminal); their connections are sent to the server. DuckDB ones, files on this machine, are never sent")
	// --yes confirmed replacing a version published elsewhere. Publish now
	// stops instead, so it warns that it does nothing, until a later release
	// removes it.
	f.BoolVarP(&o.Yes, "yes", "y", false, "deprecated, and does nothing now. It confirmed replacing a version someone else published")
	_ = f.MarkHidden("yes")
	cmd.AddCommand(newPublishCheckCmd())
	return cmd
}

// newPublishCheckCmd is publish's preflight
// (docs/designs/commands-and-api.md#asking-without-acting): as far as a
// publish goes before its first change. Today the part done here, offline:
// the files, and where they'd go.
func newPublishCheckCmd() *cobra.Command {
	o := publish.Options{Check: true}
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check what a publish would do, without publishing: the files it would send, and where to",
		Long: "Check what `anfra publish` would do, without publishing: the files of this folder it would\n" +
			"send, and the org they'd go to. Worked out here: it never authenticates, nor writes configuration.\n\n" +
			"Its result is what a publish would send, and where.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := publish.Run(cmd.Context(), ".", o, publish.Env{})
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().StringVar(&o.SetOrg, "set-org", "", "check against this org, by its address, https://<server>/o/<org>, rather than the configured one; nothing is written")
	markPrettyOutput(cmd)
	resultIs[publish.CheckResult](cmd)
	return cmd
}

// stdinQuestions asks on out and reads answers from in: a Prompt's, a choice's
// first letter, name or whole text, or nothing for the default; an Input's, a line of
// text. One reader for every question, so an answer read ahead is not lost to the
// next one. Ctrl-C, which cancels ctx, ends a question unanswered (a Prompt's
// with its default, the safe answer): the read itself cannot be interrupted, so
// it runs apart from the question.
// A command that asks through them is marked interactable (markInteractable),
// for its --help.
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

// readLines reads in's lines as they come, until its end or ctx's. ctx's end
// cancels a read waiting on a terminal, so no read outlives the command: in a
// console, the next entry is read from the same terminal
// (docs/implementations/console.md).
func readLines(ctx context.Context, in io.Reader) chan string {
	lines := make(chan string)
	go func() {
		defer close(lines)
		cr, err := cancelreader.NewReader(in)
		if err != nil {
			return
		}
		defer cr.Close()
		stop := context.AfterFunc(ctx, func() { cr.Cancel() })
		defer stop()
		r := bufio.NewReader(cr)
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
// does: a repo that fails refuses the publish, as repo_invalid with validate's
// answer. What in its Data Apps will not work in a host is a warning on
// errOut, and refuses nothing.
// repoLog opens the repo's log (anfra.log) for a command that records without
// running the repo's sidecars, and closes it. Best-effort: a log that can't be
// opened records nothing, and never stops the command.
func repoLog() (*slog.Logger, func()) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, func() {}
	}
	r := repo.Resolve(dir)
	lg, err := logging.Setup(r.LogsDir(), r.ID, envflag.OnOr(envflag.LogStderr, false))
	if err != nil {
		return nil, func() {}
	}
	return lg.Logger, func() { _ = lg.Close() }
}

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
	return apperr.NewWith(clierr.RepoInvalid, "Not published, because the repo has errors. Fix them, and publish again.", clierr.RepoProblems{Validation: jsontext.Value(body)})
}

// storedOrgName is an org's name as its stored login has it; "" when there's
// none, or the logins can't be read.
func storedOrgName(ctx context.Context) func(cloudapi.Org) string {
	return func(org cloudapi.Org) string {
		logins, err := (cliauth.Manager{}).StoredLogins(ctx)
		if err != nil {
			return ""
		}
		for _, l := range logins {
			if l.Org == org {
				return cliui.SafeText(l.Name)
			}
		}
		return ""
	}
}

// appWarnings warns of validate's Data App warnings, each a document.
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
	for _, r := range res.Reports {
		if r.Validator == "data_app" {
			printWarning(errOut, "data_app_not_portable", r.FilePath+": "+r.Message)
		}
	}
}
