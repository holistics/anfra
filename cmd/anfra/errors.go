package main

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/pretty"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// refused is a validation_failed with one violation: field, an arg, flag or
// variable as the user gives it, refused for code, as msg says.
func refused(field, code, msg string) error {
	return apperr.NewWith(apperr.ValidationFailed, msg, apperr.Violate(apperr.Violation{Field: field, Code: code, Message: msg}))
}

// commandFailedWith is a CLI-only command's failure, msg saying what failed,
// with details (clierr.CommandFailed).
func commandFailedWith(msg string, details any) error {
	return apperr.NewWith(clierr.CommandFailed, msg, details)
}

// remoteError is a running server's error: resp, decoded for its code and
// scope, and raw, the error object as the body carried it, which printError
// shows in its order.
type remoteError struct {
	resp apperr.Response
	raw  jsontext.Value
}

// remoteErrorFrom is the error a server's error answer carries; false when body
// is not one.
func remoteErrorFrom(body []byte) (*remoteError, bool) {
	var env struct {
		Error jsontext.Value `json:"error"`
	}
	var resp apperr.Response
	if jsonkit.Unmarshal(body, &env) != nil || len(env.Error) == 0 || jsonkit.Unmarshal(env.Error, &resp) != nil || resp.Code == "" {
		return nil, false
	}
	return &remoteError{resp: resp, raw: env.Error}, true
}

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

// printError shows a failed command as its error's envelope, the one HTTP and
// MCP answer, as a YAML document, or a JSON line with --json: stderr is a stream
// of them (printWarning's, then this one), so a program reads it with a YAML or
// JSONL reader. Two fields are the
// CLI's own, for whoever fixes the call: each violation's arg, the flag or
// positional the user typed, and help, the command's --help. A shown cause is
// in message, as it is over HTTP (see main for when causes are shown).
func printError(w io.Writer, err error, hint string) {
	ce, _ := errors.AsType[*commandError](err)
	if ce != nil {
		err = ce.err
	}
	var resp apperr.Response
	var body []byte
	if re, ok := errors.AsType[*remoteError](err); ok {
		resp, body = re.resp, re.raw
	} else if ce, ok := errors.AsType[*cloudapi.Error](err); ok && !coded(err) {
		resp = ce.Response // the org's server's, as it answered
	} else {
		resp = apperr.From(err).Response("")
	}
	var jerr error
	if body == nil {
		body, jerr = jsonkit.Marshal(resp)
	}
	var doc yaml.Node
	if jerr == nil {
		jerr = yaml.Unmarshal(body, &doc) // JSON is YAML: the wire's field order is kept
	}
	if jerr != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		writeDoc(w, "error", map[string]string{"code": resp.Code, "message": resp.Message})
		return
	}
	e := doc.Content[0]
	blockStyle(e)
	// help is what helps whoever fixes the call, one piece of advice a line: the
	// command's usage, when a violation names an arg, and hint.
	var help []string
	if ce != nil && nameArgs(mapValue(e, "details"), ce.args) {
		help = append(help, usageHelp(ce.path, mapValue(e, "details")))
	}
	if h := codeHelp[resp.Code]; h != "" {
		help = append(help, h)
	}
	if hint != "" {
		help = append(help, hint)
	}
	if len(help) > 0 {
		setKey(e, "help", strings.Join(help, "\n"))
	}
	if forPerson() {
		writePrettyError(w, e)
		return
	}
	writeDoc(w, "error", e)
}

// prettyError is an error's envelope as a person reads it: its message is
// the headline, written apart; its scope and status, which its code implies,
// and an empty request id, left to --yaml.
type prettyError struct {
	Code      string `json:"code"`
	Details   any    `json:"details,omitempty" pretty:",inline"`
	Context   any    `json:"context,omitempty"`
	RequestID string `json:"request_id,omitempty" pretty:"Request ID"`
	Help      string `json:"help,omitempty"`
}

