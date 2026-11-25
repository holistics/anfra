package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/internal/agentsession"
	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/command/task"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/repoenv"
	"github.com/holistics/anfra/internal/telemetry"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/apptracing"
	"github.com/holistics/anfra/shared/logkit"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// tracerName names the tracer of the CLI's own spans: a command's root. The
// tracer is looked up when a span starts, so it is the provider installed then.
const tracerName = "github.com/holistics/anfra/cmd/anfra"

// untraced are the commands that get no root span: one that runs until it is
// stopped, whose span would end with the process, if ever. serve's requests
// are traces of their own, and so is each command a console runs. Neither is
// shell completion, which runs on every Tab.
var untraced = map[string]bool{
	"anfra serve":                              true,
	"anfra console":                            true,
	"anfra " + cobra.ShellCompRequestCmd:       true,
	"anfra " + cobra.ShellCompNoDescRequestCmd: true,
}

// traceCommands makes whichever command of root runs the root of its trace, a
// span named for its path (anfra query, anfra publish), core and CLI-only
// commands alike, so no command has to start one. It returns the function that
// ends that span with the run's error once the command is done: executed is
// the command cobra ran, or found and refused. A refusal before the command
// ran, of a flag or an argument, still gets its span, so a trace shows every
// call that failed.
//
// The repo the command runs in is named on that span and every span and log
// record under it, from the start (repoAttribution), so a CLI-only command's
// carry it too. The span records args, the command's arguments as given, when
// data is on (anfra.command.input).
func traceCommands(ctx context.Context, root *cobra.Command, args []string) (end func(executed *cobra.Command, err error)) {
	var span trace.Span
	start := func(ctx context.Context, cmd *cobra.Command) context.Context {
		if untraced[cmd.CommandPath()] {
			return ctx
		}
		ctx = caller.WithSettings(ctx, traceData.Settings())
		ctx = caller.WithAllowedBaggage(callerTrace(ctx, os.Getenv), traceData.Settings())
		if dir, err := os.Getwd(); err == nil {
			ctx = caller.WithAttributes(ctx, repoAttribution(repo.Resolve(dir)))
		}
		named := attribution.Fields{}
		maps.Copy(named, sessionAttribution(os.Getenv))
		maps.Copy(named, taskAttribution(cmd))
		// The session and the task go in the command's baggage too, for the calls
		// it makes, so a command forwarded to anfra serve carries both there.
		ctx = caller.WithCallerBaggage(caller.WithAttributes(ctx, named), named, traceData.Settings())
		ctx, span = otel.Tracer(tracerName).Start(ctx, cmd.CommandPath()) //nolint:spancheck // ended by end, once the command returns
		apptracing.SetAttributes(ctx, caller.Command.Is(true))
		ctx = caller.WithAttributes(ctx, map[string]string{caller.CommandID.String(): span.SpanContext().SpanID().String()})
		apptracing.RecordPayload(ctx, caller.CommandInput, args)
		return ctx
	}
	pre := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error { //nolint:contextcheck // the command's own context, which cobra passes as cmd.Context()
		cmd.SetContext(start(cmd.Context(), cmd))
		if err := checkTaskFlag(cmd); err != nil {
			return err
		}
		if pre == nil {
			return nil
		}
		return pre(cmd, args)
	}
	return func(executed *cobra.Command, err error) {
		if span == nil && executed != nil && err != nil {
			start(ctx, executed)
		}
		if span == nil {
			return
		}
		defer span.End()
		spanCtx := trace.ContextWithSpan(ctx, span)
		code := exitCode(err)
		apptracing.SetAttributes(spanCtx, caller.ProcessExitCode.Is(code))
		if code == 0 {
			return
		}
		// OpenTelemetry's CLI spans: any exit but 0 is an error, with an error
		// status and error.type, whoever's fault it was. error.type is the
		// error's code; for an answer that says it failed, such as validate's
		// problems, which has none, the exit code. An op's span inside keeps the
		// server's rule: an error status only when the failure is anfra's.
		if _, rendered := errors.AsType[*exitCodeError](err); rendered {
			apptracing.SetAttributes(spanCtx, caller.ErrorType.Is(strconv.Itoa(code)))
			span.SetStatus(otelcodes.Error, fmt.Sprintf("exit code %d", code))
			return
		}
		// The code is the one the command answered, a forwarded command's the
		// server's, so the span says whose fault it was. Its message is
		// recorded as the data settings say, whoever's fault it was.
		resp := answered(err)
		apptracing.SetAttributes(spanCtx, caller.ErrorType.Is(resp.Code))
		caller.RecordError(span, traceData.Settings(), err, false)
		span.SetStatus(otelcodes.Error, resp.Code)
	}
}

