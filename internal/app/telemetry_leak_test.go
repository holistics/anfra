package app

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	coltrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/holistics/anfra/internal/attribution"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/internal/telemetry"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// canary is the value the leak test puts in the user's data: in a filter, in
// the SQL sent, in a result, and in the warehouse's error. A trace that holds
// it holds the user's data.
const canary = "anfracanary7f3a"

// leakTestChild is set in the process TestNoDataLeavesInTraces runs itself in.
const leakTestChild = "ANFRA_LEAK_TEST_CHILD"

// What of the user's data reaches the collector, from anfra and from each
// sidecar it spawns, as each data setting says: none with everything off,
// none unmasked with the defaults. Raw, it does, which proves the test sees a
// leak when there is one. It looks for the canary in every string exported,
// whatever holds it (an attribute, an event, a status, a span's name), so a
// key no one classified is caught too, and so are the sidecars, which filter
// their own spans out of reach of anfra's Filter.
//
// The canary is a value, as a filter's or a literal is: an identifier, such
// as a table's name, is the query's shape, which masked SQL keeps.
//
// It needs the sidecars and the dev warehouse, as TestCommandsAgainstRealSidecars
// does, and is skipped without them.
func TestNoDataLeavesInTraces(t *testing.T) {
	if os.Getenv("ANFRA_NODE_BIN") == "" || os.Getenv("ANFRA_CANAL_BIN") == "" && os.Getenv("ANFRA_CANAL_QUERY_BIN") == "" {
		t.Skip("set ANFRA_NODE_BIN and ANFRA_CANAL_BIN to check what real sidecars export")
	}
	// Tracing is set up once per process, as in anfra, and apptracing binds to
	// the first provider installed: run in a process of its own, so neither
	// this test nor the package's others see the other's.
	if os.Getenv(leakTestChild) == "" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestNoDataLeavesInTraces$", "-test.v", "-test.count=1") //nolint:gosec // G204: this test's own binary
		cmd.Env = append(os.Environ(), leakTestChild+"=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		t.Logf("%s", out)
		return
	}
	fixture, err := filepath.Abs("testdata/repo")
	if err != nil {
		t.Fatal(err)
	}
	collector := newCollector(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// One provider for the process, as in anfra: apptracing binds its tracer
	// to the first. Each case's settings go with its calls (cc.TraceData) and
	// to the sidecars it starts (their H_OTEL_DATA_* variables).
	shutdown := setupTracing(ctx, t, collector.URL)
	defer func() { _ = shutdown(context.WithoutCancel(ctx)) }()

	all := func(l caller.Level) map[caller.Category]caller.Level {
		m := map[caller.Category]caller.Level{}
		for _, c := range caller.Categories {
			m[c.Name] = l
		}
		return m
	}
	for _, tc := range []struct {
		name   string
		levels map[caller.Category]caller.Level
		leaks  bool
	}{
		{"everything off", all(caller.Off), false},
		{"the defaults", nil, false},
		{"everything raw", all(caller.Raw), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			settings := caller.NewSettings(tc.levels, 0)
			for _, kv := range caller.SidecarEnv(settings) {
				name, value, _ := strings.Cut(kv, "=")
				t.Setenv(name, value)
			}
			collector.reset()
			runWithCanary(ctx, t, fixture, settings)
			services, leaks := collector.look()
			t.Logf("services exported: %v", services)
			for _, s := range []string{"anfra", "anfra_node"} {
				if !slices.Contains(services, s) {
					t.Errorf("no span of %s was exported, so its spans went unchecked (got %v)", s, services)
				}
			}
			// The anfra-canal release manifest.yml pins doesn't export over HTTP
			// yet (docs/designs/telemetry.md, implementation). Its spans are
			// checked as soon as it does: the canary is looked for in every
			// service's.
			if !slices.Contains(services, canalquery.Name) {
				t.Logf("no span of %s was exported: its release doesn't export over HTTP yet", canalquery.Name)
			}
			if leaked := len(leaks) > 0; leaked != tc.leaks {
				t.Errorf("the canary in the traces: %v, want %v: %s", leaked, tc.leaks, strings.Join(leaks, "\n"))
			}
			// With everything raw, each stable span anfra's ops record holds
			// each of its stable attributes, on one of its spans at least: the
			// failed query's error.type, the AQL query's dataset.
			if tc.leaks {
				for _, m := range collector.missingStable() {
					t.Errorf("a stable span lost a stable attribute: %s", m)
				}
			}
		})
	}
}

