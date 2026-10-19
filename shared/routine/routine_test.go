package routine_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/goleak"

	"github.com/holistics/anfra/shared/routine"
)

var reader = sdkmetric.NewManualReader()

func TestMain(m *testing.M) {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	goleak.VerifyTestMain(m)
	os.Exit(0) // unreached: VerifyTestMain exits
}

const short = 5 * time.Millisecond

var errBoom = errors.New("boom")

// logs is a logger writing to a buffer the test can read.
type logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logs) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func newGroup(t *testing.T) (*routine.Group, *logs) {
	t.Helper()
	l := &logs{}
	g := routine.NewGroup("test", slog.New(slog.NewTextHandler(l, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { _ = g.Stop(context.Background()) })
	return g, l
}

// waitFor fails the test unless ch receives within a second.
func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func stopWithin(t *testing.T, g *routine.Group, d time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return g.Stop(ctx)
}

func TestEveryRunsRepeatedly(t *testing.T) {
	g, _ := newGroup(t)
	var n atomic.Int32
	enough := make(chan struct{})
	g.Every("every.repeats", short, func(context.Context) error {
		if n.Add(1) == 3 {
			close(enough)
		}
		return nil
	})
	g.Start(context.Background())
	waitFor(t, enough, "three runs")
}

// Immediately runs the first run at Start, not a random part of the interval
// later.
func TestEveryImmediately(t *testing.T) {
	g, _ := newGroup(t)
	ran := make(chan struct{})
	var once sync.Once
	g.Every("every.immediately", time.Hour, func(context.Context) error {
		once.Do(func() { close(ran) })
		return nil
	}, routine.Immediately())
	g.Start(context.Background())
	waitFor(t, ran, "the first run")
}

// A failed run is logged, and the next tick runs as usual.
func TestEveryCarriesOnAfterAnError(t *testing.T) {
	g, l := newGroup(t)
	var n atomic.Int32
	again := make(chan struct{})
	g.Every("every.fails", short, func(context.Context) error {
		if n.Add(1) == 2 {
			close(again)
		}
		return errBoom
	})
	g.Start(context.Background())
	waitFor(t, again, "a second run")
	if !strings.Contains(l.String(), "routine.failed") || !strings.Contains(l.String(), "boom") {
		t.Errorf("the failure was not logged:\n%s", l)
	}
}

// Stop lets a run in progress finish, with a live context, and starts no new
// one.
func TestStopLetsAnEveryRunFinish(t *testing.T) {
	g, _ := newGroup(t)
	var n atomic.Int32
	running, release := make(chan struct{}), make(chan struct{})
	var ctxErr error
	g.Every("every.finishes", short, func(ctx context.Context) error {
		if n.Add(1) > 1 {
			t.Error("a run started after Stop")
			return nil
		}
		close(running)
		<-release
		ctxErr = ctx.Err()
		return nil
	}, routine.Immediately())
	g.Start(context.Background())
	waitFor(t, running, "the run")

	stopped := make(chan struct{})
	go func() { _ = g.Stop(context.Background()); close(stopped) }()
	select {
	case <-stopped:
		t.Fatal("Stop returned before the run finished")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	waitFor(t, stopped, "Stop")
	if ctxErr != nil {
		t.Errorf("the run's context ended before it finished: %v", ctxErr)
	}
}

// Once Stop's own context ends, a run still in progress is cancelled, and Stop
// returns that context's error.
func TestStopCancelsARunAtItsDeadline(t *testing.T) {
	g, l := newGroup(t)
	running := make(chan struct{})
	g.Every("every.cut_short", short, func(ctx context.Context) error {
		close(running)
		<-ctx.Done()
		return ctx.Err()
	}, routine.Immediately())
	g.Start(context.Background())
	waitFor(t, running, "the run")

	if err := stopWithin(t, g, 20*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop = %v, want the deadline's error", err)
	}
	if !strings.Contains(l.String(), "routine.cut_short") {
		t.Errorf("the cut-short run was not logged:\n%s", l)
	}
}

// A loop that returns early is restarted after backoff.
func TestLoopRestarts(t *testing.T) {
	g, l := newGroup(t)
	var n atomic.Int32
	restarted := make(chan struct{})
	g.Loop("loop.restarts", func(ctx context.Context) error {
		if n.Add(1) == 3 {
			close(restarted)
			<-ctx.Done()
			return nil
		}
		return errBoom
	}, routine.RestartBackoff(time.Millisecond, 2*time.Millisecond))
	before := sum(t, "anfra.routine.restarts", "loop.restarts", "")
	g.Start(context.Background())
	waitFor(t, restarted, "two restarts")
	if !strings.Contains(l.String(), "boom") {
		t.Errorf("the loop's error was not logged:\n%s", l)
	}
	if got := sum(t, "anfra.routine.restarts", "loop.restarts", "") - before; got != 2 {
		t.Errorf("restarts = %d, want 2", got)
	}
}

// A loop returning without an error before its context ended is restarted
// too: it is meant to run until then.
func TestLoopRestartsAfterAnEarlyNilReturn(t *testing.T) {
	g, _ := newGroup(t)
	var n atomic.Int32
	restarted := make(chan struct{})
	g.Loop("loop.early_nil", func(ctx context.Context) error {
		if n.Add(1) == 2 {
			close(restarted)
			<-ctx.Done()
		}
		return nil
	}, routine.RestartBackoff(time.Millisecond, time.Millisecond))
	g.Start(context.Background())
	waitFor(t, restarted, "the restart")
}

// Stop cancels a loop at once, and cuts its backoff short.
func TestStopCancelsALoop(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(context.Context) error
	}{
		{"running", func(ctx context.Context) error { <-ctx.Done(); return nil }},
		{"backing_off", func(context.Context) error { return errBoom }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := newGroup(t)
			g.Loop("loop.stop."+tc.name, tc.fn, routine.RestartBackoff(time.Hour, time.Hour))
			g.Start(context.Background())
			time.Sleep(short)
			if err := stopWithin(t, g, time.Second); err != nil {
				t.Errorf("Stop = %v, want nil", err)
			}
		})
	}
}

// writeBehind is the doc's worker: its owner feeds and closes its input.
type writeBehind struct {
	in      chan int
	written []int
	ctxErr  error
}

func (w *writeBehind) work(ctx context.Context) {
	for {
		select {
		case r, ok := <-w.in:
			if !ok {
				w.ctxErr = ctx.Err()
				return
			}
			time.Sleep(time.Millisecond) // a write
			w.written = append(w.written, r)
		case <-ctx.Done():
			return
		}
	}
}

// Stop drains a worker: once its owner closes its input, it handles what is
// left, with a live context.
func TestStopDrainsAWorker(t *testing.T) {
	g, _ := newGroup(t)
	w := &writeBehind{in: make(chan int, 10)}
	g.Worker("worker.drains", w.work)
	g.Start(context.Background())
	for i := range 10 {
		w.in <- i
	}
	close(w.in)
	if err := stopWithin(t, g, time.Second); err != nil {
		t.Fatalf("Stop = %v, want nil", err)
	}
	if len(w.written) != 10 {
		t.Errorf("wrote %d items, want 10", len(w.written))
	}
	if w.ctxErr != nil {
		t.Errorf("the worker's context ended during its drain: %v", w.ctxErr)
	}
}

// A worker whose input is never closed is cancelled at Stop's deadline.
func TestStopCancelsAWorkerAtItsDeadline(t *testing.T) {
	g, l := newGroup(t)
	w := &writeBehind{in: make(chan int)}
	g.Worker("worker.cut_short", w.work)
	g.Start(context.Background())
	if err := stopWithin(t, g, 20*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Stop = %v, want the deadline's error", err)
	}
	if !strings.Contains(l.String(), "routine.cut_short") {
		t.Errorf("the cut-short worker was not logged:\n%s", l)
	}
}

func TestTask(t *testing.T) {
	g, l := newGroup(t)
	if g.Task("task.early", func(context.Context) error { return nil }) {
		t.Error("Task ran before Start")
	}
	g.Start(context.Background())

	ran := make(chan struct{})
	if !g.Task("task.runs", func(context.Context) error { close(ran); return errBoom }) {
		t.Fatal("Task refused after Start")
	}
	waitFor(t, ran, "the task")

	// Stop lets a running task finish.
	running, release := make(chan struct{}), make(chan struct{})
	var ctxErr error
	g.Task("task.finishes", func(ctx context.Context) error {
		close(running)
		<-release
		ctxErr = ctx.Err()
		return nil
	})
	waitFor(t, running, "the second task")
	stopped := make(chan struct{})
	go func() { _ = g.Stop(context.Background()); close(stopped) }()
	time.Sleep(short)
	if g.Task("task.late", func(context.Context) error { return nil }) {
		t.Error("Task ran once Stop had begun")
	}
	close(release)
	waitFor(t, stopped, "Stop")
	if ctxErr != nil {
		t.Errorf("the task's context ended before it finished: %v", ctxErr)
	}
	if !strings.Contains(l.String(), "boom") {
		t.Errorf("the task's error was not logged:\n%s", l)
	}
}

// Recover counts a panicking run as failed, and the next tick runs as usual.
func TestRecover(t *testing.T) {
	g, l := newGroup(t)
	var n atomic.Int32
	again := make(chan struct{})
	g.Every("every.recovers", short, func(context.Context) error {
		if n.Add(1) == 2 {
			close(again)
		}
		panic("kaboom")
	}, routine.Recover())
	g.Start(context.Background())
	waitFor(t, again, "a run after the panic")
	if !strings.Contains(l.String(), "kaboom") {
		t.Errorf("the panic was not logged:\n%s", l)
	}
	if got := sum(t, "anfra.routine.runs", "every.recovers", "panic"); got < 1 {
		t.Errorf("panicking runs counted = %d, want at least 1", got)
	}
}

// A group stopped before Start never starts its routines.
func TestStopBeforeStart(t *testing.T) {
	g, _ := newGroup(t)
	g.Every("every.never", short, func(context.Context) error {
		t.Error("a group stopped before Start ran a routine")
		return nil
	}, routine.Immediately())
	if err := g.Stop(context.Background()); err != nil {
		t.Errorf("Stop before Start = %v", err)
	}
	g.Start(context.Background())
	time.Sleep(4 * short)
}

func TestStopTwice(t *testing.T) {
	g, _ := newGroup(t)
	g.Loop("loop.twice", func(ctx context.Context) error { <-ctx.Done(); return nil })
	g.Start(context.Background())
	for range 2 {
		if err := stopWithin(t, g, time.Second); err != nil {
			t.Errorf("Stop = %v", err)
		}
	}
}

func TestMisuse(t *testing.T) {
	noop := func(context.Context) error { return nil }
	for name, f := range map[string]func(){
		"register after Start": func() {
			g := routine.NewGroup("misuse", nil)
			g.Start(context.Background())
			defer g.Stop(context.Background()) //nolint:errcheck // a test's cleanup
			g.Every("x", time.Second, noop)
		},
		"register after Stop": func() {
			g := routine.NewGroup("misuse", nil)
			_ = g.Stop(context.Background())
			g.Every("x", time.Second, noop)
		},
		"start twice": func() {
			g := routine.NewGroup("misuse", nil)
			g.Start(context.Background())
			defer g.Stop(context.Background()) //nolint:errcheck // a test's cleanup
			g.Start(context.Background())
		},
		"option of another kind": func() {
			routine.NewGroup("misuse", nil).Loop("x", noop, routine.Immediately())
		},
		"no interval": func() {
			routine.NewGroup("misuse", nil).Every("x", 0, noop)
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("did not panic")
				}
			}()
			f()
		})
	}
}

