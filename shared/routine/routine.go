// Package routine runs a server's background routines: the work it does beside
// its requests (docs/designs/background-routines.md).
//
// A Group owns a set of routines, starts each, and stops them together. Each
// kind is one method:
//
//   - Every: a short run, every interval, on the group's clock.
//   - Loop: one run that lasts, deciding for itself when it next wakes: a
//     listener or a poller that paces itself. Restarted with backoff when it
//     returns early, since its source breaks in normal operation.
//   - Worker: one run that handles a queue other code feeds. Stop drains it.
//   - Task: one piece of work, handed in at any time after Start.
//
// Stop cancels what never ends on its own, a loop, and lets the rest finish,
// until its own context ends. A panic crashes the process, after a log line
// naming the routine, unless the routine was registered with Recover.
//
// The helper keeps nothing that survives a restart: whatever must is the
// routine's own concern, kept in a store by its function.
package routine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"time"
)

type kind int

const (
	kindEvery kind = iota
	kindLoop
	kindWorker
)

func (k kind) String() string {
	return [...]string{"Every", "Loop", "Worker"}[k]
}

type routine struct {
	name     string
	kind     kind
	interval time.Duration               // Every
	fn       func(context.Context) error // Every, Loop
	work     func(context.Context)       // Worker
	opts     options
}

// Group owns a set of routines. Register them with Every, Loop and Worker
// before Start; hand it tasks with Task after.
type Group struct {
	name   string
	logger *slog.Logger

	mu       sync.Mutex
	routines []*routine
	started  bool
	stopped  bool

	// loopCtx is the loops' context, cancelled at once by Stop. drainCtx is every
	// other routine's, cancelled only once Stop's own context ends, or once
	// everything has returned.
	drainCtx                 context.Context // a task's
	cancelLoops, cancelDrain context.CancelFunc
	stopping                 chan struct{} // closed by Stop: start no new run or task
	done                     chan struct{} // closed once every routine has returned, after Stop
	wg                       sync.WaitGroup
}

// NewGroup returns an empty group. name names it in its log lines; logger is
// slog.Default() when nil.
func NewGroup(name string, logger *slog.Logger) *Group {
	if logger == nil {
		logger = slog.Default()
	}
	return &Group{
		name:     name,
		logger:   logger.With("group", name),
		stopping: make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Every runs fn every interval, measured from the end of the last run, with
// jitter, so runs never overlap. Its first run comes a random part of interval
// after Start, unless Immediately, so instances started together do not run in
// lockstep. An error is logged and counted; the next tick runs as usual.
//
// fn returns once it has done what is due, and promptly once its context ends.
// Stop starts no new run and lets the current one finish.
func (g *Group) Every(name string, interval time.Duration, fn func(context.Context) error, opts ...Option) {
	if interval <= 0 {
		panic(fmt.Sprintf("routine: %s: interval %v is not positive", name, interval))
	}
	g.add(&routine{name: name, kind: kindEvery, interval: interval, fn: fn, opts: newOptions(kindEvery, name, opts)})
}

// Loop runs fn, which runs until its context ends. If fn returns before then,
// with an error or without, the return is logged and counted, and fn is called
// again after backoff (RestartBackoff; 1s to 30s by default).
//
// fn opens its source, catches up on what it missed, then consumes. It returns
// an error for an outage of its source, which reopening fixes, and handles a
// failed event itself. Stop cancels its context at once, cutting a backoff
// short.
func (g *Group) Loop(name string, fn func(context.Context) error, opts ...Option) {
	g.add(&routine{name: name, kind: kindLoop, fn: fn, opts: newOptions(kindLoop, name, opts)})
}

// Worker runs fn once. Its return ends it, whenever that is: it is never called
// again.
//
// fn handles a queue its owner feeds, and returns once the owner has closed it
// and it is empty, or once its context ends. It handles every failure itself,
// having no error to return. Stop lets it drain: its context is cancelled only
// once Stop's own context ends.
func (g *Group) Worker(name string, fn func(context.Context)) {
	g.add(&routine{name: name, kind: kindWorker, work: fn})
}

func (g *Group) add(r *routine) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started || g.stopped {
		panic(fmt.Sprintf("routine: %s registered after Start or Stop", r.name))
	}
	g.routines = append(g.routines, r)
}

// Start starts every routine registered. ctx gives them its values, such as a
// tracer's; its cancellation is ignored, since Stop ends them. A group started
// twice panics; one stopped before it started never starts.
func (g *Group) Start(ctx context.Context) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started {
		panic(fmt.Sprintf("routine: group %s started twice", g.name))
	}
	g.started = true
	if g.stopped {
		close(g.done) // nothing runs, so nothing to wait for
		return
	}
	base := context.WithoutCancel(ctx)
	loopCtx, cancelLoops := context.WithCancel(base)
	drainCtx, cancelDrain := context.WithCancel(base)
	g.drainCtx, g.cancelLoops, g.cancelDrain = drainCtx, cancelLoops, cancelDrain
	for _, r := range g.routines {
		if r.kind == kindLoop {
			g.spawn(loopCtx, r)
		} else {
			g.spawn(drainCtx, r)
		}
	}
}

