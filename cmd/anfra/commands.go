package main

import (
	"context"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/pretty"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/appstep"
	"github.com/holistics/anfra/shared/apptracing"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/holistics/anfra/shared/logkit"
)

// appCommands builds the cobra tree from the registry: one command per
// registered command.Command, nested by its dotted name ("query.compile" is
// `query compile`). Flags are generated from each command's args, so a command
// added to the registry shows up in the CLI automatically — no separate CLI
// registration to keep in sync.
func appCommands() []*cobra.Command {
	byName := map[string]*cobra.Command{}
	var top []*cobra.Command
	// A group's command is registered before its subcommands (fewer dots first),
	// so each subcommand finds its parent.
	cmds := slices.Clone(app.Commands)
	slices.SortStableFunc(cmds, func(a, b command.Command) int {
		return strings.Count(a.Name(), ".") - strings.Count(b.Name(), ".")
	})
	for _, c := range cmds {
		cc := buildCobraCommand(c)
		byName[c.Name()] = cc
		i := strings.LastIndex(c.Name(), ".")
		if i < 0 {
			top = append(top, cc)
			continue
		}
		parent, ok := byName[c.Name()[:i]]
		if !ok {
			panic("command " + c.Name() + " has no registered group " + c.Name()[:i])
		}
		parent.AddCommand(cc)
	}
	return top
}

func buildCobraCommand(c command.Command) *cobra.Command {
	name := c.Name()[strings.LastIndex(c.Name(), ".")+1:]
	cmd := &cobra.Command{Use: name, Short: c.Short(), Long: c.Long(), Args: cobra.NoArgs}
	annotate(cmd, opAnnotation, c.Name())
	markStructuredOutput(cmd) // its op's answer
	if prettyViews[c.Name()] {
		annotate(cmd, prettyView, "true")
	}
	// Show flags in declaration order (else pflag sorts them alphabetically).
	cmd.Flags().SortFlags = false

	args := c.Args()
	flagArg := map[string]string{} // flag name -> the arg it sets
	var positional *command.Arg
	for i, a := range args {
		if a.Positional {
			positional = &args[i]
			// A positional has no flag to carry its help, so the long help does.
			long := c.Long()
			if long == "" {
				long = c.Short()
			}
			usage := a.Usage
			if a.Stdin {
				usage += "; read from stdin when omitted"
			}
			cmd.Long = long + "\n\nArguments:\n  " + a.Name + "  " + usage
			if a.Type == command.ArgStringArray {
				cmd.Use += " [" + a.Name + "...]"
				cmd.Args = cobra.ArbitraryArgs
			} else {
				cmd.Use += " [" + a.Name + "]"
				cmd.Args = cobra.MaximumNArgs(1)
			}
			continue
		}
		// What the op's schema enforces, said once for every name of the arg.
		usage := a.Usage
		if a.Required {
			usage += " (required)"
		}
		if a.Group != "" {
			usage += " (exactly one of: " + strings.Join(groupFlags(args, a.Group), ", ") + ")"
		}
		if len(a.Enum) > 0 {
			usage += " (one of: " + strings.Join(a.Enum, ", ") + ")"
		}
		if a.Type == command.ArgObject {
			usage += " (a JSON object"
			if len(a.Fields) > 0 {
				usage += ", or set its fields with --" + a.Flag() + ".<field>"
			}
			usage += ")"
		}
		if a.Type == command.ArgObjectArray {
			usage += " (a JSON array of objects)"
		}
		// pflag has no native aliases, so each alias is a flag of its own, setting
		// the same arg: the op knows each arg by one name.
		addFlag(cmd, a, a.Flag(), a.Shorthand, usage)
		flagArg[a.Flag()] = a.Name
		for _, al := range a.Aliases {
			addFlag(cmd, a, al, "", "alias of --"+a.Flag())
			flagArg[al] = a.Name
		}
		// An object's fields, each a flag of its own, named by its path:
		// --format-options.header sets format_options.header.
		for _, fa := range a.Fields {
			usage := fa.Usage
			if len(fa.Enum) > 0 {
				usage += " (one of: " + strings.Join(fa.Enum, ", ") + ")"
			}
			name := a.Flag() + "." + fa.Flag()
			addFlag(cmd, fa, name, "", usage)
			flagArg[name] = a.Name + "." + fa.Name
		}
	}

	// An export's answer is a link; the CLI downloads it, unless asked for the link.
	if presentationOf(c) == presentDownload {
		cmd.Flags().StringP("output", "o", "", "write the file here instead of to stdout")
		cmd.Flags().Bool("link", false, "print the export's link instead of downloading its file")
	}

	// The command's input as its flags make it, with text for its positional,
	// checked as the command would before running: for a console asking ahead.
	inputChecks[cmd] = func(text []string) error {
		values, err := flagValues(cmd.Flags(), args, flagArg)
		if err != nil {
			return err
		}
		if positional != nil && len(text) > 0 {
			if positional.Type == command.ArgStringArray {
				values[positional.Name] = text
			} else {
				values[positional.Name] = text[0]
			}
		}
		input, err := jsonkit.Marshal(values)
		if err != nil {
			return err
		}
		return c.Check(input)
	}

	cmd.RunE = func(runCmd *cobra.Command, posArgs []string) error {
		d := delivery{stdout: runCmd.OutOrStdout()}
		if presentationOf(c) == presentDownload {
			d.output, _ = runCmd.Flags().GetString("output")
			d.link, _ = runCmd.Flags().GetBool("link")
			if structuredAsked() && !d.link {
				flag := structuredFlag()
				return flagViolated(flag, flag+" only works with export --link, which prints the export's link instead of writing the file.")
			}
		}
		values, err := flagValues(runCmd.Flags(), args, flagArg)
		if err != nil {
			return err
		}
		if positional != nil && len(posArgs) > 0 {
			if positional.Type == command.ArgStringArray {
				values[positional.Name] = posArgs
			} else {
				values[positional.Name] = posArgs[0]
			}
		}
		if err := applyStdin(args, values); err != nil {
			return err
		}
		if err := runCommand(runCmd.Context(), c, values, d); err != nil {
			return &commandError{path: runCmd.CommandPath(), args: args, err: err}
		}
		return nil
	}
	return cmd
}

