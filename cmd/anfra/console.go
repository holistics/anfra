package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/update"
)

// consoleHelp is \?'s answer: the console's own commands, then anfra's, as
// anfra --help groups them, one line each, the names in violet, typed as
// they are (docs/designs/console.md#lines). Left out: the console itself,
// refused in one; help, which this replaces; completion, a shell's.
func consoleHelp(styles cliui.TerminalStyles) string {
	var b strings.Builder
	section := func(title string, rows [][2]string) {
		width := 0
		for _, r := range rows {
			width = max(width, len(r[0]))
		}
		b.WriteString(title + "\n")
		for _, r := range rows {
			b.WriteString("  " + styles.Accent.Render(r[0]) + strings.Repeat(" ", width-len(r[0])+3) + r[1] + "\n")
		}
	}
	section("The console's commands:", [][2]string{
		{`\?`, "list these"},
		{`\q, exit, quit`, "leave (or Ctrl-D)"},
	})
	root := newRootCmd()
	for _, g := range root.Groups() {
		var rows [][2]string
		for _, c := range root.Commands() {
			if c.GroupID != g.ID || c.Hidden || !c.IsAvailableCommand() || c.Name() == "console" || c.Name() == "help" || c.Name() == "completion" {
				continue
			}
			rows = append(rows, [2]string{c.Name(), c.Short})
		}
		if len(rows) > 0 {
			b.WriteString("\n")
			section(g.Title, rows)
		}
	}
	b.WriteString("\n" + styles.Muted.Render("<command> --help says more about one."))
	return b.String()
}

// consoleAct is what an entry asks of the console.
type consoleAct int

const (
	actRun   consoleAct = iota // run an anfra command
	actLeave                   // end the session
	actHelp                    // list the commands
	actSay                     // say why it can't run
)

func newConsoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "console",
		Aliases: []string{"c"},
		Short:   "Start a session at this terminal, running anfra's commands for a person",
		Long: "Start a session at this terminal, as psql or rails console does: each line runs a command,\n" +
			"typed as after anfra (validate, query -d sales \"…\"), until \\q, exit or Ctrl-D.\n" +
			"Ctrl-C clears the line being typed, or cancels the command running. It needs a terminal.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if output.nonInteractive || !cliui.CanInteract(os.Stdin, os.Stdout) {
				return clierr.Precondition("anfra console needs a terminal: run it at one, or run each command as anfra <command>.")
			}
			return runConsole(cmd.Context(), os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	markArtifactOutput(cmd, "a session, until you leave")
	return cmd
}

// runConsole is a console session: entries read from in until \q, \exit or
// Ctrl-D, each command run as `anfra <command>` runs, on a tree of its own.
// Ctrl-C cancels the command running, not the session, which only SIGTERM
// ends: so the session's context is ctx's values, not its end, which main's
// interrupt brings.
func runConsole(ctx context.Context, in io.Reader, out, errOut io.Writer) error {
	session, stop := signal.NotifyContext(context.WithoutCancel(ctx), syscall.SIGTERM)
	defer stop()
	styles := cliui.NewTerminalStyles(out)
	dir, _ := os.Getwd()
	if os.Getenv(consoleRestarted) != "" { // the session goes on, on the new anfra
		_ = os.Unsetenv(consoleRestarted)
		fmt.Fprintln(out, styles.Muted.Render("Restarted on anfra "+meta.Version+"."))
	} else {
		fmt.Fprintf(out, "anfra console %s in %s\n", meta.Version, homeRelative(dir))
		fmt.Fprintln(out, styles.Muted.Render(`Type a command as after anfra: validate, query -d sales "orders | count()".`))
		fmt.Fprintln(out, styles.Muted.Render(`\? lists the console's commands, \q or exit leaves.`))
	}
	fmt.Fprintln(out)

	exe := runningAnfra()
	history := openConsoleHistory()
	opts := cliui.EntryOptions{Prompt: filepath.Base(dir) + "> ", Leave: `\q, exit or Ctrl-D to leave`, History: history.load()}
	for {
		opts.Status = consoleBadge(styles) //nolint:contextcheck // reads local state, runs nothing
		entry, err := cliui.ReadEntry(session, in, out, opts)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		remember := func(entry string) {
			if n := len(opts.History); n == 0 || opts.History[n-1] != entry {
				opts.History = append(opts.History, entry)
				history.add(entry)
			}
		}
		args, act, msg := consoleArgs(entry)
		if act != actLeave { // Up then Enter shouldn't leave by surprise
			remember(entry)
		}
		switch act {
		case actLeave:
			return nil
		case actHelp:
			fmt.Fprintln(out, consoleHelp(styles)) //nolint:contextcheck // names the commands, runs none
		case actSay:
			fmt.Fprintln(out, msg)
		default:
			cmdCtx, stopCmd := signal.NotifyContext(session, os.Interrupt)
			code := run(cmdCtx, args, out, errOut, runOptions{inConsole: true})
			stopCmd()
			if code == 0 && args[0] == "update" && exe.replaced() {
				fmt.Fprintln(out)
				return restartConsole(exe.path)
			}
			if code != 0 && !isCommand(args[0]) { //nolint:contextcheck // names the commands, runs none
				fmt.Fprintln(out, styles.Muted.Render(`Not an anfra command. To run AQL: query -d <dataset> "…" (typing it directly comes later).`))
			}
		}
		fmt.Fprintln(out)
		if session.Err() != nil {
			return nil
		}
	}
}

