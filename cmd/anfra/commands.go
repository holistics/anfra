package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// appCommands builds one cobra command per registered app.Command. Flags are
// generated from each command's Args, so a command added to the registry shows
// up in the CLI automatically — no separate CLI registration to keep in sync.
func appCommands() []*cobra.Command {
	cmds := make([]*cobra.Command, 0, len(app.Commands))
	for _, c := range app.Commands {
		cmds = append(cmds, buildCobraCommand(c))
	}
	return cmds
}

func buildCobraCommand(c app.Command) *cobra.Command {
	strVals := map[string]*string{}
	boolVals := map[string]*bool{}
	cmd := &cobra.Command{Use: c.Name, Short: c.Short, Args: cobra.NoArgs}
	if c.Positional != nil {
		cmd.Use = fmt.Sprintf("%s [%s...]", c.Name, c.Positional.Name)
		cmd.Args = cobra.ArbitraryArgs
	}
	// Show flags in declaration order (else pflag sorts alphabetically, scattering
	// an alias like --validate away from its canonical --generate).
	cmd.Flags().SortFlags = false
	for _, a := range c.Args {
		// pflag has no native flag aliases, so register the canonical name and each
		// alias as their own flags (all set the same arg, folded by NormalizeReqArgs).
		// Each flag keeps its own usage (aliases may override) and cross-references
		// the others.
		names := []string{a.Name}
		usageOf := map[string]string{a.Name: a.Usage}
		for _, al := range a.Aliases {
			names = append(names, al.Name)
			if usageOf[al.Name] = al.Usage; al.Usage == "" {
				usageOf[al.Name] = a.Usage
			}
		}
		for _, name := range names {
			usage := usageOf[name]
			var others []string
			for _, n := range names {
				if n != name {
					others = append(others, "--"+n)
				}
			}
			if len(others) > 0 {
				usage += " (alias: " + strings.Join(others, ", ") + ")"
			}
			// The shorthand attaches to the canonical name only (aliases stay long-only,
			// and pflag allows one shorthand per flag).
			short := ""
			if name == a.Name {
				short = a.Shorthand
			}
			switch a.Type {
			case app.ArgString:
				strVals[name] = cmd.Flags().StringP(name, short, "", usage)
			case app.ArgBool:
				boolVals[name] = cmd.Flags().BoolP(name, short, false, usage)
			}
		}
	}
	cmd.RunE = func(runCmd *cobra.Command, posArgs []string) error {
		args := map[string]any{}
		for k, v := range strVals {
			args[k] = *v
		}
		for k, v := range boolVals {
			args[k] = *v
		}
		if c.Positional != nil {
			args[c.Positional.Name] = posArgs
		}
		// Fold aliases into canonical names here too, so Needs (evaluated before
		// Dispatch in the one-shot path) sees the right values.
		app.NormalizeReqArgs(c, args)
		if err := applyStdin(c, args); err != nil {
			return err
		}
		return runCommand(runCmd.Context(), c, args)
	}
	return cmd
}

// applyStdin fills c.StdinArg from piped stdin when its flag was left empty, so
// e.g. `cat query.aql | anfra query --dataset x` works.
func applyStdin(c app.Command, args map[string]any) error {
	if c.StdinArg == "" {
		return nil
	}
	if s, _ := args[c.StdinArg].(string); strings.TrimSpace(s) != "" {
		return nil
	}
	stat, _ := os.Stdin.Stat()
	if stat != nil && (stat.Mode()&os.ModeCharDevice) != 0 {
		return nil // a terminal — nothing piped
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read %s from stdin: %w", c.StdinArg, err)
	}
	if v := strings.TrimSpace(string(data)); v != "" {
		args[c.StdinArg] = v
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
	req := app.Request{Command: c.Name, Args: args}

	if isServeRunning(repo) {
		body, contentType, err := callServe(repo, req)
		if err != nil {
			return err
		}
		return present(c.Name, body, contentType)
	}

	return withRepo(ctx, func(ctx context.Context, h hostContext) error {
		clients, closeSidecars, err := startNeededSidecars(ctx, h, c, args)
		if err != nil {
			return err
		}
		defer closeSidecars()

		resp, err := app.Dispatch(ctx, clients, h.repo, req)
		if err != nil {
			return err
		}
		body, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		// resp is a {status, data} envelope we just marshalled, so it's JSON.
		return present(c.Name, body, "application/json")
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
	var need app.Sidecars
	if c.Needs != nil {
		need = c.Needs(args)
	}
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
