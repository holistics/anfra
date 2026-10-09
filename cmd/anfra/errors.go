package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// remoteError is a running server's error, as its body carried it.
type remoteError struct{ resp apperr.Response }

func (e *remoteError) Error() string { return e.resp.Code + ": " + e.resp.Message }

// commandError is a command's failure, with what printError needs to name its
// args as the user typed them: the command's path and its args.
type commandError struct {
	path string // "anfra query"
	args []command.Arg
	err  error
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

// printError shows a failed command: what it was doing and why it failed, then
// the error's details, then its cause when it has one. The args that were wrong
// are listed by their flags; other details — an invalid query's diagnostics —
// are YAML. Encapsulation is disabled in this binary (see main), so the message
// is the real one.
func printError(w io.Writer, err error) {
	ce, _ := errors.AsType[*commandError](err)
	if ce != nil {
		err = ce.err
	}
	var resp apperr.Response
	var cause error
	if re, ok := errors.AsType[*remoteError](err); ok {
		resp = re.resp
	} else {
		e := apperr.From(err)
		resp = e.Response("")
		cause = errors.Unwrap(e)
	}
	parts := make([]string, 0, len(resp.Context)+1)
	for _, c := range resp.Context {
		parts = append(parts, c.Text)
	}
	vs := violationsOf(resp.Details)
	switch {
	case ce == nil || len(vs) == 0:
		parts = append(parts, resp.Message)
		fmt.Fprintln(w, "Error:", strings.Join(parts, ": "))
		if resp.Details != nil {
			// Through JSON, so the details read as their wire form does.
			if b, jerr := jsonkit.Marshal(resp.Details); jerr == nil {
				_ = renderTo(b, "application/json", w)
			}
		}
	case len(vs) == 1:
		// The violation says it all: a domain rule's message is its violation's.
		parts = append(parts, argName(ce.args, vs[0].Field), argMessage(ce.args, vs[0]))
		fmt.Fprintln(w, "Error:", strings.Join(parts, ": "))
		fmt.Fprintf(w, "Run `%s --help` for usage.\n", ce.path)
	default:
		parts = append(parts, "these arguments are not valid:")
		fmt.Fprintln(w, "Error:", strings.Join(parts, ": "))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, v := range vs {
			fmt.Fprintf(tw, "  %s\t%s\n", argName(ce.args, v.Field), argMessage(ce.args, v))
		}
		_ = tw.Flush()
		fmt.Fprintf(w, "Run `%s --help` for usage.\n", ce.path)
	}
	if cause != nil && !strings.Contains(resp.Message, cause.Error()) {
		fmt.Fprintln(w, "Cause:", cause)
	}
}

// violationsOf reads details as violations, through JSON: a remote error's are
// decoded as any. None when they are another kind of detail.
func violationsOf(details any) []apperr.Violation {
	if details == nil {
		return nil
	}
	b, err := jsonkit.Marshal(details)
	if err != nil {
		return nil
	}
	var d apperr.Violations
	if jsonkit.Unmarshal(b, &d) != nil {
		return nil
	}
	vs := d.Violations
	for _, v := range vs {
		if v.Field == "" || v.Code == "" {
			return nil
		}
	}
	return vs
}

// argName is how the user passes the arg a violation is on: its flag, or the
// positional's name. A field no arg has (an unknown one) is shown as it is.
func argName(args []command.Arg, field string) string {
	a, ok := argOf(args, field)
	switch {
	case !ok:
		return field
	case a.Positional:
		return a.Name
	}
	return "--" + a.Flag()
}

// argMessage is the violation's message, saying how to pass a missing
// positional, which has no flag to show.
func argMessage(args []command.Arg, v apperr.Violation) string {
	if a, ok := argOf(args, v.Field); ok && a.Positional && (v.Code == "required" || v.Code == "too_short") {
		how := "pass it as an argument"
		if a.Stdin {
			how += ", or pipe it on stdin"
		}
		return "required: " + how
	}
	return clause(v.Message)
}

// clause is a message as the commands' own read after a flag: no first
// capital (but an acronym's, "SQL") and no full stop, as a schema's has.
func clause(s string) string {
	s = strings.TrimSuffix(s, ".")
	if len(s) > 1 && 'A' <= s[0] && s[0] <= 'Z' && 'a' <= s[1] && s[1] <= 'z' {
		return string(s[0]+'a'-'A') + s[1:]
	}
	return s
}

func argOf(args []command.Arg, field string) (command.Arg, bool) {
	for _, a := range args {
		if a.Name == field {
			return a, true
		}
	}
	return command.Arg{}, false
}