// writePrettyError writes error e, its envelope, in the pretty format
// (docs/designs/cli-pretty.md): a red ✗ and its message, a cause a line
// (messageParts), each under the one it caused, its colon kept, faint but
// the last, the root cause, plain; a blank line after a chain; then its code,
// details and help by the conventions.
func writePrettyError(w io.Writer, e *yaml.Node) {
	env, ok := nodeValue(e).(pretty.Object)
	if !ok {
		writeDoc(w, "error", e)
		return
	}
	str := func(k string) string { s, _ := env.Values[k].(string); return s }
	pe := prettyError{Code: str("code"), Details: env.Values["details"], Context: env.Values["context"], RequestID: str("request_id"), Help: str("help")}
	st := cliui.NewTerminalStyles(w)
	parts := messageParts(cliui.SafeText(str("message")))
	for i, part := range parts {
		if i < len(parts)-1 {
			// What led to it, faint, its colon kept: the line under it says why.
			// The last, the root cause, is plain, what's read.
			part = st.Muted.Render(part + ":")
		}
		if i == 0 {
			fmt.Fprintln(w, st.Error.Render("✗")+" "+part)
		} else {
			fmt.Fprintln(w, strings.Repeat("  ", i+1)+part) // the cause of the line above
		}
	}
	if len(parts) > 1 {
		fmt.Fprintln(w) // a chain set apart from what follows
	}
	var body bytes.Buffer
	width := cliui.Width(w)
	if width > 2 {
		width -= 2 // the indent under the headline; 0 stays 0, not a terminal
	}
	if err := pretty.RenderStyled(&body, pe, width, st); err != nil { // styled as w is, a terminal; body isn't
		return
	}
	for line := range strings.Lines(body.String()) {
		if strings.TrimSpace(line) != "" {
			line = "  " + line // under its headline
		}
		fmt.Fprint(w, line)
	}
}

// messageParts is an error's message as a chain of causes, a part a line: a
// shown cause is folded into its message, "resolve anfra-node binary: unpack
// anfra-node into anfra's folder (ANFRA_HOME)", each part after a ": " the
// cause of the one before. A part of fewer than 4 words stays with the one
// before it ("apps/a.html: line 3"): a detail, not a cause. Only ": " splits,
// so a URL or a time doesn't.
func messageParts(msg string) []string {
	var parts []string
	for i, part := range strings.Split(msg, ": ") {
		if i > 0 && len(strings.Fields(part)) < 4 {
			parts[len(parts)-1] += ": " + part
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

// nodeValue is YAML node n as the prettifier reads it, its mappings' keys in
// order (pretty.Object), its sequences lists, its scalars as they decode.
func nodeValue(n *yaml.Node) any {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil
		}
		return nodeValue(n.Content[0])
	case yaml.MappingNode:
		o := pretty.Object{Values: map[string]any{}}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i].Value
			o.Keys = append(o.Keys, k)
			o.Values[k] = nodeValue(n.Content[i+1])
		}
		return o
	case yaml.SequenceNode:
		list := make([]any, len(n.Content))
		for i, c := range n.Content {
			list[i] = nodeValue(c)
		}
		return list
	case yaml.AliasNode:
		return nodeValue(n.Alias)
	}
	var v any
	if n.Decode(&v) != nil {
		return n.Value
	}
	return v
}

// codeHelp is help that the CLI adds to an error, by the error's code. It's
// for advice that only the CLI can give. For example, a sidecar also runs
// inside the engine that platforms embed, where there's no `anfra status` or
// repo log, so the sidecar's own error can't point to them.
var codeHelp = map[string]string{
	errcode.SidecarUnavailable.String(): "The sidecar's logs are in the repo's log file, which `anfra status` shows.",
}

// failureHint records a failure the caller could fix, off a terminal and
// outside CI, and is what its error should hint: an agent that keeps failing at
// anfra's commands without the skills is nudged to set them up
// (agents.Failed). "" otherwise.
func failureHint(err error) string {
	if stderrIsInteractive() || os.Getenv("CI") != "" || updateNotifyDisabled() {
		return ""
	}
	if ce, ok := errors.AsType[*commandError](err); ok {
		err = ce.err
	}
	scope := apperr.From(err).Code.Scope()
	if re, ok := errors.AsType[*remoteError](err); ok {
		scope = re.resp.Scope
	} else if ce, ok := errors.AsType[*cloudapi.Error](err); ok && !coded(err) {
		scope = ce.Response.Scope
	}
	if scope != apperr.User && scope != apperr.Client {
		return ""
	}
	return agents.Failed(agents.System(), repoDirIfAny(), time.Now())
}

