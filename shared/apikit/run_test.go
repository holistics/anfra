package apikit_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
	"github.com/holistics/anfra/shared/jsonkit"
	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/shared/apikit"
)

// An op that returns a Run: it counts to n, a tick event per number, and answers
// the total as its page. A negative n is refused by the handler, before any
// event; fail_at fails the run once it has ticked that far.

type countIn struct {
	N      int `json:"n"`
	FailAt int `json:"fail_at,omitempty"`
	// Pause waits this many milliseconds after the first tick.
	Pause int `json:"pause,omitempty"`
}

type countPage struct {
	Total int `json:"total"`
}

type countEvent struct {
	Type string `json:"type" enum:"tick"`
	I    int    `json:"i"`
}

type countRun = apikit.Run[countPage, countEvent]

func count(name string, timeout time.Duration) *apikit.Def[string, countIn, countRun] {
	return &apikit.Def[string, countIn, countRun]{
		Name: name, Summary: "Count.", HTTP: true, MCP: true, Timeout: timeout, Errors: []apperr.AnyCode{notAllow, tooSlow},
		Handle: func(_ context.Context, _ string, in countIn) (countRun, error) {
			if in.N < 0 {
				return countRun{}, apperr.New(apperr.InvalidRequest, "Count to a number that is not negative.")
			}
			tick := func(ctx context.Context, emit func(countEvent) error) error {
				for i := range in.N {
					if in.FailAt > 0 && i == in.FailAt {
						return notAllow
					}
					if err := emit(countEvent{Type: "tick", I: i}); err != nil {
						return err
					}
					if i == 0 && in.Pause > 0 {
						select {
						case <-time.After(time.Duration(in.Pause) * time.Millisecond):
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
				return nil
			}
			return countRun{
				Page: func(ctx context.Context) (countPage, error) {
					total := 0
					err := tick(ctx, func(countEvent) error { total++; return nil })
					return countPage{Total: total}, err
				},
				Stream: func(ctx context.Context, events apikit.Events[countEvent]) error {
					if err := events.Status("counting"); err != nil {
						return err
					}
					return tick(ctx, events.Emit)
				},
			}, nil
		},
	}
}

// Invoke answers a Run's page; InvokeStream emits start, then the run's status
// and its events. Its types are the page's and the events'.
func TestARunAnswersItsPageOrItsEvents(t *testing.T) {
	o := one(t, count("a.count", 0))
	if o.OutType().Name() != "countPage" || len(o.EventTypes()) != 1 || o.EventTypes()[0].Name() != "countEvent" {
		t.Errorf("types = %v, %v; want the page's and the events'", o.OutType(), o.EventTypes())
	}
	out, err := o.Invoke(context.Background(), runtime(), request{user: "ann"}, []byte(`{"n":3}`))
	if err != nil || out.(countPage).Total != 3 {
		t.Errorf("page = %v, %v; want a total of 3", out, err)
	}
	var got []string
	err = o.InvokeStream(context.Background(), runtime(), request{user: "ann"}, []byte(`{"n":2}`), func(e any) error {
		got = append(got, fmt.Sprintf("%T", e))
		return nil
	})
	if want := "apikit.StreamStart,apikit.StreamStatus,apikit_test.countEvent,apikit_test.countEvent"; err != nil || strings.Join(got, ",") != want {
		t.Errorf("events = %v, %v; want %s", got, err, want)
	}
	greeting := one(t, greet("greetings.create"))
	if greeting.EventTypes() != nil {
		t.Error("an op without a Run has an event type")
	}
	if err := greeting.InvokeStream(context.Background(), runtime(), request{user: "ann"}, []byte(`{"name":"Zed"}`), func(any) error { return nil }); err == nil {
		t.Error("an op without a Run streamed")
	}
}

// The op's timeout covers executing the run, which happens after the handler
// has returned: in a page and in a stream alike.
func TestTheTimeoutCoversTheRun(t *testing.T) {
	o := one(t, count("slow.count", 20*time.Millisecond))
	in := []byte(`{"n":2,"pause":5000}`)
	if _, err := o.Invoke(context.Background(), runtime(), request{user: "ann"}, in); code(err) != "too_slow" {
		t.Errorf("page: got %v, want too_slow", err)
	}
	err := o.InvokeStream(context.Background(), runtime(), request{user: "ann"}, in, func(any) error { return nil })
	if code(err) != "too_slow" {
		t.Errorf("stream: got %v, want too_slow", err)
	}
}

func countServer(t *testing.T, defs ...*apikit.Def[string, countIn, countRun]) http.Handler {
	t.Helper()
	reg := newRegistry()
	for _, d := range defs {
		apikit.Register(reg, admission, d)
	}
	return httpkit.Wrap(adapter().Handler(runtime(), reg), httpkit.Config{Codes: codes})
}

// lines are an NDJSON body's events, decoded.
func lines(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for l := range strings.SplitSeq(strings.TrimSpace(body), "\n") {
		var e map[string]any
		if err := jsonkit.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("line %q: %v", l, err)
		}
		events = append(events, e)
	}
	return events
}

func types(events []map[string]any) string {
	var ts []string
	for _, e := range events {
		ts = append(ts, e["type"].(string))
	}
	return strings.Join(ts, ",")
}

// Over HTTP, a request that accepts NDJSON gets the events, one per line; any
// other gets the page as JSON.
func TestHTTPStreamsWhenAsked(t *testing.T) {
	h := countServer(t, count("a.count", 0))
	w := do(h, http.MethodPost, "/api/a.count", "ann", `{"n":2}`)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" || !strings.Contains(w.Body.String(), `"total":2`) {
		t.Errorf("JSON: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
	w = do(h, http.MethodPost, "/api/a.count", "ann", `{"n":2}`, "Accept", "application/json, application/x-ndjson;q=0.9")
	if w.Code != 200 || w.Header().Get("Content-Type") != apikit.NDJSON || types(lines(t, w.Body.String())) != "start,status,tick,tick,end" {
		t.Errorf("stream: %d %s %s", w.Code, w.Header().Get("Content-Type"), w.Body)
	}
}

// What the handler refuses, before the stream starts, is an ordinary error
// response; a failure after it ends the stream with end, failed, carrying the
// error as an error response's body would. A run that succeeds ends with end,
// succeeded.
func TestAStreamsErrors(t *testing.T) {
	h := countServer(t, count("a.count", 0))
	w := do(h, http.MethodPost, "/api/a.count", "ann", `{"n":-1}`, "Accept", apikit.NDJSON)
	if w.Code != 400 || w.Header().Get("Content-Type") != "application/json" || !strings.Contains(w.Body.String(), `"invalid_request"`) {
		t.Errorf("refused: %d %s", w.Code, w.Body)
	}
	w = do(h, http.MethodPost, "/api/a.count", "", `{"n":1}`, "Accept", apikit.NDJSON)
	if w.Code != 401 {
		t.Errorf("not admitted: %d %s", w.Code, w.Body)
	}
	w = do(h, http.MethodPost, "/api/a.count", "ann", `{"n":3,"fail_at":2}`, "Accept", apikit.NDJSON)
	events := lines(t, w.Body.String())
	if w.Code != 200 || types(events) != "start,status,tick,tick,end" {
		t.Fatalf("failed: %d %s", w.Code, w.Body)
	}
	end := events[len(events)-1]
	if e, _ := end["error"].(map[string]any); end["status"] != "failed" || e["code"] != "not_allowed" || e["status"] != float64(403) || e["request_id"] == "" {
		t.Errorf("end = %v", end)
	}
	w = do(h, http.MethodPost, "/api/a.count", "ann", `{"n":1}`, "Accept", apikit.NDJSON)
	if end := lines(t, w.Body.String()); end[len(end)-1]["status"] != "succeeded" || end[len(end)-1]["error"] != nil {
		t.Errorf("succeeded: %s", w.Body)
	}
}

// A stream idle for the ping interval gets a ping, which keeps proxies from
// closing it.
func TestAnIdleStreamIsPinged(t *testing.T) {
	defer apikit.SetPingEvery(20 * time.Millisecond)()
	h := countServer(t, count("a.count", 0))
	w := do(h, http.MethodPost, "/api/a.count", "ann", `{"n":2,"pause":100}`, "Accept", apikit.NDJSON)
	got := types(lines(t, w.Body.String()))
	if !strings.HasPrefix(got, "start,status,tick,ping") || !strings.HasSuffix(got, "ping,tick,end") {
		t.Errorf("events = %s, want pings between the ticks", got)
	}
}

// A mounted op that returns a Run streams through the host's admission.
func TestAMountedRunStreams(t *testing.T) {
	lib := apikit.NewRegistry(apikit.RegistryConfig[team, team]{Namespace: libNS})
	lib.Group("core", "The library's commands.")
	apikit.Register(lib, apikit.Admission[team, team]{Admit: func(_ context.Context, t team) (team, error) { return t, nil }},
		&apikit.Def[team, countIn, countRun]{Name: "core.count", Summary: "Count.", HTTP: true,
			Handle: func(ctx context.Context, _ team, in countIn) (countRun, error) {
				return count("core.count", 0).Handle(ctx, "", in)
			}})
	var bound []string
	host := newRegistry()
	host.Group("core", "Mounted.")
	apikit.Mount(host, lib, apikit.Untyped("core.count"), untyped(&bound))
	o, _ := host.Lookup("core.count")
	if o.EventTypes() == nil {
		t.Fatal("the mounted op does not stream")
	}
	n := 0
	err := o.InvokeStream(context.Background(), runtime(), request{user: "ann"}, []byte(`{"n":2}`), func(any) error { n++; return nil })
	if err != nil || n != 4 || len(bound) != 1 {
		t.Errorf("events %d, err %v, bound %v; want start, status and 2 ticks through the host's admission", n, err, bound)
	}
	err = o.InvokeStream(context.Background(), runtime(), request{}, []byte(`{"n":2}`), func(any) error { return nil })
	if !errors.Is(err, noEntry) {
		t.Errorf("not admitted: %v", err)
	}
}

// countLibrary is a library registry holding core.count.
func countLibrary() *apikit.Registry[team, team] {
	lib := apikit.NewRegistry(apikit.RegistryConfig[team, team]{Namespace: libNS})
	lib.Group("core", "The library's commands.")
	apikit.Register(lib, apikit.Admission[team, team]{Admit: func(_ context.Context, t team) (team, error) { return t, nil }},
		&apikit.Def[team, countIn, countRun]{Name: "core.count", Summary: "Count.", HTTP: true, Errors: []apperr.AnyCode{notAllow},
			Handle: func(ctx context.Context, _ team, in countIn) (countRun, error) {
				return count("core.count", 0).Handle(ctx, "", in)
			}})
	return lib
}

// recorder is an executor that keeps a record of runs: each submitted, for
// whom, its ID on the stream's start; and how each ended. It refuses a caller
// named "full", as a full queue would.
type recorder struct{ log []string }

func (e *recorder) Submit(_ context.Context, caller string, s apikit.Submission) (apikit.Execution, error) {
	if caller == "full" {
		return apikit.Execution{}, apperr.New(apperr.InvalidRequest, "The queue is full.")
	}
	e.log = append(e.log, "submitted "+s.Op.Name+" for "+caller)
	end := func(err error) {
		outcome := "succeeded"
		if err != nil {
			outcome = code(err)
		}
		e.log = append(e.log, "ended: "+outcome)
	}
	return apikit.Execution{ID: "run_1", Work: apikit.Work{
		Page: func(ctx context.Context) (any, error) {
			page, err := s.Work.Page(ctx)
			end(err)
			return page, err
		},
		Stream: func(ctx context.Context, emit func(any) error) error {
			err := s.Work.Stream(ctx, emit)
			end(err)
			return err
		},
	}}, nil
}

// A registry's executor is submitted each run of its ops, own and mounted, for
// the registry's caller — not a call the op refuses — before anything is sent:
// its ID goes on the stream's start, and its execution is what is run, in a
// page as in a stream. What it refuses is an ordinary error, before the
// stream starts.
func TestARegistrysExecutorExecutesItsRuns(t *testing.T) {
	exec := &recorder{}
	host := apikit.NewRegistry(apikit.RegistryConfig[request, string]{Namespace: hostNS, Executor: exec})
	host.Group("a", "Letters.")
	host.Group("core", "Mounted.")
	apikit.Register(host, admission, count("a.count", 0))
	apikit.Mount(host, countLibrary(), apikit.Untyped("core.count"), untyped(new([]string)))
	h := httpkit.Wrap(adapter().Handler(runtime(), host), httpkit.Config{Codes: codes})

	for _, path := range []string{"/api/a.count", "/api/core.count"} {
		w := do(h, http.MethodPost, path, "ann", `{"n":1}`, "Accept", apikit.NDJSON)
		if events := lines(t, w.Body.String()); events[0]["type"] != "start" || events[0]["id"] != "run_1" {
			t.Errorf("%s: start = %v, want the run's ID", path, events[0])
		}
	}
	do(h, http.MethodPost, "/api/core.count", "ann", `{"n":3,"fail_at":2}`, "Accept", apikit.NDJSON)
	do(h, http.MethodPost, "/api/core.count", "ann", `{"n":1}`)
	do(h, http.MethodPost, "/api/core.count", "ann", `{"n":-1}`, "Accept", apikit.NDJSON) // refused by the op: no run
	want := "submitted a.count for ann|ended: succeeded|submitted core.count for ann|ended: succeeded|" +
		"submitted core.count for ann|ended: not_allowed|submitted core.count for ann|ended: succeeded"
	if got := strings.Join(exec.log, "|"); got != want {
		t.Errorf("the executor saw %q, want %q", got, want)
	}
	w := do(h, http.MethodPost, "/api/core.count", "full", `{"n":1}`, "Accept", apikit.NDJSON)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "The queue is full.") {
		t.Errorf("refused by the executor: %d %s", w.Code, w.Body)
	}
}

// Without an executor, a run is executed as it is, now: its stream starts with
// no ID.
func TestWithoutAnExecutorARunIsExecutedInline(t *testing.T) {
	plain := newRegistry()
	plain.Group("core", "Mounted.")
	apikit.Mount(plain, countLibrary(), apikit.Untyped("core.count"), untyped(new([]string)))
	w := do(httpkit.Wrap(adapter().Handler(runtime(), plain), httpkit.Config{Codes: codes}), http.MethodPost, "/api/core.count", "ann", `{"n":1}`, "Accept", apikit.NDJSON)
	if events := lines(t, w.Body.String()); events[0]["type"] != "start" || events[0]["id"] != nil || events[len(events)-1]["status"] != "succeeded" {
		t.Errorf("events = %v, want a start with no ID, and the run to succeed", events)
	}
}

// The document gives a Run's 200 both forms: the page as JSON, and the events
// as NDJSON — the op's, then apikit's — told apart by their type. Discovery
// shows the events too.
func TestARunsSpec(t *testing.T) {
	reg := newRegistry()
	apikit.Register(reg, admission, count("a.count", 0))
	raw, err := adapter().Spec(runtime(), reg)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref   string `yaml:"$ref"`
						OneOf []struct {
							Ref string `yaml:"$ref"`
						} `yaml:"oneOf"`
						Discriminator struct {
							PropertyName string            `yaml:"propertyName"`
							Mapping      map[string]string `yaml:"mapping"`
						} `yaml:"discriminator"`
					} `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ok := doc.Paths["/a.count"]["post"].Responses["200"].Content
	if !strings.HasSuffix(ok["application/json"].Schema.Ref, "/CountPage") {
		t.Errorf("JSON 200 = %+v, want the page", ok["application/json"])
	}
	var refs []string
	for _, s := range ok[apikit.NDJSON].Schema.OneOf {
		refs = append(refs, s.Ref[strings.LastIndex(s.Ref, "/")+1:])
	}
	if strings.Join(refs, ",") != "CountEvent,StreamStart,StreamStatus,StreamEnd,StreamPing" {
		t.Errorf("NDJSON 200 = %v", refs)
	}
	d := ok[apikit.NDJSON].Schema.Discriminator
	if d.PropertyName != "type" || !strings.HasSuffix(d.Mapping["tick"], "/CountEvent") || !strings.HasSuffix(d.Mapping["end"], "/StreamEnd") || len(d.Mapping) != 5 {
		t.Errorf("discriminator = %+v", d)
	}

	disc := apikit.Discovery[request]{Codes: codes}
	u, err := disc.Usage(context.Background(), runtime(), reg, request{user: "ann"}, func(apikit.Meta) bool { return true }, "a.count")
	if err != nil || u.Events == nil || u.Output == nil {
		t.Errorf("usage = %+v, %v; want its output and its events", u, err)
	}
}

// An op's events name their types in a "type" field's enum, none of them
// apikit's own; Register refuses anything else.
func TestRegisterChecksTheEventTypes(t *testing.T) {
	type untyped struct {
		I int `json:"i"`
	}
	type noEnum struct {
		Type string `json:"type"`
	}
	type taken struct {
		Type string `json:"type" enum:"rows,end"`
	}
	register := func(def func(reg *apikit.Registry[request, string])) func() {
		return func() { def(newRegistry()) }
	}
	mustPanic(t, `string field "type"`, register(func(reg *apikit.Registry[request, string]) {
		apikit.Register(reg, admission, &apikit.Def[string, struct{}, apikit.Run[countPage, untyped]]{Name: "a.x", Summary: "x",
			Handle: func(context.Context, string, struct{}) (apikit.Run[countPage, untyped], error) {
				return apikit.Run[countPage, untyped]{}, nil
			}})
	}))
	mustPanic(t, `string field "type"`, register(func(reg *apikit.Registry[request, string]) {
		apikit.Register(reg, admission, &apikit.Def[string, struct{}, apikit.Run[countPage, noEnum]]{Name: "a.x", Summary: "x",
			Handle: func(context.Context, string, struct{}) (apikit.Run[countPage, noEnum], error) {
				return apikit.Run[countPage, noEnum]{}, nil
			}})
	}))
	mustPanic(t, `"end" is reserved by apikit`, register(func(reg *apikit.Registry[request, string]) {
		apikit.Register(reg, admission, &apikit.Def[string, struct{}, apikit.Run[countPage, taken]]{Name: "a.x", Summary: "x",
			Handle: func(context.Context, string, struct{}) (apikit.Run[countPage, taken], error) {
				return apikit.Run[countPage, taken]{}, nil
			}})
	}))
}

// An op whose events are of several types: an interface, each type declared in
// Events. Each is its own schema, mapped from its type's name.

type letterEvent interface{ letter() }

type vowel struct {
	Type string `json:"type" enum:"vowel"`
	V    string `json:"v"`
}

type consonant struct {
	Type string `json:"type" enum:"consonant"`
	C    string `json:"c"`
}

func (vowel) letter()     {}
func (consonant) letter() {}

type lettersRun = apikit.Run[countPage, letterEvent]

func letters() *apikit.Def[string, struct{}, lettersRun] {
	return &apikit.Def[string, struct{}, lettersRun]{
		Name: "a.letters", Summary: "Spell.", HTTP: true, Events: []any{vowel{}, consonant{}},
		Handle: func(context.Context, string, struct{}) (lettersRun, error) {
			return lettersRun{
				Page: func(context.Context) (countPage, error) { return countPage{Total: 2}, nil },
				Stream: func(_ context.Context, events apikit.Events[letterEvent]) error {
					if err := events.Emit(consonant{Type: "consonant", C: "h"}); err != nil {
						return err
					}
					return events.Emit(vowel{Type: "vowel", V: "i"})
				},
			}, nil
		},
	}
}

func TestARunWithSeveralEventTypes(t *testing.T) {
	reg := newRegistry()
	apikit.Register(reg, admission, letters())
	h := httpkit.Wrap(adapter().Handler(runtime(), reg), httpkit.Config{Codes: codes})
	w := do(h, http.MethodPost, "/api/a.letters", "ann", `{}`, "Accept", apikit.NDJSON)
	if got := types(lines(t, w.Body.String())); got != "start,consonant,vowel,end" {
		t.Errorf("events = %s", got)
	}
	raw, err := adapter().Spec(runtime(), reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`consonant: "#/components/schemas/Consonant"`, `vowel: "#/components/schemas/Vowel"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the spec does not map %s", want)
		}
	}
}

