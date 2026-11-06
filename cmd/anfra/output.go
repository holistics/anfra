package main

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/pretty"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// output is how this run writes its output, and whether it may ask: the
// root's --yaml, --json and --non-interactive
// (docs/designs/commands-and-api.md#cli-output). The format flags and
// --non-interactive are independent: --json sets the format only. inConsole
// is run's option, not a flag: a console's run is a person's, so it colors
// YAML and JSON at a terminal, and indents JSON (docs/designs/console.md).
var output struct {
	pretty         bool
	yaml           bool
	json           bool
	schema         bool
	jq             string
	nonInteractive bool
	inConsole      bool
}

// docTones are the tones of an error's and a warning's document: its code.
var docTones = pretty.Tones{"error.code": "bad", "warning.code": "warn"}

// paintYAML is b, YAML, colored for out when the run is a console's and out is
// a terminal (pretty.ColorYAML); else b.
func paintYAML(out io.Writer, b []byte, tones pretty.Tones) []byte {
	if !output.inConsole {
		return b
	}
	return pretty.ColorYAML(b, cliui.NewTerminalStyles(out), tones)
}

// paintJSON is b, JSON lines, for a console's run: each document indented by
// 2, as gh and jq show JSON to a person, then colored at a terminal; else b,
// a line a document, for a program.
func paintJSON(out io.Writer, b []byte, tones pretty.Tones) []byte {
	if !output.inConsole {
		return b
	}
	var indented bytes.Buffer
	for line := range bytes.Lines(b) {
		v := jsontext.Value(bytes.TrimSuffix(line, []byte("\n")))
		if err := v.Indent(jsontext.WithIndent("  ")); err != nil {
			return b
		}
		indented.Write(v)
		indented.WriteByte('\n')
	}
	return pretty.ColorJSON(indented.Bytes(), cliui.NewTerminalStyles(out), tones)
}

// structuredAsked reports whether the run asked for structured output, YAML or
// JSON, rather than a command's default.
func structuredAsked() bool { return output.yaml || output.json || output.jq != "" }

// structuredFlag is the flag that asked for structured output, to name in a
// refusal: --jq over --yaml over --json.
func structuredFlag() string {
	switch {
	case output.jq != "":
		return "--jq"
	case output.yaml:
		return "--yaml"
	}
	return "--json"
}

// A command's annotations about its output. Every runnable command has an
// output kind (TestEveryCommandHasAnOutputKind).
const (
	// outputKind is what the command writes to stdout, and so whether it
	// takes --yaml and --json.
	outputKind = "anfra.output"
	// artifactDesc is an artifact's alone: what it writes instead of a result,
	// such as "a shell script".
	artifactDesc = "anfra.artifact-desc"
	// defaultFormat is a structured command's format without --yaml or
	// --json: formatYAML, or formatPretty for a CLI-only command, as people
	// run them more than programs do.
	defaultFormat = "anfra.default-format"
	// interactable marks a command that may ask questions, when the run is
	// interactive (a terminal, without --non-interactive), and refuses instead
	// otherwise. A command that adds a question marks itself: nothing finds
	// them.
	interactable = "anfra.interactable"
	// prettyView marks a command whose result has a pretty view, reviewed
	// (docs/designs/cli-pretty.md#where-it-stands): it takes --pretty, and a
	// console shows it so by default. Every CLI-only command's; a core
	// command's when it's in prettyViews.
	prettyView = "anfra.pretty-view"
)

// prettyViews are the core commands whose results have a pretty view: one
// at a time, each once its view clears the bar (docs/designs/cli-pretty.md).
var prettyViews = map[string]bool{"query": true}

// The default formats.
const (
	formatYAML = "yaml"
	// formatPretty: the pretty format, laid out for a person
	// (docs/designs/cli-pretty.md), by the prettifier; --pretty on a command
	// with a pretty view.
	formatPretty = "pretty"
)

const (
	// outputStructured: a result with a fixed shape, in YAML or JSON. A core
	// command's is its op's answer; a CLI-only command's, a type of its own.
	outputStructured = "structured"
	// outputArtifact: something other than a result, such as a log or a file:
	// --yaml and --json are refused for good.
	outputArtifact = "artifact"
)

func annotate(cmd *cobra.Command, key, value string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[key] = value
}

// markStructuredOutput marks cmd as writing a result, as YAML by default.
func markStructuredOutput(cmd *cobra.Command) {
	annotate(cmd, outputKind, outputStructured)
	annotate(cmd, defaultFormat, formatYAML)
}