// coded reports whether err carries a code of its own, at its top: a code
// that wraps, say, the org's server's answer is what it says.
func coded(err error) bool {
	_, ok := err.(*apperr.Error)
	return ok
}

// printWarning shows a problem that did not stop the command, such as an
// operator's setting that could not take effect, as a YAML document like
// printError's.
func printWarning(w io.Writer, code, message string) {
	if forPerson() { // as a person reads it: a yellow !, its code faint
		st := cliui.NewTerminalStyles(w)
		fmt.Fprintln(w, st.Warning.Render("! "+cliui.SafeText(message))+st.Muted.Render(" ("+code+")"))
		return
	}
	e := &yaml.Node{Kind: yaml.MappingNode}
	setKey(e, "code", code)
	setKey(e, "message", message)
	writeDoc(w, "warning", e)
}

// writeDoc writes {key: v} as one document of a YAML stream, or with --json as
// one line of a JSONL stream.
func writeDoc(w io.Writer, key string, v any) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	value := &yaml.Node{}
	if n, ok := v.(*yaml.Node); ok {
		value = n
	} else if err := value.Encode(v); err != nil {
		return
	}
	if output.json {
		writeJSONDoc(w, key, value)
		return
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	b, err := marshalYAML(root)
	if err != nil {
		return
	}
	_, _ = w.Write(paintYAML(w, append([]byte("---\n"), b...), docTones))
}

// blockStyle drops the JSON's style from n and beneath, its quotes and braces,
// so the document reads as YAML written by hand. A string is styled as yaml.v3
// styles a Go string: quoted where a reader would take it for something else.
// A number kept as a string, such as a query row's integer past 2^53, stays a
// string, so a reader doesn't parse it as a number and round it; "yes" stays a
// string for a YAML 1.1 reader, which would take it for true.
func blockStyle(n *yaml.Node) {
	n.Style = 0
	if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
		var s yaml.Node
		if s.Encode(n.Value) == nil {
			n.Style = s.Style
		}
	}
	for _, c := range n.Content {
		blockStyle(c)
	}
}

// nameArgs adds to each of details' violations the arg it is on, as the user
// passes it, and reports whether there were any.
func nameArgs(details *yaml.Node, args []command.Arg) bool {
	if details == nil || details.Kind != yaml.MappingNode {
		return false
	}
	vs := mapValue(details, "violations")
	if vs == nil || vs.Kind != yaml.SequenceNode || len(vs.Content) == 0 {
		return false
	}
	for _, v := range vs.Content {
		field := mapValue(v, "field")
		if v.Kind != yaml.MappingNode || field == nil {
			return false
		}
		setKey(v, "arg", argName(args, field.Value))
	}
	return true
}

// usageHelp is the help for a call whose violations nameArgs named: its
// command's usage, as a shell runs it. In a console, as typed there, without
// the anfra, and first how to set each flag a violation names for every
// command (docs/designs/console.md#lines): a flag missing is most often one
// the session would rather hold.
func usageHelp(path string, details *yaml.Node) string {
	if !output.inConsole {
		return fmt.Sprintf("Run `%s --help` for usage.", path)
	}
	var sets []string
	if vs := mapValue(details, "violations"); vs != nil {
		for _, v := range vs.Content {
			if arg := mapValue(v, "arg"); arg != nil && strings.HasPrefix(arg.Value, "--") && !slices.Contains(sets, arg.Value) {
				sets = append(sets, arg.Value)
			}
		}
	}
	usage := fmt.Sprintf("Run `%s --help` for usage.", strings.TrimPrefix(path, "anfra "))
	if len(sets) == 0 {
		return usage
	}
	for i, f := range sets {
		sets[i] = "`\\" + f + " <value>`"
	}
	return fmt.Sprintf("%s sets it for every command here. %s", strings.Join(sets, " or "), usage)
}

// mapValue is the value of key in mapping m, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setKey sets key in mapping m to the string value: in place, or after the
// others.
func setKey(m *yaml.Node, key, value string) {
	if v := mapValue(m, key); v != nil {
		v.SetString(value)
		return
	}
	v := &yaml.Node{}
	v.SetString(value)
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
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

func argOf(args []command.Arg, field string) (command.Arg, bool) {
	for _, a := range args {
		if a.Name == field {
			return a, true
		}
	}
	return command.Arg{}, false
}
