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

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/meta"
)

// The console's own commands (docs/designs/console.md#lines).
const consoleHelp = `The console's commands:
  \?          list these
  \q, \exit   leave (or Ctrl-D)
Any other \ runs an anfra command: \help lists them, \<command> --help says more.`

func newConsoleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "console",
		Short: "Start a session at this terminal, running anfra's commands for a person",
		Long: "Start a session at this terminal, as psql or rails console does: each line starting with \\\n" +
			"runs an anfra command, without the anfra (\\validate, \\query -d sales \"…\"), until \\q or Ctrl-D.\n" +
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
	fmt.Fprintf(out, "anfra console %s in %s\n", meta.Version, homeRelative(dir))
	fmt.Fprintln(out, styles.Muted.Render(`Start a command with \, as in \validate or \query -d sales "orders | count()".`))
	fmt.Fprintln(out, styles.Muted.Render(`\? lists the console's commands, \q leaves.`))
	fmt.Fprintln(out)

	opts := cliui.EntryOptions{Prompt: filepath.Base(dir) + "> ", Leave: `\q or Ctrl-D to leave`}
	for {
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
		if n := len(opts.History); n == 0 || opts.History[n-1] != entry {
			opts.History = append(opts.History, entry)
		}
		args, leave, msg := consoleArgs(entry)
		switch {
		case leave:
			return nil
		case msg != "":
			fmt.Fprintln(out, msg)
		default:
			cmdCtx, stopCmd := signal.NotifyContext(session, os.Interrupt)
			run(cmdCtx, args, out, errOut)
			stopCmd()
		}
		fmt.Fprintln(out)
		if session.Err() != nil {
			return nil
		}
	}
}

// consoleArgs is what an entry asks: an anfra command's args, to leave, or a
// message instead, the console's own (\?) or why it can't run.
func consoleArgs(entry string) (args []string, leave bool, msg string) {
	rest, isCommand := strings.CutPrefix(entry, `\`)
	if !isCommand {
		return nil, false, `Start a command with \, as in \validate: typing AQL directly isn't supported yet.`
	}
	switch strings.TrimSpace(rest) {
	case "q", "exit":
		return nil, true, ""
	case "?", "":
		return nil, false, consoleHelp
	}
	args, err := splitArgs(rest)
	if err != nil {
		return nil, false, err.Error()
	}
	if args[0] == "console" {
		return nil, false, `You're in the console already: \? lists its commands.`
	}
	return args, false, ""
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
