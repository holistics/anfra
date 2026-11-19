package telemetry

// Every attribute and span event anfra's spans can carry is declared here
// once, with its category, or as metadata (no category), which is always
// recorded: anfra's own, and the keys the libraries it uses add. A key that
// isn't declared is never exported (Filter), and the test that records real
// runs fails on it, so a new one is classified before it ships
// (docs/designs/telemetry.md, how to classify a new attribute's category).
//
// Metadata is recorded whatever the user's settings, so declaring a key meta
// is the one mistake here that leaks: declare a key meta only when no value it
// can hold comes from the user's data, repo, warehouse, org or machine. When
// unsure, give it a category.

// Key is a declared attribute or span event.
type Key struct{ name string }

// String is the key's name, as it is recorded.
func (k Key) String() string { return k.name }

// Category is the key's category; "" for metadata.
func (k Key) Category() Category { return registry[k.name].category }

// Is is the key with a value, to set on a span (apptracing.SetAttributes).
func (k Key) Is(v any) KeyValue { return KeyValue{Key: k, Value: v} }

// KeyValue is a declared key and its value.
type KeyValue struct {
	Key   Key
	Value any
}

// keyDef is how a declared key is treated.
type keyDef struct {
	category Category
	leftOut  bool // never recorded, whatever the settings
	event    bool // a span event's name, not an attribute's
	// mask is the value as its category's masked level records it, for the
	// warehouse system (db.system.name) the span ran on. nil when the key has
	// no masked form, so masked leaves it out.
	mask func(value, system string) string
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
func meta(name string) Key              { return declare(name, keyDef{}) }
func data(name string, c Category) Key  { return declare(name, keyDef{category: c}) }
func event(name string, c Category) Key { return declare(name, keyDef{category: c, event: true}) }
func leftOut(name string) Key           { return declare(name, keyDef{leftOut: true}) }
func masked(name string, c Category, mask func(value, system string) string) Key {
	return declare(name, keyDef{category: c, mask: mask})
}

// maskError is MaskError as a key's mask: an error's message is masked the
// same on every warehouse.
func maskError(msg, _ string) string { return MaskError(msg) }
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
	Dataset            = data("anfra.dataset", Inputs)
	DataSource         = data("anfra.data_source", Inputs)
	DBSystem           = meta("db.system.name")
	WarehouseQueryID   = meta("anfra.warehouse.query_id")
	ReturnedRows       = meta("db.response.returned_rows")
	BytesScanned       = meta("anfra.query.bytes_scanned")
	QueryText          = masked("db.query.text", SQL, MaskSQL)
	QueryTextTruncated = meta("anfra.query.text_truncated")
	Target             = data("anfra.target", Inputs)
	ProblemCount       = meta("anfra.problem_count")
	CommandStdin       = meta("anfra.command.stdin")
	RequestID          = meta("h.request.id")
	HTTPRoute          = meta("http.route")
	SessionID          = meta("session.id")
	ConversationID     = meta("gen_ai.conversation.id")
	// DroppedKeys names the undeclared keys Filter left out of the span: key
	// names, which code chose, never their values.
	DroppedKeys = meta("anfra.telemetry.dropped_keys")
)

// PanicStatus is the status text of a span whose code panicked: like a public
// error code, it holds no data, so it is always recorded.
const PanicStatus = "panic"

// The payload events, and the attributes each carries: the payload, its whole
// size, and whether it was cut. The event's category covers its payload.
var (
	CommandInput     = event("anfra.command.input", Inputs)
	OpInput          = event("anfra.op.input", Inputs)
	OpOutput         = event("anfra.op.output", Outputs)
	Payload          = meta("anfra.payload")
	PayloadSize      = meta("anfra.payload.size")
	PayloadTruncated = meta("anfra.payload.truncated")
)

// An error's event, as OpenTelemetry records it, is Errors: none when errors
// are off, and a masked error keeps its message, masked, without its stack.
// The span's error.type, the code, is recorded whatever errors say.
var (
	Exception           = event("exception", Errors)
	ExceptionMessage    = masked("exception.message", Errors, maskError)
	ExceptionStacktrace = data("exception.stacktrace", Errors)
	ExceptionType       = meta("exception.type")
	ExceptionEscaped    = meta("exception.escaped")
	// ErrorMessage is the semantic conventions' error.message, which a
	// library may set in place of an exception event: the same text, so the
	// same category and masking.
	ErrorMessage = masked("error.message", Errors, maskError)
)

// A query's steps, as span events at the time each started: compiling, and
// canal-query's.
var QuerySteps = []Key{
	event("compiling_query", ""), event("connecting_to_dw", ""), event("executing_query", ""),
	event("fetching_query_result", ""), event("retrying_query", ""),
}

// A step's parameters (apptracing.Start), h.step.<key>.
var (
	StepOrigin  = data("h.step.origin", Inputs)
	StepOrg     = data("h.step.org", Inputs)
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
	declareAll(func(n string) Key { return data(n, Inputs) }, "url.full", "url.query", "server.address")
	// The caller's address is a person's IP when serve serves others.
	declareAll(leftOut, "client.address", "network.peer.address")
}

// classify is how the key name is treated, and whether it is declared.
func classify(name string) (keyDef, bool) {
	d, ok := registry[name]
	return d, ok
}

// Declared reports whether name is a declared attribute or event: the
// registry test checks every key a real run records.
func Declared(name string) bool {
	_, ok := registry[name]
	return ok
}