func TestMetrics(t *testing.T) {
	g, _ := newGroup(t)
	var n atomic.Int32
	enough := make(chan struct{})
	g.Every("every.metrics", short, func(context.Context) error {
		switch n.Add(1) {
		case 1:
			return errBoom
		case 3:
			close(enough)
		}
		return nil
	}, routine.Immediately())
	// The counters add up across the test binary's runs (-count).
	failedBefore := sum(t, "anfra.routine.runs", "every.metrics", "error")
	okBefore := sum(t, "anfra.routine.runs", "every.metrics", "ok")
	g.Start(context.Background())
	waitFor(t, enough, "three runs")
	if err := stopWithin(t, g, time.Second); err != nil {
		t.Fatal(err)
	}
	if got := sum(t, "anfra.routine.runs", "every.metrics", "error") - failedBefore; got != 1 {
		t.Errorf("failed runs = %d, want 1", got)
	}
	if got := sum(t, "anfra.routine.runs", "every.metrics", "ok") - okBefore; got < 2 {
		t.Errorf("ok runs = %d, want at least 2", got)
	}
	if got := sum(t, "anfra.routine.last_success", "every.metrics", ""); got < time.Now().Add(-time.Minute).Unix() {
		t.Errorf("last success = %d, want about now", got)
	}
}

// sum is metric's value for routine, and outcome unless empty: a counter's sum,
// or a gauge's value.
func sum(t *testing.T, metric, name, outcome string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	match := func(set attribute.Set) bool {
		if v, ok := set.Value("routine"); !ok || v.AsString() != name {
			return false
		}
		if outcome == "" {
			return true
		}
		v, ok := set.Value("outcome")
		return ok && v.AsString() == outcome
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metric {
				continue
			}
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					if match(dp.Attributes) {
						total += dp.Value
					}
				}
			case metricdata.Gauge[int64]:
				for _, dp := range data.DataPoints {
					if match(dp.Attributes) {
						total = dp.Value
					}
				}
			}
		}
	}
	return total
}
