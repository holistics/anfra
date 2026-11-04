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
	"github.com/spf13/pflag"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/update"
)

// consoleHelp is \?'s answer: where the session is, in full, as the line
// under the prompt may cut it short, and any notice in full; then anfra's
// own help for where it is, anfra --help in command mode, the mode's
// command's --help in a text mode; then what the console adds.
func consoleHelp(s *consoleSession, dir string) string {
	st := s.styles
	key := st.Accent.Render
	var b strings.Builder
	label := func(l string) string { return st.Muted.Render(l) }
	mode := "commands"
	if s.mode != nil {
		mode = s.modePath + " " + s.textName()
	}
	flags := s.styledFlags()
	if flags == "" {
		flags = label("none")
	}
	b.WriteString(label("Repo:  ") + homeRelative(dir) + "\n")
	modeStyle := st.Accent
	if st.Muted.Render("x") != "x" { // bold as the prompt is, with styles only
		modeStyle = modeStyle.Bold(true)
	}
	b.WriteString(label("Mode:  ") + modeStyle.Render(mode) + "\n")
	b.WriteString(label("Flags: ") + flags + label(" (view all with ") + st.Code.Render(`\--`) + label(")") + "\n")
	var notices []string
	if w := consoleWarning(s); w != "" {
		notices = append(notices, w)
	}
	for _, n := range consoleNotices() {
		notices = append(notices, codeSpans(n, st))
	}
	if len(notices) > 0 {
		b.WriteString(label("Notices:") + "\n  " + strings.Join(notices, "\n  ") + "\n")
	}
	b.WriteString("\nUsage:\n")
	pair := func(k1, v1, k2, v2 string) string {
		return "  " + key(k1) + strings.Repeat(" ", max(14-len(k1), 1)) + v1 + strings.Repeat(" ", max(24-len(v1), 2)) + key(k2) + "  " + v2 + "\n"
	}
	root := newRootCmd()
	if c := s.modeCommand(); c != nil {
		b.WriteString("  " + s.howToType() + " Alt+Enter runs it as it is.\n")
		b.WriteString(pair(`\<command>`, "runs a command", `\c`, "goes back to commands"))
		b.WriteString(pair(`\--<flag>`, "sets a flag", `\q`, "leaves"))
		local := pflag.NewFlagSet("", pflag.ContinueOnError)
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name != "help" && !f.Hidden {
				local.AddFlag(f)
			}
		})
		b.WriteString("\n" + s.modePath + " flags:\n" + local.FlagUsages())
	} else {
		b.WriteString("  " + key("c <command>") + "   types its text directly: " + strings.Join(modeCommands(root), ", ") + "\n")
		b.WriteString(pair(`\--<flag>`, "sets a flag", `\q`, "leaves"))
		for _, g := range root.Groups() {
			var rows [][2]string
			width := 0
			for _, c := range root.Commands() {
				if c.GroupID == g.ID && c.IsAvailableCommand() && c.Name() != "console" && c.Name() != "help" && c.Name() != "completion" {
					rows = append(rows, [2]string{c.Name(), c.Short})
					width = max(width, len(c.Name()))
				}
			}
			b.WriteString("\n" + g.Title + "\n")
			for _, r := range rows {
				b.WriteString("  " + key(r[0]) + strings.Repeat(" ", width-len(r[0])+3) + r[1] + "\n")
			}
		}
	}
	global := pflag.NewFlagSet("", pflag.ContinueOnError)
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && !notSessionFlags[f.Name] {
			global.AddFlag(f)
		}
	})
	b.WriteString("\nGlobal flags:\n" + strings.TrimRight(global.FlagUsages(), "\n"))
	return b.String()
}