// markPrettyOutput marks cmd as writing a result, for a person by default
// (docs/designs/cli-pretty.md), and as YAML or JSON when asked.
func markPrettyOutput(cmd *cobra.Command) {
	annotate(cmd, outputKind, outputStructured)
	annotate(cmd, defaultFormat, formatPretty)
	annotate(cmd, prettyView, "true")
}

// wantsPretty reports whether a run of a command with a pretty view, or not
// (hasView), writes it: asked with --pretty, which a command without one
// refuses; or by default in a console, for every command, reviewed or not, a
// trial (docs/designs/cli-pretty.md#where-it-stands); unless the run asks
// for YAML or JSON.
func wantsPretty(hasView bool) bool {
	return hasView && output.pretty || forPerson()
}

// forPerson reports whether the run writes for a person rather than a
// program: a console's, or --pretty, without --yaml or --json. Its errors and
// warnings are then pretty too.
func forPerson() bool {
	return (output.pretty || output.inConsole) && !structuredAsked()
}

// markArtifactOutput marks cmd and the commands under it as writing writes,
// such as "a shell script", rather than a result.
func markArtifactOutput(cmd *cobra.Command, writes string) {
	annotate(cmd, outputKind, outputArtifact)
	annotate(cmd, artifactDesc, writes)
	for _, c := range cmd.Commands() {
		markArtifactOutput(c, writes)
	}
}

// markInteractable marks cmd as one that may ask questions.
func markInteractable(cmd *cobra.Command) { annotate(cmd, interactable, "true") }

// The root's command groups, in anfra --help: the ops, and the CLI's own.
const (
	coreGroup = "core"
	cliGroup  = "cli"
)

// addOutputFlags puts --yaml, --json and --non-interactive on root, for every
// command, and refuses a format where a command writes no structured output.
// It groups root's commands, core and CLI-only, and ends each command's help
// with its output. --no-interactive is --non-interactive under another name,
// accepted for good and shown nowhere: the name some CLIs use.
func addOutputFlags(root *cobra.Command, core []*cobra.Command) {
	pf := root.PersistentFlags()
	jqCode = nil // a tree starts without --jq, as its flags start at their defaults
	pf.BoolVar(&output.pretty, "pretty", false, "write the answer in the pretty format, laid out for a person: a command with a pretty view")
	pf.BoolVar(&output.yaml, "yaml", false, "write the answer as YAML, whatever the command's default")
	pf.BoolVar(&output.json, "json", false, "write the answer, warnings and errors as JSON")
	pf.StringVar(&output.jq, "jq", "", "filter the answer with a jq expression, writing what it gives as YAML, or JSON with --json")
	pf.BoolVar(&output.schema, "schema", false, "write the command's input and output, as JSON Schema, instead of running it: each input field is a flag, --<field>, unless its x-cli says it's positional")
	pf.BoolVar(&output.nonInteractive, "non-interactive", false, "ask no questions and print no notices: refuse what would need an answer")
	root.SetGlobalNormalizationFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		// A flag by its field's name works too: --data_source, as --schema names it.
		name = strings.ReplaceAll(name, "_", "-")
		if name == "no-interactive" {
			name = "non-interactive"
		}
		return pflag.NormalizedName(name)
	})
	// cobra's own commands, added now rather than when it runs, to be marked.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	for _, c := range root.Commands() {
		switch c.Name() {
		case "help":
			markArtifactOutput(c, "its text")
		case "completion":
			markArtifactOutput(c, "a shell script")
		}
	}

	root.AddGroup(&cobra.Group{ID: coreGroup, Title: "Core commands:"}, &cobra.Group{ID: cliGroup, Title: "CLI-only commands:"})
	for _, c := range root.Commands() {
		c.GroupID = cliGroup
		if slices.Contains(core, c) {
			c.GroupID = coreGroup
		}
	}
	root.SetHelpCommandGroupID(cliGroup)
	root.SetCompletionCommandGroupID(cliGroup)
	describeOutput(root)
	// cobra's own refusals, of a flag or of the args, are the caller's mistakes.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return flagRefused(err) })
	refuseArgsAsRequests(root)

	schemaInstead(root)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if output.schema && cmd.Annotations[outputKind] == outputArtifact {
			return artifactRefused(cmd, "--schema")
		}
		if output.pretty {
			switch {
			case output.yaml || output.json || output.jq != "":
				return flagViolated("--pretty", "--pretty can't be combined with --yaml, --json, or --jq. Pass one format.")
			case cmd.Annotations[outputKind] == outputArtifact:
				return artifactRefused(cmd, "--pretty")
			case cmd.Annotations[prettyView] == "":
				return flagViolated("--pretty", cmd.CommandPath()+" doesn't take --pretty yet. Use --yaml or --json.")
			}
			return nil
		}
		switch {
		case output.yaml && output.json:
			return flagViolated("--json", "--yaml and --json can't be combined. Pass one.")
		case !structuredAsked():
			return nil
		}
		flag := structuredFlag()
		if cmd.Annotations[outputKind] == outputArtifact {
			return artifactRefused(cmd, flag)
		}
		if output.jq != "" {
			return compileJQ(output.jq)
		}
		return nil
	}
}