// flagValues are the args the user set by flag, as the op's input has them:
// only those set, so an unset one is absent, and the op applies its default and
// sees which of a group are set. An object arg's flag is JSON, refused when it is
// not an object; or its fields are set by their own flags, and make the object,
// but not both. An object array arg's flag is JSON, refused when it is not an
// array of objects.
func flagValues(fs *pflag.FlagSet, args []command.Arg, flagArg map[string]string) (map[string]any, error) {
	values := map[string]any{}
	objects := map[string]map[string]any{} // an object arg set by its fields' flags
	var err error
	fs.Visit(func(f *pflag.Flag) {
		key, ok := flagArg[f.Name]
		if !ok || err != nil {
			return
		}
		if arg, field, ok := strings.Cut(key, "."); ok {
			if objects[arg] == nil {
				objects[arg] = map[string]any{}
			}
			objects[arg][field] = flagValue(f, fieldType(args, arg, field))
			return
		}
		raw := jsontext.Value(strings.TrimSpace(f.Value.String()))
		t := argType(args, key)
		if t == command.ArgObjectArray {
			var objs []map[string]any
			if jsonkit.Unmarshal(raw, &objs) != nil || objs == nil || slices.ContainsFunc(objs, func(o map[string]any) bool { return o == nil }) {
				err = refused("--"+f.Name, "invalid", fmt.Sprintf("--%s takes a JSON array of objects, such as '[{\"name\": \"a\"}]'.", f.Name))
				return
			}
			values[key] = raw
			return
		}
		if t != command.ArgObject {
			values[key] = flagValue(f, t)
			return
		}
		var obj map[string]any
		if jsonkit.Unmarshal(raw, &obj) != nil || obj == nil {
			err = refused("--"+f.Name, "invalid", fmt.Sprintf("--%s takes a JSON object, such as '{\"filters\": []}'.", f.Name))
			return
		}
		values[key] = raw
	})
	if err != nil {
		return nil, err
	}
	for _, a := range args {
		obj, ok := objects[a.Name]
		if !ok {
			continue
		}
		if _, set := values[a.Name]; set {
			return nil, refused("--"+a.Flag(), "invalid", fmt.Sprintf("--%s takes a JSON object or its --%s.<field> flags, not both.", a.Flag(), a.Flag()))
		}
		values[a.Name] = obj
	}
	return values, nil
}

// flagValue is a flag's value as its arg's type has it in the op's input: a
// slice, a bool, a number, or the string.
func flagValue(f *pflag.Flag, t command.ArgType) any {
	if v, ok := f.Value.(pflag.SliceValue); ok {
		return v.GetSlice()
	}
	switch t {
	case command.ArgBool:
		return f.Value.String() == "true"
	case command.ArgInt:
		return jsontext.Value(f.Value.String())
	}
	return f.Value.String()
}