// spawn runs r on ctx: the loops' context for a loop, the drain context for any
// other.
func (g *Group) spawn(ctx context.Context, r *routine) {
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		defer g.crashOnPanic(ctx, r.name)
		g.logger.Debug("routine.start", "routine", r.name)
		defer g.logger.Debug("routine.stop", "routine", r.name)
		switch r.kind {
		case kindEvery:
			g.every(ctx, r)
		case kindLoop:
			g.loop(ctx, r)
		case kindWorker:
			g.worker(ctx, r)
		}
	}()
}

// Task runs fn once, now, on a goroutine of its own, and reports whether it
// did: false before Start, and once Stop has begun, so the caller can do the
// work inline or drop it. An error is logged and counted; nothing retries a
// task, so one that must succeed retries inside fn.
//
// Stop lets a running task finish: its context is cancelled only once Stop's
// own context ends.
func (g *Group) Task(name string, fn func(context.Context) error) bool {
	g.mu.Lock()
	if !g.started || g.stopped {
		g.mu.Unlock()
		return false
	}
	g.wg.Add(1)
	g.mu.Unlock()
	go func() {
		ctx := g.drainCtx
		defer g.wg.Done()
		defer g.crashOnPanic(ctx, name)
		addActive(ctx, name, 1)
		defer addActive(ctx, name, -1)
		g.run(ctx, name, false, fn)
	}()
	return true
}

// Stop starts no new run or task, cancels the loops, and waits for every
// routine to return: runs and tasks in progress finish, and workers drain.
// Once ctx ends, it cancels those too, waits for them to return, and returns
// ctx's error. A second call waits as the first does. Stop before Start is
// harmless.
//
// Stop returns only once every routine has returned, so what they use can then
// be closed.
func (g *Group) Stop(ctx context.Context) error {
	g.mu.Lock()
	if !g.started {
		g.stopped = true
		g.mu.Unlock()
		return nil
	}
	if !g.stopped {
		g.stopped = true
		close(g.stopping)
		g.cancelLoops()
		go func() {
			g.wg.Wait()
			g.cancelDrain()
			close(g.done)
		}()
	}
	g.mu.Unlock()

	select {
	case <-g.done:
		return nil
	case <-ctx.Done():
		g.cancelDrain()
		<-g.done
		return ctx.Err()
	}
}

func (g *Group) every(ctx context.Context, r *routine) {
	wait := time.Duration(0)
	if !r.opts.immediately {
		wait = time.Duration(rand.Int64N(int64(r.interval))) //nolint:gosec // jitter, not security
	}
	for g.sleep(wait) {
		g.run(ctx, r.name, r.opts.recover, r.fn)
		wait = jittered(r.interval, r.opts.jitter)
	}
}

