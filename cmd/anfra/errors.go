package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/agents"
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

// printError shows a failed command as its error's envelope, the one HTTP and
// MCP answer, as a YAML document: stderr is a stream of them (printWarning's,
// then this one), so a program reads it with a YAML library. Two fields are the
// CLI's own, for whoever fixes the call: each violation's arg, the flag or
// positional the user typed, and help, the command's --help. A shown cause is
// in message, as it is over HTTP (see main for when causes are shown).
func printError(w io.Writer, err error, hint string) {
	ce, _ := errors.AsType[*commandError](err)
	if ce != nil {
		err = ce.err
	}
	var resp apperr.Response
	if re, ok := errors.AsType[*remoteError](err); ok {
		resp = re.resp
	} else {
		resp = apperr.From(err).Response("")
	}
	body, jerr := jsonkit.Marshal(resp)
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
	if v := mapValue(e, "request_id"); v != nil && v.Value == "" {
		deleteKey(e, "request_id") // a command run here has none
	}
	// help is what helps whoever fixes the call, one piece of advice a line: the
	// command's usage, when a violation names an arg, and hint.
	var help []string
	if ce != nil && nameArgs(mapValue(e, "details"), ce.args) {
		help = append(help, fmt.Sprintf("Run `%s --help` for usage.", ce.path))
	}
	if hint != "" {
		help = append(help, hint)
	}
	if len(help) > 0 {
		setKey(e, "help", strings.Join(help, "\n"))
	}
	writeDoc(w, "error", e)
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
	}
	if scope != apperr.User && scope != apperr.Client {
		return ""
	}
	return agents.Failed(agents.System(), repoDirIfAny(), time.Now())
}

// printWarning shows a problem that did not stop the command, such as an
// operator's setting that could not take effect, as a YAML document like
// printError's.
func printWarning(w io.Writer, code, message string) {
	e := &yaml.Node{Kind: yaml.MappingNode}
	setKey(e, "code", code)
	setKey(e, "message", message)
	writeDoc(w, "warning", e)
}

// writeDoc writes {key: v} as one document of a YAML stream.
func writeDoc(w io.Writer, key string, v any) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	value := &yaml.Node{}
	if n, ok := v.(*yaml.Node); ok {
		value = n
	} else if err := value.Encode(v); err != nil {
		return
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
	b, err := yaml.Marshal(root)
	if err != nil {
		return
	}
	_, _ = io.WriteString(w, "---\n")
	_, _ = w.Write(b)
}

// blockStyle drops the JSON's style from n and beneath, its quotes and braces,
// so the document reads as an answer's YAML does. A string that needs quotes
// still gets them.
func blockStyle(n *yaml.Node) {
	n.Style = 0
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

func deleteKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
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