// consoleBadge is the notice under the console's prompt, one at a time, as
// short as a badge, the command dealing with it a code span
// (docs/designs/console.md#the-rest): a newer anfra first, then what of the
// agents' skills needs attention. Both from local state, as the CLI's notices
// after a command are, so it's checked again after each; "" when nothing
// needs attention, or with ANFRA_NO_UPDATE_NOTIFIER.
func consoleBadge(styles cliui.TerminalStyles) string {
	if updateNotifyDisabled() {
		return ""
	}
	badge := func(mark, label, cmd string) string {
		return styles.Muted.Render(mark+" "+label+" · Run") + " " + styles.Code.Render(cmd)
	}
	if r := update.Cached(); r != nil && r.IsNewer() {
		return badge("↑", "anfra "+r.Version, "update")
	}
	if mark, label := agents.Badge(agents.System(), repoDirIfAny()); mark != "" {
		return badge(mark, label, "agent setup")
	}
	return ""
}

// consoleRestarted tells a console it was restarted on a new anfra, after
// update replaced the one it ran on: it says so, rather than its banner.
const consoleRestarted = "ANFRA_CONSOLE_RESTARTED"

// anfraFile is the file the console's anfra was started from: its path, and
// the file at it then.
type anfraFile struct {
	path string
	info os.FileInfo // nil when unknown
}

// runningAnfra is the running anfra's file, taken when the console starts:
// an update renames the running file aside before putting the new one at its
// path (minio/selfupdate), and os.Executable follows the file, not the path.
func runningAnfra() anfraFile {
	path, err := os.Executable()
	if err != nil {
		return anfraFile{}
	}
	info, _ := os.Stat(path)
	return anfraFile{path: path, info: info}
}

// replaced reports whether another file is at f's path now: an update put a
// new anfra in its place.
func (f anfraFile) replaced() bool {
	if f.info == nil {
		return false
	}
	now, err := os.Stat(f.path)
	return err == nil && !os.SameFile(f.info, now)
}

// consoleArgs is what an entry asks: an anfra command's args, to leave, the
// commands listed (\?), or a message saying why it can't run. In command
// mode, the console's only one yet, a line is a command; a \ before it, as
// a text mode will need, is taken too (docs/designs/console.md#lines).
func consoleArgs(entry string) (args []string, act consoleAct, msg string) {
	rest := strings.TrimPrefix(entry, `\`)
	switch strings.TrimSpace(rest) {
	case "q", "exit", "quit":
		return nil, actLeave, ""
	case "?", "":
		return nil, actHelp, ""
	}
	args, err := splitArgs(rest)
	if err != nil {
		return nil, actSay, err.Error()
	}
	if args[0] == "console" || args[0] == "c" {
		return nil, actSay, `You're in the console already: \? lists its commands.`
	}
	return args, actRun, ""
}

// isCommand reports whether name is one of anfra's commands, or an alias of
// one: a line starting otherwise, AQL say, is said how to type it.
func isCommand(name string) bool {
	for _, c := range newRootCmd().Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return true
		}
	}
	return false
}

// splitArgs splits s into args as a POSIX shell does, without expanding
// anything: words apart by spaces; in '…' every character as it is; in "…"
// and outside quotes, a backslash keeping the next character (in "…", only
// before $ ` " \ or a newline).
func splitArgs(s string) ([]string, error) {
	var args []string
	var word strings.Builder
	inWord := false
	var quote rune
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				word.WriteRune(c)
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(runes) && strings.ContainsRune("$`\"\\\n", runes[i+1]):
				i++
				word.WriteRune(runes[i])
			default:
				word.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == '\\':
			if i+1 < len(runes) {
				i++
				word.WriteRune(runes[i])
			}
			inWord = true
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				args = append(args, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("A quote (%c) isn't closed.", quote) //nolint:staticcheck // shown as a sentence
	}
	if inWord {
		args = append(args, word.String())
	}
	return args, nil
}

// homeRelative is dir with the home folder as ~, as a shell's prompt shows it.
func homeRelative(dir string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	if rel, err := filepath.Rel(home, dir); err == nil && !strings.HasPrefix(rel, "..") {
		if rel == "." {
			return "~"
		}
		return filepath.Join("~", rel)
	}
	return dir
}