// sessionAttribution names the session a command runs in, found in its
// environment through getenv, on every span and log record of the command, so
// all of one session's calls are found together (internal/agentsession): the
// coding agent's own, with the agent's name, or else the one its launcher set.
// Nothing outside either. anfra
// serve is untraced, so never named: it outlives the session it was started in.
func sessionAttribution(getenv func(string) string) attribution.Fields {
	s, ok := agentsession.FromEnv(getenv)
	if !ok {
		return nil
	}
	return sessionFields(s)
}

// addTaskFlag puts --task on every command: the task a command is run for
// (internal/command/task).
func addTaskFlag(root *cobra.Command) {
	root.PersistentFlags().String(task.Flag, "", task.FlagUsage)
}

// commandTask is the --task cmd is run with, "" for none.
func commandTask(cmd *cobra.Command) string {
	f := cmd.Flags().Lookup(task.Flag)
	if f == nil {
		return ""
	}
	return strings.TrimSpace(f.Value.String())
}

// checkTaskFlag refuses a --task that isn't a task's id, so a typo fails at
// once instead of recording the command in no task.
func checkTaskFlag(cmd *cobra.Command) error {
	if id := commandTask(cmd); id != "" && !task.ValidID(id) {
		return flagViolated("--"+task.Flag, task.InvalidIDMessage(id))
	}
	return nil
}

// taskAttribution names the task cmd is run for, from its --task, on every span
// and log record of the command, so a task's commands are found by its id,
// whichever agent of a session ran them, and whenever. A malformed id names
// none: checkTaskFlag refuses it.
func taskAttribution(cmd *cobra.Command) attribution.Fields {
	id := commandTask(cmd)
	if !task.ValidID(id) {
		return nil
	}
	return attribution.Fields{caller.TaskID.String(): id}
}

// mcpTask names the task an MCP tool call is made for, from its task
// argument, for apikit.MCP.CallArgs, and refuses an id that isn't a task's, as
// checkTaskFlag does.
func mcpTask(id string) (map[string]string, error) {
	if !task.ValidID(id) {
		msg := task.InvalidIDMessage(id)
		return nil, apperr.NewWith(apperr.ValidationFailed, msg,
			apperr.Violate(apperr.Violation{Field: task.Flag, Code: "invalid", Message: msg}))
	}
	return map[string]string{caller.TaskID.String(): id}, nil
}

// metaSession names the session of the agent that makes an MCP tool call, from
// the call's _meta (agentsession.FromMeta), for apikit.MCP.MetaCaller.
func metaSession(meta map[string]any) map[string]string {
	s, ok := agentsession.FromMeta(func(key string) string {
		v, _ := meta[key].(string)
		return v
	})
	if !ok {
		return nil
	}
	return sessionFields(s)
}

// sessionFields are s as a caller's fields: its id, and its agent's name when
// a known agent gave it.
func sessionFields(s agentsession.Session) attribution.Fields {
	fields := attribution.Fields{caller.ConversationID.String(): s.ID}
	if s.Agent != "" {
		fields[caller.AgentName.String()] = s.Agent
	}
	return fields
}

// callerTrace is ctx under the trace its caller passed in the environment, as
// TRACEPARENT and TRACESTATE (OpenTelemetry's environment carriers), such as
// the span of a coding agent's tool call: the command's span then goes under
// it. Read once, at the start.
func callerTrace(ctx context.Context, getenv func(string) string) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, envCarrier{getenv: getenv})
}

// envCarrier reads a trace from environment variables: a propagator's keys,
// uppercased. It never writes one: anfra changes no process-wide state for a
// command, and passes its trace on per call, in each request's headers.
type envCarrier struct {
	getenv func(string) string
}

func (c envCarrier) Get(key string) string { return c.getenv(strings.ToUpper(key)) }

func (envCarrier) Set(string, string) {}

func (envCarrier) Keys() []string { return []string{"TRACEPARENT", "TRACESTATE", "BAGGAGE"} }

// flushTimeout bounds how long exiting waits on the last spans: a collector that
// is down must not hold up a command that is done.
const flushTimeout = 2 * time.Second