// Register refuses events that are an interface without their types declared,
// a declared type that is not one of them, and two types of one name.
func TestRegisterChecksSeveralEventTypes(t *testing.T) {
	for want, events := range map[string][]any{
		"declare each type in Events":         nil,
		"does not implement":                  {vowel{}, struct{ Type string }{}},
		`two of its events are named "vowel"`: {vowel{}, vowelToo{}},
	} {
		d := letters()
		d.Events = events
		mustPanic(t, want, func() { apikit.Register(newRegistry(), admission, d) })
	}
}

type vowelToo struct {
	Type string `json:"type" enum:"vowel"`
}

func (vowelToo) letter() {}

// holder is an executor that holds each run before executing it, as one
// waiting for capacity does.
type holder struct{ hold time.Duration }

func (h holder) Submit(_ context.Context, _ string, s apikit.Submission) (apikit.Execution, error) {
	return apikit.Execution{Work: apikit.Work{
		Page: func(ctx context.Context) (any, error) {
			time.Sleep(h.hold)
			return s.Work.Page(ctx)
		},
		Stream: func(ctx context.Context, emit func(any) error) error {
			time.Sleep(h.hold)
			return s.Work.Stream(ctx, emit)
		},
	}}, nil
}

// The op's timeout — and a mount's — bounds its run from when the run starts,
// not what its executor does first: a run held longer than the timeout still
// has all of it, and running past it still runs out.
func TestTheTimeoutStartsWithTheRun(t *testing.T) {
	host := apikit.NewRegistry(apikit.RegistryConfig[request, string]{Namespace: hostNS, Executor: holder{hold: 60 * time.Millisecond}})
	host.Group("a", "Letters.")
	host.Group("core", "Mounted.")
	apikit.Register(host, admission, count("a.count", 30*time.Millisecond))
	apikit.Mount(host, countLibrary(), apikit.Untyped("core.count"), untyped(new([]string)), apikit.WithTimeout(30*time.Millisecond))

	for _, name := range []string{"a.count", "core.count"} {
		o, _ := host.Lookup(name)
		for in, want := range map[string]string{`{"n":1}`: "", `{"n":2,"pause":200}`: "too_slow"} {
			if _, err := o.Invoke(context.Background(), runtime(), request{user: "ann"}, []byte(in)); codeOrNone(err) != want {
				t.Errorf("%s %s page: got %v, want %q", name, in, err, want)
			}
			err := o.InvokeStream(context.Background(), runtime(), request{user: "ann"}, []byte(in), func(any) error { return nil })
			if codeOrNone(err) != want {
				t.Errorf("%s %s stream: got %v, want %q", name, in, err, want)
			}
		}
	}
}

// codeOrNone is err's public code, or "" for none.
func codeOrNone(err error) string {
	if err == nil {
		return ""
	}
	return code(err)
}

// A client gone before the stream's start still has its execution executed,
// cancelled: the executor sees the end of every run it took.
func TestAnExecutionIsExecutedWhenItsClientLeftBeforeItsStart(t *testing.T) {
	exec := &recorder{}
	host := apikit.NewRegistry(apikit.RegistryConfig[request, string]{Namespace: hostNS, Executor: exec})
	host.Group("a", "Letters.")
	apikit.Register(host, admission, count("a.count", 0))
	o, _ := host.Lookup("a.count")
	gone := errors.New("the client is gone")
	err := o.InvokeStream(context.Background(), runtime(), request{user: "ann"}, []byte(`{"n":1}`), func(any) error { return gone })
	if !errors.Is(err, gone) {
		t.Fatalf("err = %v, want the failed start's", err)
	}
	if got := strings.Join(exec.log, "|"); !strings.HasPrefix(got, "submitted a.count for ann|ended: ") {
		t.Errorf("the executor saw %q, want the run submitted and ended", got)
	}
}
