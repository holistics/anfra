package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/repoenv"
	"github.com/holistics/anfra/internal/telemetry"
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
		ctx, span = otel.Tracer(tracerName).Start(ctx, cmd.CommandPath()) //nolint:spancheck // ended by end, once the command returns
		apptracing.RecordPayload(ctx, caller.CommandInput, args)
		if dir, err := os.Getwd(); err == nil {
			ctx = caller.WithAttributes(ctx, repoAttribution(repo.Resolve(dir)))
		}
		return ctx
	}
	pre := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error { //nolint:contextcheck // the command's own context, which cobra passes as cmd.Context()
		cmd.SetContext(start(cmd.Context(), cmd))
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
		if _, rendered := errors.AsType[*exitCodeError](err); err == nil || rendered {
			return // an answer that says it failed is already the command's, not a failure of it
		}
		// As apptracing marks a step: error.type always, an error status only
		// when the failure is anfra's, not the caller's.
		apptracing.MarkFailed(span, traceData.Settings(), err)
	}
}

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
	if dest.URL == "" {
		// From is set only when OTEL_SDK_DISABLED turned telemetry off, which
		// would otherwise look the same as no endpoint at all.
		return &command.TelemetryInfo{State: command.TelemetryOff, SetBy: dest.From, Data: data, PayloadLimit: limit}
	}
	state := command.TelemetryOn
	if !dest.Answering(300 * time.Millisecond) {
		state = command.TelemetryFailing
	}
	info := &command.TelemetryInfo{State: state, Endpoint: dest.Redacted(), SetBy: dest.From, Data: data, PayloadLimit: limit}
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
