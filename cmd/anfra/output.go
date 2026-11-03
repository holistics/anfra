package main

import (
	"encoding/json/jsontext"
	"errors"
	"io"
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
// --non-interactive are independent: --json sets the format only.
var output struct {
	yaml           bool
	json           bool
	nonInteractive bool
}

// structuredAsked reports whether the run asked for structured output, YAML or
// JSON, rather than a command's default.
func structuredAsked() bool { return output.yaml || output.json }

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
)

// The default formats.
const (
	formatYAML = "yaml"
	// formatPretty: the pretty format, laid out for a person
	// (docs/designs/cli-pretty.md): by the prettifier, or by a command's own
	// code until the prettifier covers it. It has no flag yet.
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
	pf.BoolVar(&output.yaml, "yaml", false, "write the answer as YAML, whatever the command's default")
	pf.BoolVar(&output.json, "json", false, "write the answer, warnings and errors as JSON")
	pf.BoolVar(&output.nonInteractive, "non-interactive", false, "ask no questions and print no notices: refuse what would need an answer")
	root.SetGlobalNormalizationFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
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

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		flag := "--json"
		switch {
		case output.yaml && output.json:
			return formatRefused("--json", "pass --yaml or --json, not both")
		case output.yaml:
			flag = "--yaml"
		case !output.json:
			return nil
		}
		if cmd.Annotations[outputKind] == outputArtifact {
			return formatRefused(flag, cmd.CommandPath()+" writes "+cmd.Annotations[artifactDesc]+", not structured output")
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
		if cmd.Annotations[defaultFormat] == formatPretty {
			rows = [][2]string{{"default", "the pretty format"}, {"--yaml", "a YAML document"}}
		} else {
			rows = [][2]string{{"default", "a YAML document"}}
		}
		rows = append(rows, [2]string{"--json", "a JSON document"})
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

// formatRefused is a format flag refused, as a violation of that flag.
func formatRefused(flag, why string) error {
	msg := flag + " isn't available here: " + why + "."
	return apperr.NewWith(apperr.ValidationFailed, msg,
		apperr.Violate(apperr.Violation{Field: flag, Code: "invalid", Message: msg}))
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
	return yaml.Marshal(&n)
}

// writeJSONDoc writes {key: n} as one line of a JSONL stream, keeping n's order:
// n is an error or a warning, which the CLI edits as a node.
func writeJSONDoc(w io.Writer, key string, n *yaml.Node) {
	enc := jsontext.NewEncoder(w)
	_ = enc.WriteToken(jsontext.BeginObject)
	_ = enc.WriteToken(jsontext.String(key))
	_ = encodeNode(enc, n)
	_ = enc.WriteToken(jsontext.EndObject)
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