func addFlag(cmd *cobra.Command, a command.Arg, name, short, usage string) {
	switch a.Type {
	case command.ArgString, command.ArgObject, command.ArgObjectArray:
		cmd.Flags().StringP(name, short, a.Default, usage)
	case command.ArgInt:
		cmd.Flags().IntP(name, short, 0, usage)
	case command.ArgBool:
		cmd.Flags().BoolP(name, short, false, usage)
	case command.ArgStringArray:
		cmd.Flags().StringSliceP(name, short, nil, usage)
	}
}

// argType is the type of the arg named name.
func argType(args []command.Arg, name string) command.ArgType {
	for _, a := range args {
		if a.Name == name {
			return a.Type
		}
	}
	return ""
}

// fieldType is the type of the field named field of the object arg named arg.
func fieldType(args []command.Arg, arg, field string) command.ArgType {
	for _, a := range args {
		if a.Name == arg {
			return argType(a.Fields, field)
		}
	}
	return ""
}

// groupFlags are the CLI flags of a group's args.
func groupFlags(args []command.Arg, group string) []string {
	var out []string
	for _, a := range args {
		if a.Group == group {
			out = append(out, "--"+a.Flag())
		}
	}
	return out
}

// applyStdin fills a stdin arg from piped stdin when it was left unset, so
// e.g. `cat query.aql | anfra query -d sales` works.
func applyStdin(args []command.Arg, values map[string]any) error {
	for _, a := range args {
		if !a.Stdin {
			continue
		}
		if s, _ := values[a.Name].(string); strings.TrimSpace(s) != "" {
			continue
		}
		stat, _ := os.Stdin.Stat()
		if stat != nil && (stat.Mode()&os.ModeCharDevice) != 0 {
			return nil // a terminal — nothing piped
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read %s from stdin: %w", a.Name, err)
		}
		if v := strings.TrimSpace(string(data)); v != "" {
			values[a.Name] = v
		}
		return nil // one stdin
	}
	return nil
}

// runCommand runs a command as its op: on the repo's running server when there
// is one (found through its runtime file), otherwise in this process, spawning
// only the sidecars it needs.
//
// The command is the root of its trace: the op's span is under it, here or on
// the server, which continues the trace it is sent.
func runCommand(ctx context.Context, c command.Command, args map[string]any, d delivery) error {
	ctx, span := tracer.Start(ctx, "anfra "+c.Name())
	defer span.End()

	body, err := invoke(ctx, c, args)
	if err != nil {
		return err
	}
	return deliver(ctx, c, body, d)
}

// inputChecks are each core command's check of the input its parsed flags
// make, with text for its positional, without running it (command.Check):
// for a console, to warn of a flag missing before an entry fails for it.
// Keyed by command, they're a tree's, as resultTypes are.
var inputChecks = map[*cobra.Command]func(text []string) error{}

// cliLocal are the core commands the CLI runs itself while the repo's server
// runs: their answer is about the anfra running them, so forwarded they would
// report the server's facts as this CLI's (docs/designs/cli-vs-warm-server.md).
var cliLocal = map[string]bool{
	"version": true, // this anfra's version, and its update and skills checks
}

// invoke runs a command as runCommand does, and returns its answer's body rather
// than showing it: for a command that runs another (publish runs validate).
func invoke(ctx context.Context, c command.Command, args map[string]any) ([]byte, error) {
	repoDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve repo dir: %w", err)
	}
	input, err := jsonkit.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("encode args: %w", err)
	}
	if srv, ok := findServer(ctx, repo.Resolve(repoDir)); ok {
		forward := !cliLocal[c.Name()]
		warnIfOtherVersion(os.Stderr, srv, forward)
		if forward {
			return callServe(ctx, srv.URL, c.Name(), input)
		}
	}

	var body []byte
	err = withRepo(ctx, false, func(ctx context.Context, h hostContext) error {
		err := func() error {
			clients, closeSidecars, err := startNeededSidecars(ctx, h, c, input)
			if err != nil {
				return err
			}
			defer closeSidecars()

			out, err := app.Invoke(ctx, h.commandContext(clients), c.Name(), input)
			if err != nil {
				return err
			}
			if body, err = jsonkit.Marshal(out); err != nil {
				return fmt.Errorf("marshal result: %w", err)
			}
			return nil
		}()
		if err != nil {
			logFailure(ctx, h.cfg.Logger, c.Name(), err)
		}
		return err
	})
	return body, err
}

