package telemetry

// Every attribute and span event anfra's spans can carry is declared here
// once, as the user's data, recorded only when data is, or as metadata, which
// is always recorded: anfra's own, and the keys the libraries it uses add. A
// key that isn't declared is never exported (Filter), and the test that
// records real runs fails on it, so a new one is classified before it ships
// (docs/designs/telemetry.md, data in spans).
//
// Metadata is recorded whatever the user's settings, so declaring a key meta
// is the one mistake here that leaks: declare a key meta only when no value it
// can hold comes from the user's data, repo, warehouse, org or machine. When
// unsure, declare it data.

// Key is a declared attribute or span event.
type Key struct{ name string }

// String is the key's name, as it is recorded.
func (k Key) String() string { return k.name }

// IsData reports whether the key can hold the user's data, so is recorded only
// when data is.
func (k Key) IsData() bool { return registry[k.name].data }

// Is is the key with a value, to set on a span (apptracing.SetAttributes).
func (k Key) Is(v any) KeyValue { return KeyValue{Key: k, Value: v} }

// KeyValue is a declared key and its value.
type KeyValue struct {
	Key   Key
	Value any
}

// keyDef is how a declared key is treated.
type keyDef struct {
	data    bool // the user's data: recorded only when data is
	leftOut bool // never recorded, whatever the settings
	event   bool // a span event's name, not an attribute's
}

var registry = map[string]keyDef{}

func declare(name string, d keyDef) Key {
	if _, dup := registry[name]; dup {
		panic("telemetry: key " + name + " declared twice")
	}
	registry[name] = d
	return Key{name: name}
}

// meta declares a key that is always recorded: see the rule above.
func meta(name string) Key             { return declare(name, keyDef{}) }
func data(name string) Key             { return declare(name, keyDef{data: true}) }
func event(name string, data bool) Key { return declare(name, keyDef{data: data, event: true}) }
func leftOut(name string) Key          { return declare(name, keyDef{leftOut: true}) }
func declareAll(f func(string) Key, names ...string) {
	for _, n := range names {
		f(n)
	}
}

// anfra's own attributes.
var (
	Op                 = meta("anfra.op")
	ErrorType          = meta("error.type")
	RepoID             = meta("anfra.repo.id")
	QueryID            = meta("anfra.query.id")
	QueryKind          = meta("anfra.query.kind")
	Dataset            = data("anfra.dataset")
	DataSource         = data("anfra.data_source")
	DBSystem           = meta("db.system.name")
	WarehouseQueryID   = meta("anfra.warehouse.query_id")
	ReturnedRows       = meta("db.response.returned_rows")
	BytesScanned       = meta("anfra.query.bytes_scanned")
	QueryText          = data("db.query.text")
	QueryTextTruncated = meta("anfra.query.text_truncated")
	Target             = data("anfra.target")
	ProblemCount       = meta("anfra.problem_count")
	CommandStdin       = meta("anfra.command.stdin")
	RequestID          = meta("h.request.id")
	HTTPRoute          = meta("http.route")
	ConversationID     = meta("gen_ai.conversation.id")
	// SessionID is ConversationID again, on spans only, under OpenTelemetry's
	// general name for a session: Langfuse and Claude Code's own telemetry group
	// sessions by it alone. A caller's baggage never sets it, since it is declared.
	SessionID = meta("session.id")
	AgentName = meta("gen_ai.agent.name")
	// DroppedKeys names the undeclared keys Filter left out of the span: key
	// names, which code chose, never their values.
	DroppedKeys = meta("anfra.telemetry.dropped_keys")
)

// A CLI command's span, and the command each span is part of.
var (
	// Command marks a call's own span, a CLI command's root, which is not
	// always its trace's root: it goes under the caller's span when the caller
	// passes its trace.
	Command = meta("anfra.command")
	// ProcessExitCode is the code a CLI command exits with, on its command
	// span (OpenTelemetry's CLI conventions), so a caller's trace shows which
	// of its calls failed.
	ProcessExitCode = meta("process.exit.code")
	// CommandID is the command a span is part of: its command span's id, on
	// every span the command's process records, so a command's spans are found
	// together when its trace holds other commands too.
	CommandID = meta("anfra.command.id")
)

// An MCP request's span, as OpenTelemetry's MCP conventions name it: the
// method, and for a tool call, the tool, an op's name or a discovery tool's.
var (
	MCPMethod          = meta("mcp.method.name")
	MCPSessionID       = meta("mcp.session.id")
	MCPProtocolVersion = meta("mcp.protocol.version")
	ToolName           = meta("gen_ai.tool.name")
	// GenAIOperation is execute_tool on a tool call's span, and unset on any
	// other, as the conventions say.
	GenAIOperation = meta("gen_ai.operation.name")
)

// PanicStatus is the status text of a span whose code panicked: like a public
// error code, it holds no data, so it is always recorded.
const PanicStatus = "panic"

// The payload events, and the attributes each carries: the payload, its whole
// size, and whether it was cut. The event is data, which covers its payload.
var (
	CommandInput     = event("anfra.command.input", true)
	OpInput          = event("anfra.op.input", true)
	OpOutput         = event("anfra.op.output", true)
	Payload          = meta("anfra.payload")
	PayloadSize      = meta("anfra.payload.size")
	PayloadTruncated = meta("anfra.payload.truncated")
)

// An error's event, as OpenTelemetry records it, is data: none when data is
// off. The span's error.type, the code, is always recorded.
var (
	Exception           = event("exception", true)
	ExceptionMessage    = data("exception.message")
	ExceptionStacktrace = data("exception.stacktrace")
	ExceptionType       = meta("exception.type")
	ExceptionEscaped    = meta("exception.escaped")
	// ErrorMessage is the semantic conventions' error.message, which a
	// library may set in place of an exception event: the same text, so data
	// too.
	ErrorMessage = data("error.message")
)

// A query's steps, as span events at the time each started: compiling, and
// canal-query's.
var QuerySteps = []Key{
	event("compiling_query", false), event("connecting_to_dw", false), event("executing_query", false),
	event("fetching_query_result", false), event("retrying_query", false),
}

// A step's parameters (apptracing.Start), h.step.<key>.
var (
	StepOrigin  = data("h.step.origin")
	StepOrg     = data("h.step.org")
	StepSidecar = meta("h.step.sidecar")
)

// The keys the libraries add: otelhttp's, on the CLI's org and sidecar calls
// and on serve's requests, and the resource's.
func init() {
	declareAll(meta,
		"http.request.method", "http.request.method_original", "http.response.status_code",
		"http.request.body.size", "http.response.body.size",
		"network.protocol.name", "network.protocol.version", "network.peer.port",
		"server.port", "url.scheme", "url.path",
		"user_agent.original",
	)
	// The org calls' URLs name the org, in their host too, and a query string
	// can hold what the caller sent.
	declareAll(data, "url.full", "url.query", "server.address")
	// The caller's address is a person's IP when serve serves others.
	declareAll(leftOut, "client.address", "network.peer.address")
}

// classify is how the key name is treated, and whether it is declared.
// A key anfra doesn't declare is a caller's when a host named its caller by
// it: metadata, recorded whatever the settings (callerKey).
func classify(name string) (keyDef, bool) {
	if d, ok := registry[name]; ok {
		return d, true
	}
	return keyDef{}, callerKey(name)
}

// Declared reports whether name is a declared attribute or event: the
// registry test checks every key a real run records.
func Declared(name string) bool {
	_, ok := registry[name]
	return ok
}