// sleep waits d, and reports false if Stop began first.
func (g *Group) sleep(d time.Duration) bool {
	select {
	case <-g.stopping:
		return false
	default:
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-g.stopping:
		return false
	case <-t.C:
		return true
	}
}

// errReturnedEarly is a loop's return before its context ended, without an
// error.
var errReturnedEarly = errors.New("returned before its context ended")

func (g *Group) loop(ctx context.Context, r *routine) {
	b := NewBackoff(r.opts.backoffMin, r.opts.backoffMax)
	addActive(ctx, r.name, 1)
	defer addActive(ctx, r.name, -1)
	for {
		start := time.Now()
		err := r.fn(ctx)
		if ctx.Err() != nil {
			return // stopped
		}
		if err == nil {
			err = errReturnedEarly
		}
		lasted := time.Since(start)
		countRun(ctx, r.name, outcomeError)
		g.logger.ErrorContext(ctx, "routine.failed", "routine", r.name, "outcome", outcomeError, "error", err.Error())
		if lasted > r.opts.backoffMax {
			b.Reset()
		}
		if b.Wait(ctx) != nil {
			return
		}
		countRestart(ctx, r.name)
	}
}

func (g *Group) worker(ctx context.Context, r *routine) {
	addActive(ctx, r.name, 1)
	defer addActive(ctx, r.name, -1)
	r.work(ctx)
	g.logCutShort(ctx, r.name)
}

// run runs fn once, and reports it: its outcome, its duration, and its time if
// it succeeded. With recoverPanic, a panic is its failure; otherwise it passes
// on to crashOnPanic.
func (g *Group) run(ctx context.Context, name string, recoverPanic bool, fn func(context.Context) error) {
	start := time.Now()
	outcome, err := g.call(ctx, name, recoverPanic, fn)
	recordRun(ctx, name, outcome, time.Since(start))
	if err != nil {
		g.logger.ErrorContext(ctx, "routine.failed", "routine", name, "outcome", outcome, "error", err.Error())
	}
	g.logCutShort(ctx, name)
}

func (g *Group) call(ctx context.Context, name string, recoverPanic bool, fn func(context.Context) error) (outcome string, err error) {
	if recoverPanic {
		defer func() {
			if p := recover(); p != nil {
				g.logger.ErrorContext(ctx, "routine.panic", "routine", name, "panic", fmt.Sprint(p), "stack", string(debug.Stack()), "recovered", true)
				outcome, err = outcomePanic, fmt.Errorf("panic: %v", p)
			}
		}()
	}
	if err := fn(ctx); err != nil {
		return outcomeError, err
	}
	return outcomeOK, nil
}

// logCutShort logs a routine whose context ended at Stop's deadline, before it
// returned on its own. Called before it returns, so before the group's own
// cancel after every routine has returned.
func (g *Group) logCutShort(ctx context.Context, name string) {
	if ctx.Err() != nil {
		g.logger.ErrorContext(ctx, "routine.cut_short", "routine", name)
	}
}

// repanic is panic, but for this package's tests, which cannot survive a
// crash.
var repanic = func(v any) { panic(v) }

// crashOnPanic logs a panic with its routine and stack, then panics again,
// crashing the process. Deferred directly, so that its recover sees the panic.
func (g *Group) crashOnPanic(ctx context.Context, name string) {
	if p := recover(); p != nil {
		countRun(ctx, name, outcomePanic)
		g.logger.ErrorContext(ctx, "routine.panic", "routine", name, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
		repanic(p)
	}
}

func jittered(d time.Duration, f float64) time.Duration {
	return time.Duration(float64(d) * (1 + f*(2*rand.Float64()-1))) //nolint:gosec // jitter, not security
}