// logFailure logs a command's failure, as httpkit logs a request's: its code, its
// scope and its whole chain, at the scope's level. The terminal shows what the
// error lets callers see; the log keeps the rest, whatever is shown.
func logFailure(ctx context.Context, logger *slog.Logger, name string, err error) {
	e := apperr.From(err)
	pub := e.Code.Public()
	level := slog.LevelInfo
	switch pub.Scope() {
	case apperr.Client:
		level = slog.LevelWarn
	case apperr.Server:
		level = slog.LevelError
	}
	attrs := []slog.Attr{
		slog.Any(logkit.EventKey, eventCommandFailed),
		slog.String("command", name),
		slog.String("code", pub.String()),
		slog.String("scope", string(pub.Scope())),
	}
	if e.Code != pub {
		attrs = append(attrs, slog.String("internal_code", e.Code.Qualified()))
	}
	attrs = append(attrs, slog.String("error", err.Error()))
	logger.LogAttrs(ctx, level, "Command failed", attrs...)
}

// delivery is how the CLI delivers an answer: to stdout, an export as its
// file, to stdout or to output, or as its link alone.
type delivery struct {
	stdout io.Writer
	output string
	link   bool
}

// presentation is how the CLI shows a command's answer.
type presentation int

const (
	presentYAML     presentation = iota // its JSON, as YAML
	presentDownload                     // an export: its file (--output), or its link (--link)
)

// presentationOf is how the CLI shows c's answer, by what the answer is: the
// one place that decides it, for the flags it takes and for how it is shown.
func presentationOf(c command.Command) presentation {
	if c.Output() == reflect.TypeFor[command.ExportLink]() {
		return presentDownload
	}
	return presentYAML
}

// deliver shows an answer as presentationOf says: an export's file downloaded,
// or its link shown when asked for (--link), and any other answer shown.
func deliver(ctx context.Context, c command.Command, body []byte, d delivery) error {
	if presentationOf(c) != presentDownload || d.link {
		return presentTo(c, body, d.stdout)
	}
	var link command.ExportLink
	if err := jsonkit.Unmarshal(body, &link); err != nil {
		return fmt.Errorf("decode the export's link: %w", err)
	}
	return download(ctx, link.URL, d.output, d.stdout)
}

// download copies the file at link (file:// from the CLI's own exports,
// http(s):// from a server's) to output, or without one to stdout. A partly written output
// is removed; the CLI's own file, once written out, is too: it was made for
// this.
func download(ctx context.Context, link, output string, stdout io.Writer) error {
	src, err := openLink(ctx, link)
	if err != nil {
		return err
	}
	defer src.Close()
	if err := copyTo(output, src, stdout); err != nil {
		return err
	}
	if f, ok := src.(*os.File); ok {
		_ = os.Remove(f.Name())
	}
	return nil
}

func copyTo(output string, src io.Reader, stdout io.Writer) error {
	if output == "" {
		_, err := io.Copy(stdout, src)
		return err
	}
	dst, err := os.Create(output)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(output)
		return clierr.Unreachablef(clierr.AnfraServe, err, "download the export: %v", err)
	}
	return dst.Close()
}

func openLink(ctx context.Context, link string) (io.ReadCloser, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("read the export's link %q: %w", link, err)
	}
	switch u.Scheme {
	case "file":
		return os.Open(u.Path)
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, clierr.Unreachablef(clierr.AnfraServe, err, "download the export: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			// anfra serve's error, shown as any of its errors is (an expired link,
			// say); a server of another kind's status.
			if body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)); err == nil {
				if re, ok := remoteErrorFrom(body); ok {
					return nil, re
				}
			}
			return nil, clierr.Unreachablef(clierr.AnfraServe, nil, "download the export: the server returned %s", resp.Status)
		}
		return resp.Body, nil
	}
	return nil, fmt.Errorf("the export's link %q is not one the CLI can download", link)
}

// presentTo shows an answer as YAML on out, and turns an invalid verdict into a
// silent exit code 1: the answer says why.
func presentTo(c command.Command, body []byte, out io.Writer) error {
	body, err := addHints(c, body)
	if err != nil {
		return err
	}
	if wantsPretty(prettyViews[c.Name()]) {
		// Its pretty view reads its type's hints: the answer, its JSON, as
		// that type again, though a server answered it.
		v := reflect.New(c.Output())
		if err := jsonkit.Unmarshal(body, v.Interface()); err != nil {
			return fmt.Errorf("decode %s's answer: %w", c.Name(), err)
		}
		if err := pretty.Render(out, v.Elem().Interface(), cliui.Width(out)); err != nil {
			return err
		}
	} else if err := renderTo(body, "application/json", out); err != nil {
		return err
	}
	valid, err := c.Valid(body)
	if err != nil {
		return err
	}
	if !valid {
		return &exitCodeError{code: 1}
	}
	return nil
}