// setupTracing installs anfra's tracing, exporting to endpoint, and the
// variables that point the sidecars it spawns there too, restoring the
// environment after the test.
func setupTracing(ctx context.Context, t *testing.T, endpoint string) func(context.Context) error {
	t.Helper()
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "OTEL_") || strings.HasPrefix(name, "H_OTEL_") || strings.HasPrefix(name, canalquery.EnvPrefix) {
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
	// What Setup sets, for the sidecars to inherit: unset, so restored after.
	for _, name := range []string{"H_OTEL_ENABLED", "H_OTEL_INTERNAL_EXPORTER_URL", "H_OTEL_TRACES_EXPORTER",
		"H_OTEL_DATA_INPUTS", "H_OTEL_DATA_OUTPUTS", "H_OTEL_DATA_ERRORS"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
	for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TIMEOUT", "OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_METRICS_EXPORTER", "OTEL_LOGS_EXPORTER"} {
		t.Setenv(canalquery.EnvPrefix+name, "")
		_ = os.Unsetenv(canalquery.EnvPrefix + name)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)
	shutdown, err := telemetry.Setup(ctx, "anfra", "test", caller.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	return shutdown
}

// runWithCanary runs commands holding the canary against real sidecars, with
// settings, and waits for every span to be exported: the sidecars' as they
// stop, anfra's when flushed.
func runWithCanary(ctx context.Context, t *testing.T, fixture string, settings caller.Settings) {
	t.Helper()
	local := repo.Resolve(fixture)
	r := local.Repo()
	clients, stop := startSidecarsUntil(ctx, t, r)
	// The repo named on every span, as the CLI's host names it.
	cc := command.CommandContext{Clients: clients, TenantID: "local", Repo: r, AllowLocalRepo: true, DataPerms: dataperm.Ungoverned(),
		TraceData: settings, Observers: command.Observers{Queries: telemetry.Queries{}},
		Attribution: attribution.Fields{caller.RepoID.String(): local.ID}}
	for _, req := range []Request{
		// In a filter's value: an input.
		{Command: "query", Args: map[string]any{"dataset": "ecommerce", "query": "explore { dimensions { name: products.name } }",
			"input": map[string]any{"filters": []any{map[string]any{"field": "products.name", "operator": "is", "values": []any{canary}}}}}},
		// A literal in the SQL sent, and so in the result: SQL and an output.
		{Command: "query", Args: map[string]any{"lang": "sql", "data_source": "demo", "query": "select '" + canary + "' as v"}},
		// In the warehouse's error, quoted, as Postgres quotes a value it refuses.
		{Command: "query", Args: map[string]any{"lang": "sql", "data_source": "demo", "query": "select 1 / 0 where '" + canary + "' = 'x' or cast('" + canary + "' as int) = 1"}},
	} {
		_, _ = Dispatch(ctx, cc, req) // the last fails, as it should
	}
	stop()
	if p, ok := otel.GetTracerProvider().(interface{ ForceFlush(context.Context) error }); ok {
		if err := p.ForceFlush(ctx); err != nil {
			t.Logf("flush anfra's spans: %v", err)
		}
	}
}

// collector is an OTLP/HTTP endpoint that keeps every trace exported to it.
type collector struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []*coltrace.ExportTraceServiceRequest
}

func newCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			body = zr
		}
		b, err := io.ReadAll(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		req := &coltrace.ExportTraceServiceRequest{}
		if strings.Contains(r.Header.Get("Content-Type"), "json") {
			err = protojson.Unmarshal(b, req)
		} else {
			err = proto.Unmarshal(b, req)
		}
		if err != nil {
			t.Errorf("the collector couldn't read an export from %s: %v", r.UserAgent(), err)
		}
		c.mu.Lock()
		c.reqs = append(c.reqs, req)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *collector) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqs = nil
}

// look is the services that exported spans, and where the canary is in them:
// "service, span: key = value" for each place.
func (c *collector) look() (services, leaks []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, req := range c.reqs {
		for _, rs := range req.GetResourceSpans() {
			service := "?"
			for _, a := range rs.GetResource().GetAttributes() {
				if a.GetKey() == "service.name" {
					service = a.GetValue().GetStringValue()
				}
			}
			for _, ss := range rs.GetScopeSpans() {
				if len(ss.GetSpans()) > 0 && !slices.Contains(services, service) {
					services = append(services, service)
				}
				for _, s := range ss.GetSpans() {
					at := service + ", " + s.GetName() + ": "
					check := func(key, value string) {
						if strings.Contains(value, canary) {
							leaks = append(leaks, at+key+" = "+value)
						}
					}
					check("the span's name", s.GetName())
					check("its status", s.GetStatus().GetMessage())
					checkAttrs(s.GetAttributes(), "", check)
					for _, e := range s.GetEvents() {
						check("an event's name", e.GetName())
						checkAttrs(e.GetAttributes(), e.GetName()+".", check)
					}
				}
			}
		}
	}
	return services, leaks
}

// missingStable is each stable attribute (caller.StableSpans) that no span of
// anfra's ops exported holds, "span: attribute", and each stable span of an
// op that none was exported for. The command's root span isn't one an op
// records: cmd/anfra checks it.
func (c *collector) missingStable() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	held := map[string]map[string]bool{} // by stable span, the attributes seen
	for _, req := range c.reqs {
		for _, rs := range req.GetResourceSpans() {
			for _, ss := range rs.GetScopeSpans() {
				for _, s := range ss.GetSpans() {
					for _, st := range caller.StableSpans {
						if !st.Matches(s.GetName()) {
							continue
						}
						if held[st.Name] == nil {
							held[st.Name] = map[string]bool{}
						}
						for _, a := range s.GetAttributes() {
							held[st.Name][a.GetKey()] = true
						}
					}
				}
			}
		}
	}
	var missing []string
	for _, st := range caller.StableSpans {
		if strings.HasPrefix(st.Name, "anfra ") {
			continue
		}
		if held[st.Name] == nil {
			missing = append(missing, st.Name+": no span exported")
			continue
		}
		for _, k := range st.Attributes {
			if !held[st.Name][k.String()] {
				missing = append(missing, st.Name+": "+k.String())
			}
		}
	}
	return missing
}

// checkAttrs checks each attribute's value, as text, with check.
func checkAttrs(attrs []*common.KeyValue, prefix string, check func(key, value string)) {
	for _, a := range attrs {
		check(prefix+a.GetKey(), protojson.Format(a.GetValue()))
	}
}