// startTelemetry installs tracing, exported if the environment asks for it, and returns
// the function that flushes it. Telemetry never stops a command: a setup that
// fails is reported and the command runs untraced.
func startTelemetry(ctx context.Context) (flush func()) {
	shutdown, err := telemetry.Setup(ctx, "anfra", meta.Version, traceData.Settings())
	if err != nil {
		printWarning(os.Stderr, "telemetry_off", fmt.Sprintf("Telemetry is off: %v", err))
		return func() {}
	}
	if w := telemetry.ProtocolWarning(); w != "" && telemetry.Installed().URL != "" {
		printWarning(os.Stderr, "tracing_over_http", w)
	}
	for _, w := range telemetry.GuardWarnings(telemetry.Installed(), traceData) {
		printWarning(os.Stderr, w.Code, w.Message)
	}
	return func() {
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
		defer cancel()
		_ = shutdown(flushCtx)
	}
}

// failureEvery is how often a failing export is logged: once for a command,
// and once in a while for a server, not on every batch.
const failureEvery = 10 * time.Minute

var lastFailure struct {
	sync.Mutex
	at time.Time
}

// reportTelemetryFailure logs an export's failure, unless one was logged less
// than failureEvery ago, saying what to do about it.
func reportTelemetryFailure(lg *slog.Logger, err error) {
	lastFailure.Lock()
	if !lastFailure.at.IsZero() && time.Since(lastFailure.at) < failureEvery {
		lastFailure.Unlock()
		return
	}
	lastFailure.at = time.Now()
	lastFailure.Unlock()

	// Installed, not Current: a server reports where it sends, as it started.
	dest := telemetry.Installed()
	hint := "Check that the telemetry stack or collector at " + dest.Redacted() + " is running, or unset " + dest.From + " to stop sending traces."
	lg.Warn("Telemetry failed", logkit.EventKey, eventTelemetryFailed, "error", err, "endpoint", dest.Redacted(), "hint", hint)
}

// telemetryInfo is where this process sends its traces, for status:
// as it started, which for a server can differ from what a new command would
// pick up.
func telemetryInfo() *command.TelemetryInfo {
	dest := telemetry.Installed()
	data, limit := dataInfo()
	bag := baggageInfo()
	if dest.URL == "" {
		// From is set only when OTEL_SDK_DISABLED turned telemetry off, which
		// would otherwise look the same as no endpoint at all.
		return &command.TelemetryInfo{State: command.TelemetryOff, SetBy: dest.From, Data: data, PayloadLimit: limit, Baggage: bag}
	}
	state := command.TelemetryOn
	if !dest.Answering(300 * time.Millisecond) {
		state = command.TelemetryFailing
	}
	info := &command.TelemetryInfo{State: state, Endpoint: dest.Redacted(), SetBy: dest.From, Data: data, PayloadLimit: limit, Baggage: bag}
	info.Hints = dest.StatusHints(traceData)
	return info
}

// traceData is what of the user's data this process's spans record, as the
// environment and the repo's .env set it, resolved once at start (main), after
// envflag.Validate has refused a value it doesn't take.
var traceData caller.Resolution

// resolveTraceData resolves traceData. Validate has checked every value, so a
// failure here leaves data off, the safe value.
func resolveTraceData() {
	r, err := caller.Resolve(os.Getenv)
	if err != nil {
		r = caller.Resolution{PayloadLimit: caller.Resolved[int]{Value: caller.DefaultPayloadLimit}}
	}
	traceData = r
}

// dataInfo is traceData as status shows it.
func dataInfo() (*command.TelemetryData, *command.TelemetryLimit) {
	data := &command.TelemetryData{On: traceData.Data.Value, SetBy: setBy(traceData.Data.From)}
	limit := &command.TelemetryLimit{Value: traceData.PayloadLimit.Value, SetBy: setBy(traceData.PayloadLimit.From)}
	return data, limit
}

// baggageInfo is the baggage traceData lets in, as status shows it.
func baggageInfo() *command.TelemetryBaggage {
	include := slices.Clone(traceData.Settings().BaggageInclude())
	if include == nil {
		include = []string{}
	}
	return &command.TelemetryBaggage{Include: include, SetBy: setBy(traceData.BaggageInclude.From)}
}

// setBy is the variable that set a setting as status names it: "default" for
// none, and "in .env" after it when the repo's file set it.
func setBy(name string) string {
	if name == "" {
		return "default"
	}
	return repoenv.Describe(name)
}

// traceSidecar traces each call to a sidecar, and carries the trace into it.
// Its span is sampled only when telemetry exports, but its ids always go along,
// so the sidecar's log records join this one's.
func traceSidecar(sidecar string, rt http.RoundTripper) http.RoundTripper {
	return otelhttp.NewTransport(rt, otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return sidecar + " " + r.Method + " " + r.URL.Path
	}))
}
