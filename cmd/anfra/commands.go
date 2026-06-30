package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/anfra-ai/anfra/internal/app"
	"github.com/anfra-ai/anfra/internal/project"
	"github.com/anfra-ai/anfra/internal/sidecar"
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
	cmd := &cobra.Command{Use: c.Name, Short: c.Short}
	for _, a := range c.Args {
		switch a.Type {
		case app.ArgString:
			strVals[a.Name] = cmd.Flags().String(a.Name, "", a.Usage)
		case app.ArgBool:
			boolVals[a.Name] = cmd.Flags().Bool(a.Name, false, a.Usage)
		}
	}
	cmd.RunE = func(_ *cobra.Command, _ []string) error {
		args := map[string]any{}
		for k, v := range strVals {
			args[k] = *v
		}
		for k, v := range boolVals {
			args[k] = *v
		}
		if err := applyStdin(c, args); err != nil {
			return err
		}
		return runCommand(c, args)
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
// project, otherwise runs it one-shot (spawning only the sidecars it needs).
func runCommand(c app.Command, args map[string]any) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve project dir: %w", err)
	}
	proj := project.Resolve(projectDir)
	req := app.Request{Command: c.Name, Args: args}

	if isServeRunning(proj) {
		body, err := callServe(proj, req)
		if err != nil {
			return err
		}
		return render(c.Name, body)
	}

	return withProject(func(h hostContext) error {
		clients, closeSidecars, err := startNeededSidecars(h, c, args)
		if err != nil {
			return err
		}
		defer closeSidecars()

		res, err := app.Dispatch(h.ctx, clients, h.proj, req)
		if err != nil {
			return err
		}
		body, err := json.Marshal(res)
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		return render(c.Name, body)
	})
}

// startNeededSidecars spawns just the sidecars the command declares it needs
// for these args, returning the clients and a single close func (LIFO).
func startNeededSidecars(h hostContext, c app.Command, args map[string]any) (app.Clients, func(), error) {
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
		if err := node.Start(h.ctx); err != nil {
			closeAll()
			return app.Clients{}, nil, fmt.Errorf("start anfra-node sidecar: %w", err)
		}
		closers = append(closers, node.Close)
		clients.Node = node.Client()
	}
	if need.Canal {
		canal := sidecar.NewCanalQuery(h.cfg)
		if err := canal.Start(h.ctx); err != nil {
			closeAll()
			return app.Clients{}, nil, fmt.Errorf("start canal-query sidecar: %w", err)
		}
		closers = append(closers, canal.Close)
		clients.Canal = canal.Client()
	}
	return clients, closeAll, nil
}

// render prints a command result. The body is the raw /call JSON (server path)
// or the JSON-marshalled Dispatch result (one-shot) — same shape either way.
// query gets pipe-friendly presentation; anything else prints as YAML.
func render(command string, body []byte) error {
	if command == "query" {
		var q app.QueryResult
		if err := json.Unmarshal(body, &q); err != nil {
			return fmt.Errorf("decode query result: %w", err)
		}
		if q.Result == nil { // --generate: just the SQL, pipe-friendly
			fmt.Println(q.SQL)
			return nil
		}
		var out queryOutput
		out.SQL = q.SQL
		out.Result.Fields = q.Result.Fields
		out.Result.Records = q.Result.Records
		return printYAML(out)
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return fmt.Errorf("decode result: %w", err)
	}
	return printYAML(v)
}

// queryOutput is the YAML shape printed for an executed query.
type queryOutput struct {
	SQL    string `yaml:"sql"`
	Result struct {
		Fields  []string `yaml:"fields"`
		Records [][]any  `yaml:"records"`
	} `yaml:"result"`
}

func printYAML(v any) error {
	b, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Print(string(b))
	return nil
}
