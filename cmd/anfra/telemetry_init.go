package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/home"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/telemetry"
	"github.com/holistics/anfra/internal/telemetry/stack"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// newExperimentalCmd groups the commands that may change, or go, in a minor
// release, and that a release's changelog notes when they do. Leaving the group
// is a rename: the old name then stays, hidden, for a release.
func newExperimentalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "experimental",
		Short: "Commands that may change, or go, in a minor release",
		Long: "Commands that may change, or go, in a minor release, with their settings (ANFRA_EXPERIMENTAL_*).\n" +
			"The changelog says when one does.",
	}
	cmd.AddCommand(newTelemetryCmd())
	return cmd
}

// newTelemetryCmd groups the commands for recording what anfra does in the
// user's own endpoint (docs/designs/telemetry.md). Its own commands are
// CLI-only. The task ops (internal/command/task) join them, under task
// (appCommands).
func newTelemetryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Record what each anfra command does, to your own OpenTelemetry endpoint. Nothing is sent to Holistics.",
		Long: "Record what each anfra command does, to your own OpenTelemetry endpoint, so you or your agent can look back at it:\n" +
			"each command's steps, how long each took, and each query's row count. Nothing is sent to Holistics.\n\n" +
			"To turn it on, in the repo's folder:\n" +
			"  1. anfra experimental telemetry init  writes the repo's local telemetry stack's compose file\n" +
			"  2. docker compose -f <file> up -d     starts the stack: the file init's hint names\n" +
			"  3. OTEL_EXPORTER_OTLP_ENDPOINT=<url>  in the .env at the repo's root: the line init's hint gives\n" +
			"To record to a collector you already run, skip 1 and 2, and give its endpoint.\n\n" +
			"A trace holds your data: what a command asked for, the SQL it ran, what came back (never a query's rows),\n" +
			"and an error's message and stack. " + caller.DataVar + "=0, in the environment or the .env, which\n" +
			"the environment wins over, leaves it out. anfra warns when it is on by default and the endpoint is on\n" +
			"another machine.\n\n" +
			"`anfra status` shows where anfra records, whether the endpoint answers, and each setting with what set it.",
	}
	cmd.AddCommand(newTelemetryInitCmd())
	return cmd
}

// telemetryInitResult is what init wrote, and what is left to do.
type telemetryInitResult struct {
	Message string          `json:"message" doc:"the outcome, in a sentence"`
	Files   []telemetryFile `json:"files" doc:"the files init writes"`
	Hints   []string        `json:"hints,omitempty" doc:"what to do next: start the local telemetry stack, point anfra at it, check it"`
}

// telemetryFile is a file init writes, and what it did to it.
type telemetryFile struct {
	Path   string `json:"path" doc:"the file, absolute"`
	Change string `json:"change" doc:"what init did to it" enum:"created,updated,unchanged"`
}

func newTelemetryInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write this repo's local telemetry stack's compose file, for Docker to run",
		Long: "Write the compose file of this repo's local telemetry stack to " + home.Shown(filepath.Join(home.Dir(), "repos", "<repo id>", "telemetry", stack.FileName)) + ".\n" +
			"The stack is an OpenTelemetry collector that receives anfra's traces, and ClickHouse, which keeps them for three\n" +
			"days. Each repo has its own, on ports of its own, so stacks of several repos run side by side. init starts\n" +
			"nothing, and changes no setting. Its hints say how to start the stack with Docker, and the line that points\n" +
			"the repo's commands at it, for the .env at the repo's root.\n\n" +
			"Run it again after upgrading anfra: it rewrites the compose file it wrote, for the new version, keeping its\n" +
			"ports. If a compose.yaml that anfra didn't write is there, init leaves it alone and fails.\n\n" +
			"Its result opens with message, the outcome; then files, each with whether init created, updated or\n" +
			"left it; and hints, last, what to do next.\n\n" +
			"`anfra experimental telemetry --help` says what a trace holds of your data, and how to set it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolve repo dir: %w", err)
			}
			if info, err := os.Stat(filepath.Join(cwd, repo.ConfigDirName)); err != nil || !info.IsDir() {
				return clierr.Precondition("This folder isn't an anfra repo, and each repo has its own telemetry stack. " +
					"Run `anfra experimental telemetry init` in the repo's folder, or `anfra init` first.")
			}
			r := repo.Resolve(cwd)
			dir := filepath.Join(r.DataDir, "telemetry")
			w, err := stack.Write(dir, stack.Spec{Version: meta.Version, Name: stack.Name(r.ID), Taken: otherStacksPorts(dir)})
			if errors.Is(err, stack.ErrNotOurs) {
				return clierr.Precondition("%s is a compose file that anfra didn't write, so init leaves it alone. "+
					"Move it elsewhere, then run `anfra experimental telemetry init` again.", home.Shown(w.Path))
			}
			if err != nil {
				return commandFailedWith(fmt.Sprintf("Can't write the compose file in %s: %v.", home.Shown(dir), err), nil)
			}
			return writeResult(cmd, telemetryInitResult{
				Message: initMessage(w.Change),
				Files:   []telemetryFile{{Path: w.Path, Change: string(w.Change)}},
				Hints:   initHints(cmd, r, w),
			})
		},
	}
	markPrettyOutput(cmd)
	resultIs[telemetryInitResult](cmd)
	return cmd
}

// otherStacksPorts are the ports of the other repos' stacks on this machine,
// which a new stack avoids even while they are stopped.
func otherStacksPorts(own string) []int {
	files, _ := filepath.Glob(filepath.Join(home.Dir(), "repos", "*", "telemetry", stack.FileName))
	var taken []int
	for _, f := range files {
		if filepath.Dir(f) == own {
			continue
		}
		if p, ok := stack.PortsIn(f); ok {
			taken = append(taken, p.List()...)
		}
	}
	return taken
}

func initMessage(c stack.Change) string {
	switch c {
	case stack.Updated:
		return "Updated this repo's local telemetry stack's compose file for this anfra version. Run `docker compose up -d` with it to upgrade the stack."
	case stack.Unchanged:
		return "This repo's local telemetry stack's compose file is already this anfra version's."
	default:
		return "Wrote this repo's local telemetry stack's compose file. Start the stack with Docker, then point the repo's commands at it."
	}
}

// initHints are the steps init leaves to the user: start the stack, point the
// repo at it unless something else already does, restart its server, check.
func initHints(cmd *cobra.Command, r repo.Local, w stack.Written) []string {
	shown := home.Shown(w.Path)
	endpoint := w.Ports.Endpoint()
	line := "OTEL_EXPORTER_OTLP_ENDPOINT=" + endpoint
	hints := []string{"Start the local telemetry stack: `docker compose -f " + shown + " up -d`. It needs Docker, and downloads its images the first time."}
	switch dest := telemetry.Current(); {
	case dest.Disabled():
		hints = append(hints, fmt.Sprintf("Then point the repo's commands at it: add `%s` to the .env at the repo's root, and unset %s, "+
			"which turns telemetry off whatever else is set.", line, dest.From))
	case dest.URL == "":
		hints = append(hints, fmt.Sprintf("Then point the repo's commands at it: add `%s` to the .env at the repo's root. "+
			"Every command in the repo reads it, an agent's included.", line))
	case dest.URL != endpoint:
		hints = append(hints, fmt.Sprintf("Traces go to %s now, set by %s. To record to this repo's stack instead, set it to %s.", dest.Redacted(), dest.From, endpoint))
	}
	if _, ok := findServer(cmd.Context(), r); ok {
		hints = append(hints, "`anfra serve` is running for this repo, so restart it once the endpoint is set, to record its commands.")
	}
	hints = append(hints, "To check it, run `anfra status`, which says where traces go, and whether telemetry is on or failing.")
	return hints
}