// describeOutput ends the help of cmd and every command under it with its
// output and its questions, from its annotations, so the help can't say
// otherwise: sections laid out as cobra's Flags: is.
func describeOutput(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		describeOutput(c)
	}
	if !cmd.Runnable() {
		return
	}
	var rows [][2]string
	switch cmd.Annotations[outputKind] {
	case outputArtifact:
		rows = [][2]string{{cmd.Annotations[artifactDesc], ""}}
	default:
		switch {
		case cmd.Annotations[defaultFormat] == formatPretty:
			rows = [][2]string{{"default", "the pretty format"}, {"--yaml", "a YAML document"}}
		case cmd.Annotations[prettyView] != "":
			rows = [][2]string{{"default", "a YAML document"}, {"--pretty", "the pretty format, the console's default"}}
		default:
			rows = [][2]string{{"default", "a YAML document"}}
		}
		rows = append(rows, [2]string{"--json", "a JSON document"}, [2]string{"--schema", "its input and output, as JSON Schema"})
	}
	sections := helpSection("Output:", rows)
	if cmd.Annotations[interactable] != "" {
		sections += "\n\n" + helpSection("Questions:", [][2]string{
			{"on a terminal", "asked when an answer is needed"},
			{"without a terminal", "never asked: fails instead"},
			{"--non-interactive", "as without a terminal"},
		})
	}
	long := cmd.Long
	if long == "" {
		long = cmd.Short
	}
	cmd.Long = strings.TrimRight(long, "\n") + "\n\n" + sections
}

// helpSection is a heading, then its rows indented, their second columns
// aligned; a row with no second column is its first alone.
func helpSection(heading string, rows [][2]string) string {
	width := 0
	for _, r := range rows {
		if r[1] != "" {
			width = max(width, len(r[0]))
		}
	}
	var b strings.Builder
	b.WriteString(heading)
	for _, r := range rows {
		b.WriteString("\n  " + r[0])
		if r[1] != "" {
			b.WriteString(strings.Repeat(" ", width-len(r[0])+3) + r[1])
		}
	}
	return b.String()
}

// flagRefused is pflag's refusal of a flag as invalid_request, a violation on
// the flag as typed: unknown, missing its value, or given one it can't parse.
func flagRefused(err error) error {
	msg := err.Error()
	field, code := "flags", "invalid"
	for _, p := range []struct {
		re   *regexp.Regexp
		code string
	}{
		{regexp.MustCompile(`^unknown (?:shorthand )?flag: (?:'.' in )?(-\S+)`), "unknown"},
		{regexp.MustCompile(`^flag needs an argument: (?:'.' in )?(-\S+)`), "required"},
		{regexp.MustCompile(`^invalid argument ".*" for "(?:-., )?(--?[^"]+)" flag`), "invalid"},
		{regexp.MustCompile(`^bad flag syntax: (\S+)`), "invalid"},
	} {
		if m := p.re.FindStringSubmatch(msg); m != nil {
			field, code = m[1], p.code
			break
		}
	}
	return apperr.NewWith(apperr.InvalidRequest, msg, apperr.Violate(apperr.Violation{Field: field, Code: code, Message: msg}))
}

// refuseArgsAsRequests makes cmd's and its commands' refusals of their args,
// cobra's, invalid_request: too many, too few, an arg it doesn't take.
func refuseArgsAsRequests(cmd *cobra.Command) {
	for _, c := range cmd.Commands() {
		refuseArgsAsRequests(c)
	}
	if cmd.Args == nil {
		return
	}
	validate := cmd.Args
	cmd.Args = func(c *cobra.Command, args []string) error {
		err := validate(c, args)
		if err == nil || apperr.From(err).Code != apperr.InternalServerError.Code() {
			return err
		}
		return apperr.NewWith(apperr.InvalidRequest, err.Error(),
			apperr.Violate(apperr.Violation{Field: "args", Code: "invalid", Message: err.Error()}))
	}
}