// resultHints is hints that the CLI adds to a command's answer, by the
// command's name. As codeHelp is for errors, it's for advice only the CLI
// can give: the same command, served over HTTP or MCP, has no CLI commands
// to point to.
var resultHints = map[string]func() []string{
	"status": func() []string {
		c, err := repoconfig.Read(".")
		if err != nil || c.Org.URL == "" {
			return nil
		}
		return []string{"To see whether this folder is behind its org, or has changes it hasn't published, run `anfra publish status`."}
	},
}

// addHints adds resultHints to an answer's hints field, keeping its fields'
// order: the answer, decoded as its type, encoded again.
func addHints(c command.Command, body []byte) ([]byte, error) {
	hints := resultHints[c.Name()]
	if hints == nil {
		return body, nil
	}
	more := hints()
	if len(more) == 0 {
		return body, nil
	}
	v := reflect.New(c.Output())
	if err := jsonkit.Unmarshal(body, v.Interface()); err != nil {
		return nil, fmt.Errorf("decode %s's answer: %w", c.Name(), err)
	}
	f := v.Elem().FieldByName("Hints")
	if !f.IsValid() || f.Type() != reflect.TypeFor[[]string]() {
		return body, nil
	}
	f.Set(reflect.AppendSlice(f, reflect.ValueOf(more)))
	return jsonkit.Marshal(v.Interface())
}

// startNeededSidecars spawns just the sidecars the command declares it needs
// for these args, returning the clients and a single close func (LIFO).
func startNeededSidecars(ctx context.Context, h hostContext, c command.Command, input []byte) (command.Clients, func(), error) {
	need := c.Needs(input)
	var clients command.Clients
	var closers []func()
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	if need.Node {
		node := anfranode.New(h.cfg)
		if err := startSidecar(ctx, anfranode.Name, node.Start); err != nil {
			closeAll()
			return command.Clients{}, nil, err
		}
		closers = append(closers, func() { node.Close(ctx) })
		clients.Node = node.Client()
	}
	if need.CanalQuery {
		canal := canalquery.New(h.cfg)
		if err := startSidecar(ctx, canalquery.Name, canal.Start); err != nil {
			closeAll()
			return command.Clients{}, nil, err
		}
		closers = append(closers, func() { canal.Close(ctx) })
		clients.CanalQuery = canal.Client()
	}
	return clients, closeAll, nil
}

// startSidecarStep is starting a sidecar a command needs: public, so that a
// failure in it says which sidecar anfra was starting in its context, and its
// message is the failure's own.
var startSidecarStep = appstep.Define(clierr.NS, "start_sidecar", appstep.Public("start {sidecar}"))

// startSidecar runs start, the sidecar's, in its step. Any failure is the
// sidecar being unavailable, so that the CLI adds the help for it; its message
// is its cause's when causes are shown, the code's own otherwise.
func startSidecar(ctx context.Context, sidecar string, start func(context.Context) error) (err error) {
	ctx, span := apptracing.Start(ctx, startSidecarStep, "sidecar", sidecar)
	defer span.End(&err)
	if err := start(ctx); err != nil {
		return apperr.Encapsulate(err, errcode.SidecarUnavailable, "")
	}
	return nil
}

// renderTo prints a command response. A JSON body (per its Content-Type) is
// converted to YAML for readability; anything else is written through unchanged.
func renderTo(body []byte, contentType string, out io.Writer) error {
	if !isJSONContentType(contentType) {
		_, err := out.Write(body)
		return err
	}
	if jqCode != nil {
		return renderJQ(body, out)
	}
	if output.json {
		v := jsontext.Value(slices.Clone(body))
		if err := v.Compact(); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		_, err := out.Write(paintJSON(out, append(v, '\n'), nil))
		return err
	}
	b, err := jsonToYAML(body)
	if err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	_, err = out.Write(paintYAML(out, b, nil))
	return err
}

func isJSONContentType(contentType string) bool {
	return strings.HasPrefix(strings.TrimSpace(contentType), "application/json")
}