// consoleWarning is what the next entry would fail for, as its command's own
// check finds it (flagProblem), in yellow, where the eye lands, the left of
// the line under the frame: a flag missing, and how to set it; a flag that
// can't be used, or is invalid, in the check's words, and how to unset it.
// "" when none.
func consoleWarning(s *consoleSession) string {
	long, short, code, msg := s.flagProblem()
	switch {
	case long == "":
		return ""
	case code == "required":
		return s.styles.Warning.Render("! "+long+" is missing. Set it using") + " " + s.styles.Code.Render(`\`+short+" <value>")
	}
	// The check's message says what's wrong, standing alone, as every
	// surface shows it: first letter up, and the fix after it.
	msg = strings.TrimSuffix(msg, ".")
	if msg != "" {
		msg = strings.ToUpper(msg[:1]) + msg[1:]
	}
	return s.styles.Warning.Render("! "+msg+". Unset it using") + " " + s.styles.Code.Render(`\`+long+"=")
}

// textName is what the mode's text is: "AQL", "SQL", or "text".
func (s *consoleSession) textName() string {
	switch s.lang {
	case "aql", "sql":
		return strings.ToUpper(s.lang)
	}
	return "text"
}

// consoleNotices are the notices the badge stands for, in full, as the CLI
// says them after a command elsewhere, a command as typed in the console:
// `update`, not `anfra update`.
func consoleNotices() []string {
	if updateNotifyDisabled() {
		return nil
	}
	var notices []string
	if n := update.CachedNotice(); n != "" {
		notices = append(notices, n)
	}
	if n := agents.Attention(agents.System(), repoDirIfAny()); n != "" {
		notices = append(notices, n)
	}
	for i, n := range notices {
		notices[i] = strings.ReplaceAll(n, "`anfra ", "`")
	}
	return notices
}

// codeSpans is text with its `code` as code spans, without the backquotes,
// with styles; as written, without.
func codeSpans(text string, st cliui.TerminalStyles) string {
	parts := strings.Split(text, "`")
	if st.Code.Render("x") == "x" || len(parts)%2 == 0 {
		return text
	}
	for i := 1; i < len(parts); i += 2 {
		parts[i] = st.Code.Render(parts[i])
	}
	return strings.Join(parts, "")
}

func newConsoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "console [command [flags]]",
		Aliases: []string{"c"},
		Short:   "Start a session at this terminal, running anfra's commands for a person",
		Long: "Start a session at this terminal, as psql or rails console does. Each line runs a command,\n" +
			"typed as after anfra (validate, query -d sales \"…\"), until \\q, exit or Ctrl-D.\n" +
			"Ctrl-C clears the line being typed, or cancels the command running. It needs a terminal.\n\n" +
			"With a command, its text is typed directly. anfra console query takes AQL, each query ending\n" +
			"with ;;, and \\<command> runs a command. In the console, console <command> switches to one,\n" +
			"and console alone goes back to commands. \\--<flag> <value> sets a flag for every command.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if output.nonInteractive || !cliui.CanInteract(os.Stdin, os.Stdout) {
				return clierr.Precondition("anfra console needs a terminal. Run it at one, or run each command as anfra <command>.")
			}
			var s consoleSession
			if len(args) > 0 {
				if msg := s.switchMode(args); s.mode == nil {
					return clierr.Precondition("%s", msg)
				}
			}
			return runConsole(cmd.Context(), os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr(), &s)
		},
	}
	// A mode's flags are the mode's: anfra console query --lang sql.
	cmd.Flags().SetInterspersed(false)
	markArtifactOutput(cmd, "a session, until you leave")
	return cmd
}

// runConsole is a console session: entries read from in until \q, \exit or
// Ctrl-D, each command run as `anfra <command>` runs, on a tree of its own.
// Ctrl-C cancels the command running, not the session, which only SIGTERM
// ends: so the session's context is ctx's values, not its end, which main's
// interrupt brings.
func runConsole(ctx context.Context, in io.Reader, out, errOut io.Writer, s *consoleSession) error {
	session, stop := signal.NotifyContext(context.WithoutCancel(ctx), syscall.SIGTERM)
	defer stop()
	styles := cliui.NewTerminalStyles(out)
	s.styles = styles
	dir, _ := os.Getwd()
	if os.Getenv(consoleRestarted) != "" { // the session goes on, on the new anfra
		_ = os.Unsetenv(consoleRestarted)
		fmt.Fprintln(out, styles.Muted.Render("Restarted on anfra "+meta.Version+"."))
	} else {
		fmt.Fprintf(out, "anfra console %s in %s\n", meta.Version, homeRelative(dir))
		if s.mode != nil {
			fmt.Fprintln(out, styles.Muted.Render(s.modeHint()))
		} else {
			fmt.Fprintln(out, styles.Muted.Render(`Type a command as after anfra; c query to type AQL. \? helps, \q leaves.`))
		}
	}
	fmt.Fprintln(out)

	exe := runningAnfra()
	history := openConsoleHistory()
	opts := cliui.EntryOptions{Leave: `\q, exit or Ctrl-D to leave`, History: history.load(), Ends: s.ends}
	for {
		opts.Prompt, opts.Context = s.prompt(), s.styledContext(filepath.Base(dir), false) //nolint:contextcheck // names the flags, runs nothing
		opts.Status = consoleBadge(s)                                                      //nolint:contextcheck // reads local state, runs nothing
		opts.Warning = consoleWarning(s)                                                   //nolint:contextcheck // reads the commands, runs none
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
		args, act, msg := s.dispatch(entry) //nolint:contextcheck // reads the commands, runs none
		if act != actLeave {                // Up then Enter shouldn't leave by surprise
			remember(entry)
		}
		switch act {
		case actLeave:
			return nil
		case actHelp:
			fmt.Fprintln(out, consoleHelp(s, dir)) //nolint:contextcheck // names the commands, runs none
		case actSay:
			if msg != "" {
				fmt.Fprintln(out, msg)
			}
		default:
			args = s.withFlags(args) //nolint:contextcheck // reads the commands, runs none
			cmdCtx, stopCmd := signal.NotifyContext(session, os.Interrupt)
			code := run(cmdCtx, args, out, errOut, runOptions{inConsole: true})
			stopCmd()
			if code == 0 && args[0] == "update" && exe.replaced() {
				fmt.Fprintln(out)
				return restartConsole(exe.path)
			}
			if code != 0 && s.mode == nil && !isCommand(args[0]) { //nolint:contextcheck // names the commands, runs none
				fmt.Fprintln(out, styles.Muted.Render(`Not an anfra command. To type AQL, enter console query.`))
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
// agents' skills needs attention. What the next entry would fail for is a
// warning, apart (consoleWarning). Both from local state, as the CLI's notices
// after a command are, so it's checked again after each; "" when nothing
// needs attention, or with ANFRA_NO_UPDATE_NOTIFIER.
func consoleBadge(s *consoleSession) string {
	styles := s.styles
	badge := func(mark, label, cmd string) string {
		return styles.Muted.Render(mark+" "+label+" · Run") + " " + styles.Code.Render(cmd)
	}
	if updateNotifyDisabled() {
		return ""
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