// writeResult writes a CLI-only command's result, v, to cmd's stdout: for a
// person when that's the command's default (pretty.Render), else as an op's
// answer is written, YAML in its fields' order or one JSON document.
func writeResult(cmd *cobra.Command, v any) error {
	out := cmd.OutOrStdout()
	if cmd.Annotations[defaultFormat] == formatPretty && !structuredAsked() {
		return pretty.Render(out, v, cliui.Width(out))
	}
	return writeStructured(out, v)
}

// writeStructured writes v as YAML, or with --json as JSON.
func writeStructured(w io.Writer, v any) error {
	b, err := jsonkit.Marshal(v)
	if err != nil {
		return err
	}
	return renderTo(b, "application/json", w)
}

// artifactRefused is a format flag refused by an artifact's command, which
// writes something other than a result.
func artifactRefused(cmd *cobra.Command, flag string) error {
	return flagViolated(flag, cmd.CommandPath()+" doesn't take "+flag+", because it writes "+cmd.Annotations[artifactDesc]+", not a result.")
}

// flagViolated is flag refused with msg, as a violation of that flag.
func flagViolated(flag, msg string) error {
	return apperr.NewWith(apperr.ValidationFailed, msg,
		apperr.Violate(apperr.Violation{Field: flag, Code: "invalid", Message: msg}))
}

// flagAsked reports whether args give flag, a boolean, before any --: for
// an error printed before the flags are parsed, or because they could not be.
func flagAsked(args []string, flag string) bool {
	end := slices.Index(args, "--")
	if end < 0 {
		end = len(args)
	}
	return slices.Contains(args[:end], flag) || slices.Contains(args[:end], flag+"=true")
}

// jsonAsked reports whether args ask for --json, for an error printed before
// the flags are parsed, or because they could not be.
func jsonAsked(args []string) bool {
	end := slices.Index(args, "--")
	if end < 0 {
		end = len(args)
	}
	return slices.Contains(args[:end], "--json") || slices.Contains(args[:end], "--json=true")
}

// jsonToYAML is the JSON document b as YAML, in b's order: JSON is YAML, so b
// is read as it is, and only its style dropped.
func jsonToYAML(b []byte) ([]byte, error) {
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	if len(n.Content) == 0 {
		return nil, errors.New("no JSON document")
	}
	blockStyle(&n)
	return marshalYAML(&n)
}

// marshalYAML is v as the CLI writes YAML: indented by 2, as most YAML is
// (yaml.v3's own is 4), so a deep answer, a query's columns say, costs its
// reader, often an agent, fewer characters.
func marshalYAML(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// writeJSONDoc writes {key: n} as one line of a JSONL stream, keeping n's order:
// n is an error or a warning, which the CLI edits as a node.
func writeJSONDoc(w io.Writer, key string, n *yaml.Node) {
	var b bytes.Buffer
	enc := jsontext.NewEncoder(&b)
	_ = enc.WriteToken(jsontext.BeginObject)
	_ = enc.WriteToken(jsontext.String(key))
	_ = encodeNode(enc, n)
	_ = enc.WriteToken(jsontext.EndObject)
	_, _ = w.Write(paintJSON(w, b.Bytes(), docTones))
}

// encodeNode writes n, a YAML node read from JSON or made by setKey, as JSON.
func encodeNode(enc *jsontext.Encoder, n *yaml.Node) error {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return enc.WriteToken(jsontext.Null)
		}
		return encodeNode(enc, n.Content[0])
	case yaml.MappingNode:
		if err := enc.WriteToken(jsontext.BeginObject); err != nil {
			return err
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if err := enc.WriteToken(jsontext.String(n.Content[i].Value)); err != nil {
				return err
			}
			if err := encodeNode(enc, n.Content[i+1]); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndObject)
	case yaml.SequenceNode:
		if err := enc.WriteToken(jsontext.BeginArray); err != nil {
			return err
		}
		for _, c := range n.Content {
			if err := encodeNode(enc, c); err != nil {
				return err
			}
		}
		return enc.WriteToken(jsontext.EndArray)
	case yaml.AliasNode:
		return encodeNode(enc, n.Alias)
	}
	switch n.ShortTag() {
	case "!!null":
		return enc.WriteToken(jsontext.Null)
	case "!!bool":
		return enc.WriteToken(jsontext.Bool(n.Value == "true"))
	case "!!int", "!!float":
		if v := jsontext.Value(n.Value); v.IsValid() {
			return enc.WriteValue(v)
		}
	}
	return enc.WriteToken(jsontext.String(n.Value))
}
