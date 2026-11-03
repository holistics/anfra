package main

import (
	"encoding/json/jsontext"
	"errors"
	"io"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/shared/apperr"
)

// output is how this run writes its structured output, and whether it may
// ask: the root's --json and --non-interactive
// (docs/designs/commands-and-api.md#cli-output). The two are independent: --json
// sets the format only.
var output struct {
	json           bool
	nonInteractive bool
}

// A command's output kind: what it writes to stdout, and so whether it takes
// --json. Every runnable command has one (TestEveryCommandHasAnOutputKind).
const (
	outputKind = "anfra.output"
	// artifactDescription is an artifact's alone: what it writes instead of a
	// result, such as "a shell script".
	artifactDescription = "anfra.artifact"
)

const (
	// outputStructured: a result with a fixed shape, in YAML or JSON. A core
	// command's is its op's answer; a CLI-only command's, a type of its own.
	outputStructured = "structured"
	// outputArtifact: something other than a result, such as a log or a file:
	// --json is refused for good.
	outputArtifact = "artifact"
	// noStructuredOutputYet: a CLI-only command that still writes prose, until
	// it has a structured result (docs/implementations/cli-json.md, step 2).
	noStructuredOutputYet = "no-structured-output-yet"
)

func setOutputKind(cmd *cobra.Command, kind, writes string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[outputKind] = kind
	if writes != "" {
		cmd.Annotations[artifactDescription] = writes
	}
}

func markStructuredOutput(cmd *cobra.Command) { setOutputKind(cmd, outputStructured, "") }

// markArtifactOutput marks cmd and the commands under it as writing writes,
// such as "a shell script", rather than a result.
func markArtifactOutput(cmd *cobra.Command, writes string) {
	setOutputKind(cmd, outputArtifact, writes)
	for _, c := range cmd.Commands() {
		markArtifactOutput(c, writes)
	}
}

func markNoStructuredOutputYet(cmd *cobra.Command) {
	setOutputKind(cmd, noStructuredOutputYet, "")
	for _, c := range cmd.Commands() {
		markNoStructuredOutputYet(c)
	}
}

// addOutputFlags puts --json and --non-interactive on root, for every command,
// and refuses --json where a command writes no structured output.
// --no-interactive is --non-interactive under another name, accepted for good
// and shown nowhere: the name some CLIs use.
func addOutputFlags(root *cobra.Command) {
	pf := root.PersistentFlags()
	pf.BoolVar(&output.json, "json", false, "write the answer, warnings and errors as JSON instead of YAML")
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
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if !output.json {
			return nil
		}
		switch cmd.Annotations[outputKind] {
		case outputStructured:
			return nil
		case outputArtifact:
			return jsonRefused(cmd.CommandPath() + " writes " + cmd.Annotations[artifactDescription] + ", not structured output")
		default:
			return jsonRefused(cmd.CommandPath() + " doesn't write structured output yet; run it without --json")
		}
	}
}

// jsonRefused is --json refused, as a violation of that flag.
func jsonRefused(why string) error {
	msg := "--json isn't available here: " + why + "."
	return apperr.NewWith(apperr.ValidationFailed, msg,
		apperr.Violate(apperr.Violation{Field: "--json", Code: "invalid", Message: msg}))
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
