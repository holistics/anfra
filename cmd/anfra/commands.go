package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

// appCommands builds the cobra tree from the registry: one command per
// registered app.Command, nested by its dotted name ("query.compile" is
// `query compile`). Flags are generated from each command's args, so a command
// added to the registry shows up in the CLI automatically — no separate CLI
// registration to keep in sync.
func appCommands() []*cobra.Command {
	byName := map[string]*cobra.Command{}
	var top []*cobra.Command
	// A group's command is registered before its subcommands (fewer dots first),
	// so each subcommand finds its parent.
	cmds := slices.Clone(app.Commands)
	slices.SortStableFunc(cmds, func(a, b app.Command) int {
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

func buildCobraCommand(c app.Command) *cobra.Command {
	name := c.Name()[strings.LastIndex(c.Name(), ".")+1:]
	cmd := &cobra.Command{Use: name, Short: c.Short(), Long: c.Long(), Args: cobra.NoArgs}
	// Show flags in declaration order (else pflag sorts them alphabetically).
	cmd.Flags().SortFlags = false

	args := c.Args()
	flagArg := map[string]string{} // flag name -> the arg it sets
	var positional *app.Arg
	for i, a := range args {
		if a.Positional {
			positional = &args[i]
			// A positional has no flag to carry its help, so the long help does.
			long := c.Long()
			if long == "" {
				long = c.Short()
			}
			cmd.Long = long + "\n\nArguments:\n  " + a.Name + "  " + a.Usage
			if a.Type == app.ArgStringArray {
				cmd.Use += " [" + a.Name + "...]"
				cmd.Args = cobra.ArbitraryArgs
			} else {
				cmd.Use += " [" + a.Name + "]"
				cmd.Args = cobra.MaximumNArgs(1)
			}
			continue
		}
		// What Dispatch enforces, said once for every name of the arg.
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
		// pflag has no native aliases, so each alias is a flag of its own, folded
		// into the arg by Dispatch.
		addFlag(cmd, a, a.Flag(), a.Shorthand, usage)
		flagArg[a.Flag()] = a.Name
		for _, al := range a.Aliases {
			addFlag(cmd, a, al, "", "alias of --"+a.Flag())
			flagArg[al] = al
		}
	}

	cmd.RunE = func(runCmd *cobra.Command, posArgs []string) error {
		values := map[string]any{}
		// Only the flags the user set: an unset one is absent, so Dispatch applies
		// its default and sees which of a group are set.
		runCmd.Flags().Visit(func(f *pflag.Flag) {
			key, ok := flagArg[f.Name]
			if !ok {
				return
			}
			switch v := f.Value.(type) {
			case pflag.SliceValue:
				values[key] = v.GetSlice()
			default:
				if f.Value.Type() == "bool" {
					values[key] = f.Value.String() == "true"
				} else {
					values[key] = f.Value.String()
				}
			}
		})
		if positional != nil && len(posArgs) > 0 {
			if positional.Type == app.ArgStringArray {
				values[positional.Name] = posArgs
			} else {
				values[positional.Name] = posArgs[0]
			}
		}
		if err := applyStdin(args, values); err != nil {
			return err
		}
		return runCommand(runCmd.Context(), c, values)
	}
	return cmd
}

func addFlag(cmd *cobra.Command, a app.Arg, name, short, usage string) {
	switch a.Type {
	case app.ArgString:
		cmd.Flags().StringP(name, short, a.Default, usage)
	case app.ArgBool:
		cmd.Flags().BoolP(name, short, false, usage)
	case app.ArgStringArray:
		cmd.Flags().StringSliceP(name, short, nil, usage)
	}
}

// groupFlags are the CLI flags of a group's args.
func groupFlags(args []app.Arg, group string) []string {
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
func applyStdin(args []app.Arg, values map[string]any) error {
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

// runCommand routes a command to the warm server when one is running for this
// repo, otherwise runs it one-shot (spawning only the sidecars it needs).
func runCommand(ctx context.Context, c app.Command, args map[string]any) error {
	repoDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve repo dir: %w", err)
	}
	repo := repo.Resolve(repoDir)
	req := app.Request{Command: c.Name(), Args: args}

	if isServeRunning(repo) {
		body, contentType, err := callServe(repo, req)
		if err != nil {
			return err
		}
		return present(c.Name(), body, contentType)
	}

	return withRepo(ctx, func(ctx context.Context, h hostContext) error {
		clients, closeSidecars, err := startNeededSidecars(ctx, h, c, args)
		if err != nil {
			return err
		}
		defer closeSidecars()

		resp, err := app.Dispatch(ctx, h.commandContext(clients), req)
		if err != nil {
			return err
		}
		body, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		// resp is a {status, data} envelope we just marshalled, so it's JSON.
		return present(c.Name(), body, "application/json")
	})
}

// present shows a {status, data} response envelope: it renders just Data (the CLI
// stays clean), and maps a non-ok Status to a silent non-zero exit — /call
// callers get the full envelope instead. Non-JSON bodies pass through unchanged.
func present(commandName string, body []byte, contentType string) error {
	return presentTo(commandName, body, contentType, os.Stdout)
}

func presentTo(commandName string, body []byte, contentType string, out io.Writer) error {
	if !isJSONContentType(contentType) {
		_, err := out.Write(body)
		return err
	}
	var env struct {
		Status app.Status      `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	var err error
	if commandName == "search" {
		err = renderSearchResults(env.Data, out)
	} else {
		err = renderTo(env.Data, contentType, out)
	}
	if err != nil {
		return err
	}
	if env.Status != app.StatusOK {
		return &exitCodeError{code: 1}
	}
	return nil
}

// startNeededSidecars spawns just the sidecars the command declares it needs
// for these args, returning the clients and a single close func (LIFO).
func startNeededSidecars(ctx context.Context, h hostContext, c app.Command, args map[string]any) (app.Clients, func(), error) {
	need := c.Needs(args)
	var clients app.Clients
	var closers []func()
	closeAll := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}

	if need.Node {
		node := sidecar.NewAnfraNode(h.cfg)
		if err := node.Start(ctx); err != nil {
			closeAll()
			return app.Clients{}, nil, fmt.Errorf("start anfra-node sidecar: %w", err)
		}
		closers = append(closers, node.Close)
		clients.Node = node.Client()
	}
	if need.CanalQuery {
		canal := sidecar.NewCanalQuery(h.cfg)
		if err := canal.Start(ctx); err != nil {
			closeAll()
			return app.Clients{}, nil, fmt.Errorf("start canal-query sidecar: %w", err)
		}
		closers = append(closers, canal.Close)
		clients.CanalQuery = canal.Client()
	}
	return clients, closeAll, nil
}

// renderTo prints a command response. A JSON body (per its Content-Type) is
// converted to YAML for readability; anything else is written through unchanged.
func renderTo(body []byte, contentType string, out io.Writer) error {
	if !isJSONContentType(contentType) {
		_, err := out.Write(body)
		return err
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	_, err = out.Write(b)
	return err
}

func renderSearchResults(body []byte, out io.Writer) error {
	var data struct {
		Results []struct {
			DisplayName *string `json:"display_name"`
			Source      string  `json:"source"`
			Type        string  `json:"type"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("decode search results: %w", err)
	}
	for _, result := range data.Results {
		displayName := ""
		if result.DisplayName != nil {
			displayName = *result.DisplayName
		}
		if _, err := fmt.Fprintf(out, "%s | %s | %s\n", result.Source, result.Type, displayName); err != nil {
			return err
		}
	}
	return nil
}

func isJSONContentType(contentType string) bool {
	return strings.HasPrefix(strings.TrimSpace(contentType), "application/json")
}
