# Background routines

> **Built, 2026-10-19,** as `shared/routine`; `anfra serve`'s routines run on it.

A server runs work beside its requests: a sweep every few minutes, a listener waiting on a
database, a worker draining a queue. This doc says how anfra and the platforms that embed it
run such work: which kinds there are, when a goroutine is the right home for it, how each
starts, fails and stops, and what it reports without being asked.

The helper is in `shared/` because both anfra and the platforms that embed it need it.
`anfra serve` sweeps its export files, checks for updates and watches a repo for Data Apps; a
platform adds its own heartbeats, sweeps and listeners. A convention that only one side follows
is not one.

## The rules

These hold for every goroutine, with or without the helper:

1. **Every goroutine has an owner that stops it and waits for it.** Nothing is started
   fire-and-forget. The owner is a request, which waits before it returns, or a group, which
   waits in its `Stop`.
2. **It returns once its context ends**; a worker, once its input is closed and drained, or at
   the latest once its context ends. A blocking call inside it takes that context, or is
   bounded.
3. **It is stopped before what it uses is closed.** A routine that writes to the store stops
   before the store closes. Owners stop in the reverse order they started.
4. **A failure is retried on a schedule, never by a tight loop.** The helper retries a failed
   run: a periodic routine on its next tick, a loop by restarting it with backoff. The routine
   retries a failed item itself, with backoff (see [Who retries what](#who-retries-what)).
5. **A panic crashes the process, unless one iteration's work is all it can have corrupted.**
   See [Panics](#panics).
6. **It reports its health without being asked**: its runs, failures and last success, through
   the helper (see [What it reports](#what-it-reports)).
7. **Tests check that nothing leaks**, with `goleak` on each package that starts routines.

Rules 1 and 2 are Go's own guidance ([Goroutine lifetimes](https://go.dev/wiki/CodeReviewComments#goroutine-lifetimes);
Dave Cheney, [Never start a goroutine without knowing how it will stop](https://dave.cheney.net/2016/12/22/never-start-a-goroutine-without-knowing-how-it-will-stop));
rule 7 is the [Uber Go style guide](https://github.com/uber-go/guide/blob/master/style.md)'s.

## Kinds of routine

Two questions tell the kinds apart:

1. **Who waits for it to end?** The call that started it (request-scoped); the server, which
   waits for it when it stops (task, periodic, loop, worker); or nobody, because it must
   outlive the process (durable job).
2. **For work the server owns, what starts each piece of it?** One trigger (task), the
   helper's clock (periodic), the routine itself, as events arrive or on a schedule of its own
   (loop), or a queue inside the process that other code feeds (worker).

| Kind | Examples | Lives as long as | When it fails | Runs as |
|---|---|---|---|---|
| **Request-scoped** | A stream's pings; waiting for a sidecar to exit; the parallel parts of one computation | The request or function that starts it, which waits for it | Its error is the request's | A plain goroutine, under the function's `WaitGroup` or `errgroup` |
| **Task** | Publishing a Data Apps change event once file changes settle; releasing a cancelled query's resources after its request has returned | Until done; refused once its group stops | Logged and counted | `Group.Task` |
| **Periodic** | `anfra serve`'s export sweep, update check and idle check; an instance's heartbeat | The server | The iteration is counted failed; the next tick retries | `Group.Every` |
| **Long-running loop** | The Data Apps file watcher, so a preview reloads as a file is saved; a `LISTEN` on a Postgres channel, so queued work starts at once | The server; it runs until its context ends | Its source breaks: it returns an error, and is restarted with backoff | `Group.Loop` |
| **Worker** | A batch span processor exporting queued spans; a write-behind buffer that batches records into a store | Until its owner closes its input and it has drained | Per item, retried by the worker itself | `Group.Worker`; `Stop` drains it |
| **Durable job** | A scheduled export; a retention run that must finish | Across restarts | Retried from its record | Not a goroutine: a platform's job system |

**Request-scoped**: concurrency inside one call. The call starts the goroutines and waits for
them before it returns, so they never outlive it, and nothing outside the call sees them. It is
not background work, and needs no helper.

**Task**: one piece of work, triggered once, that the call that triggered it does not wait for.
This is what a bare `go f()` usually means; the group gives it an owner, which waits for it at
`Stop`. Unlike request-scoped work, nobody waits for its result. Unlike a durable job, losing
it in a crash costs nothing that matters. A task is the one kind whose work the group itself
takes in: each call to `Task` hands it one piece, which it runs on a goroutine of its own. When
pieces must queue, be batched or be drained at stop, they need a worker instead.

**Periodic**: driven by the clock. Each run is a short call that looks at the current state,
does what is due and returns, holding nothing until the next run. Because each run finds its
own work, a missed or failed run costs only a delay.

**Long-running loop**: one run that lasts for the server's life and decides for itself when it
next wakes, finding its work in a source outside the process. It is one of two things:

- **A listener.** It holds the source open and wakes when the source delivers an event, so it
  reacts at once where a periodic routine would react within an interval. Use one when that
  delay would show, and polling often enough to hide it would cost too much. The Data Apps
  file watcher reloads a preview as a file is saved; a `LISTEN` on a Postgres channel starts
  queued work as soon as another instance queues it.
- **A poller that paces itself.** It decides each time how soon to look again: at once while it
  keeps finding work, backing off while it finds none. A poller on a fixed interval is `Every`.

**The helper restarts it when `fn` returns an error.** A loop's failures are mostly outages of
its source: a long-held connection drops when Postgres fails over or a proxy closes it as idle;
a file watcher's queue overflows during a large `git checkout`. Such outages pass, and
reopening the source is all the recovery a loop needs, since its work is recorded outside the
process. Without the restart, the instance would look healthy while never hearing of new work
again. Write `fn` as "open the source, catch up, then
consume", so a restart recovers what was missed through the same path as startup.

Events that arrive while the source is closed are lost, so the work a loop triggers must also
be found another way: by catching up when the source reopens, or by a slower periodic routine.
If a missed event costs only a delay, a periodic routine alone is simpler.

**Worker**: driven by a queue inside the process that other code feeds. Producers hand it items
and move on; it handles them at its own pace, often in batches. Its owner, not the helper,
provides how items are handed in (a `Submit`) and how the input ends (a `Close`):

```go
type writeBehind struct {
	in    chan record
	store *Store
}

func (w *writeBehind) Submit(r record) { w.in <- r }
func (w *writeBehind) Close()          { close(w.in) }

// work handles w.in until Close is called and w.in is empty, retrying a failed
// write with backoff until the store is back. See How Stop drains.
func (w *writeBehind) work(ctx context.Context)
```

An accepted item exists only in this process's memory, so an abrupt stop would lose it. A
worker therefore stops by **draining**: its owner closes its input once its producers have
stopped, and the worker handles what is left and returns. That return is its normal end, and
may come before the group's `Stop`. It returns before draining only if `Stop`'s deadline
passes, when its context is cancelled and it drops what remains. Any other return is a bug the helper cannot
tell from a drain, because it does not see the input close; the owner can, for instance by
failing a `Submit` once the worker has gone.

Nothing outside the process can end a worker's run. Its input is a channel that does not
break. Its destination may fail, as a store does during a failover, but that fails a write,
not the worker, and the worker retries the batch it holds. A restart would have nothing to
reopen and nothing outside to catch up from, and would throw that batch away. So a worker is
never restarted.

The test between a loop and a worker: if an abrupt stop would lose something only this process
knew about, it is a worker. Names elsewhere blur this. A "worker" that polls a database table
for jobs is a loop here, because the table, not the process, holds the work.

**Durable job**: work recorded before it runs, so that it survives its process: another process
picks it up after a crash or a deploy. It is not a goroutine.

### Limits

- **A periodic routine's interval is minutes to about an hour**, and well under the time
  between deploys. Its timer lives in the process, so a restart resets it: with a staggered
  start, a routine whose interval outlasts the process may never run; with `Immediately()`, it
  runs on every deploy instead. Work on a calendar ("monthly", "the 1st at 02:00") or a longer
  interval runs often, reading from durable state what is due (see
  [Where a routine runs](#where-a-routine-runs)). At the other end, an interval of a second or less is usually polling for
  promptness, which a listener gives instead.
- **A periodic run or a task finishes within `Stop`'s deadline.** Longer work is a durable job.
- **A worker's queue is bounded**, since its items live in memory.

### References

Others sort background work much as this doc does:

- ASP.NET Core's [Background tasks with hosted services](https://learn.microsoft.com/en-us/aspnet/core/fundamentals/host/hosted-services)
  has *timed background tasks* (periodic) and *queued background tasks* (worker: a queue that
  requests feed and one consumer drains). A `BackgroundService`'s `ExecuteAsync` "represents
  the entire lifetime of the background service": a loop's shape.
- Azure's [Best practices for background jobs](https://learn.microsoft.com/en-us/azure/architecture/best-practices/background-jobs)
  sorts jobs by trigger, *event-driven* or *schedule-driven*, as this doc's second question
  does. On shutdown, a job "should stop accepting new work, finish or checkpoint the current
  work item, and exit cleanly", as `Stop` drains.
- Spring's [Task Execution and Scheduling](https://docs.spring.io/spring-framework/reference/integration/scheduling.html)
  has `@Scheduled` (periodic; `fixedDelay` is measured "from the completion time of each
  preceding invocation", as `Every` is) and `@Async` (task).

By kind:

- **Request-scoped**: Nathaniel J. Smith, [Notes on structured concurrency, or: Go statement considered harmful](https://vorpus.org/blog/notes-on-structured-concurrency-or-go-statement-considered-harmful/),
  the case for goroutines that never outlive the call that starts them.
- **Task**: CockroachDB's `Stopper.RunAsyncTask`, which refuses new tasks once the stopper
  quiesces ([stopper.go](https://github.com/cockroachdb/cockroach/blob/master/pkg/util/stop/stopper.go)).
- **Loop, as a listener**: Kubernetes' watch ([Efficient detection of changes](https://kubernetes.io/docs/reference/using-api/api-concepts/#efficient-detection-of-changes)):
  a client lists, then watches from the list's version, and after a disconnect starts a new
  watch or lists again; client-go's `Reflector` does it. That is "open the source, catch up,
  then consume". Postgres delivers a [`NOTIFY`](https://www.postgresql.org/docs/current/sql-notify.html)
  only to sessions that have "previously executed `LISTEN`", so a closed source misses events.
- **Worker**: client-go's [`workqueue`](https://pkg.go.dev/k8s.io/client-go/util/workqueue),
  whose `ShutDownWithDrain` waits for items in progress; OpenTelemetry's [`BatchSpanProcessor`](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/trace),
  whose `Shutdown` exports what is queued.
- **Loop and Worker together**: Erlang/OTP's [supervisor](https://www.erlang.org/docs/22/man/supervisor.html)
  sets each child's `restart` (`transient`: restarted only after an abnormal exit, as a loop;
  `temporary`: never, as a worker) and `shutdown` (`brutal_kill`, as a loop is cancelled; a
  timeout, as a worker is drained).

## Where a routine runs

The helper keeps nothing: a routine's timer, its backoff and a worker's queue live in the
process, and go with it. Whatever must survive a restart is the routine's own concern, kept in
a store by its `fn`. That decides where work can run: in the server as a routine, as a durable
job, or in a process of its own.

### In the server

Work can run as a routine when a restart costs it nothing:

- **Each run reads what is due from durable state**, and brings that state closer to what it
  should be. A missed run, or a restart, costs a delay; the next run sees the same state.
  - *Partitions ahead*: "partitions exist through two weeks from now". An hourly run creates
    what is missing; the two weeks cover a long outage.
  - *Retention by dropping partitions*: "no partition lies wholly before the cutoff". Retention
    sounds monthly, but an hourly check keeps it true.
  - *Work at a time*, such as a report on the 1st: once each occurrence is recorded, it is a
    state too, "every occurrence due by now has run", which an hourly run can check.
- **It belongs to this process**: waking its waiters, pinging its streams, sweeping its own
  local files.
- **It belongs to one request**, which fails with it.

Work whose only memory of what is due is a timer in the process does not fit: a restart loses
it.

Work that must run **once across several instances** can still run on every instance, provided
each run is idempotent, or claims its work in a shared store (`FOR UPDATE SKIP LOCKED`, or an
advisory lock in Postgres).

### As a durable job

Each piece of work is a record that a platform's job system runs, when any of these hold:

- it **must finish** once started, or must not run twice;
- it can **outlast the drain** of a deploy;
- its **result is seen by a user**, who must be able to tell that it failed.

Users' scheduled jobs, such as a scheduled export, split in two:

- **The scheduler** decides what is due. It reads each schedule's next run from the store,
  claims the due ones, and in one transaction queues a durable job and advances the next run.
  That is a periodic routine in the server, on every instance, each claim taken once.
- **The job** runs the work, in the job system, as any durable job does.

So scheduled jobs need no scheduler process. Whether their jobs run in the server or in a
process of their own is decided by resources, as for any work (below).

### In a process of its own

A separate process, such as a Kubernetes CronJob or a deployment of workers, is for resources
and availability, not durability. Work moves there when:

- it would **slow requests**, with heavy CPU, memory or I/O;
- it **scales on a signal of its own**, such as a queue's depth rather than requests;
- it must **run while the server does not**, as when the server scales to zero or stops when
  idle;
- it needs **credentials** the request path should not hold.

For example:

- *A scheduled export to PDF* renders in a headless browser: heavy, crash-prone, and scaled on
  the exports waiting. Renderers run as a deployment of workers taking export jobs.
- *Embedding every organization's models for search*: long and CPU-heavy, scaled on its
  backlog, and needing a provider's key and access across organizations.
- *A scheduled CSV export* stays in the server: the warehouse runs the query, and the server
  only streams rows, as it does for an interactive export.

In each, the scheduler that queues the jobs stays in the server.

A process of its own does not make work durable: crontab misses a run while its host is down,
as `Every` does on a restart. A CronJob that must not miss a run queues a durable job.

### Cost

Cost settles each of these:

- **A run's cost sets its interval.** Checking state often is right only when the check is
  cheap: an hourly query of the catalog costs nothing; an hourly scan of a large table does
  not. An expensive check runs less often, or keeps a record or an index that makes it cheap.
- **That cost, times the instances, sets where it runs.** Cheap and idempotent runs on every
  instance. Expensive runs on one, by a claim, or in a process of its own.
- **A process of its own costs a deployment**, with its configuration and its monitoring. So a
  routine starts in the server, and moves out only when one of the reasons above pays for
  that.

## Panics

A panic means a goroutine stopped partway, in a state nobody planned for: a mutex still held,
a map half updated. Infrastructure written in Go treats a panic in its own loops as fatal:

- Kubernetes' `wait.Until` logs and re-panics
  ([`runtime.HandleCrash`](https://github.com/kubernetes/apimachinery/blob/master/pkg/util/runtime/runtime.go);
  `ReallyCrash` is false only in tests).
- CockroachDB's `Stopper` reports and re-panics
  ([stopper.go](https://github.com/cockroachdb/cockroach/blob/master/pkg/util/stop/stopper.go)).
- River's maintenance services do not recover at all
  ([job_cleaner.go](https://github.com/riverqueue/river/blob/master/internal/maintenance/job_cleaner.go)).
- `errgroup` deliberately does not recover, because hiding a panic "hides it from crash-monitoring
  tools" and "risks deadlocks"
  ([errgroup.go](https://github.com/golang/sync/blob/master/errgroup/errgroup.go)). The Go team
  regrets that `net/http` recovers handler panics
  ([golang/go#25245](https://github.com/golang/go/issues/25245)).

They recover only at the edge of one unit of work, whose state can be thrown away:

- controller-runtime recovers each reconcile and counts it
  ([`reconcile_panics_total`](https://github.com/kubernetes-sigs/controller-runtime/blob/main/pkg/config/controller.go)).
- River recovers a job's own code and retries the job.
- Sourcegraph turns a panic in one iteration of a periodic routine into that iteration's error
  ([periodic.go](https://github.com/sourcegraph/sourcegraph-public-snapshot/blob/main/internal/goroutine/periodic.go)).

So:

- **By default, a panic crashes the process.** The helper first logs it with the routine's name
  and stack, then re-panics. The orchestrator restarts the process from a clean state, and
  whatever was running on it fails as it would on any crash.
- **`Recover()` makes one routine recover per iteration**, counting each panic and continuing.
  Use it only for a routine whose iteration holds no state outside its own call: one that
  works through a database transaction, which a panic rolls back, and touches no shared
  memory. A sweep of database rows qualifies. A listener that wakes in-memory waiters does not.

## The helper: `shared/routine`

A `Group` owns a set of routines, starts each, and stops them together:

```go
g := routine.NewGroup("serve", logger) // the group's name, in its log lines

g.Every("serve.export_sweep", exportLinkLife/6, sweepExports)
g.Every("serve.idle", time.Minute, stopIfIdle)
g.Every("serve.update_check", time.Hour, checkUpdates(logger), routine.Immediately())
g.Loop("listen", listen, routine.RestartBackoff(time.Second, 30*time.Second))

g.Start(ctx) // ctx: values only (logger, tracer); its cancellation is ignored
...
g.Stop(ctx)  // cancels its loops, lets the rest finish until ctx ends; idempotent
```

A group with a worker stops after the worker's producers, and after its owner closes its input:

```go
srv.Shutdown(ctx) // requests, the producers, stop
wb.Close()        // the worker's owner closes its input
g.Stop(ctx)       // cancels the loops; waits for the worker to drain
```

What the helper does:

| | Calls `fn` | When `fn` returns an error | On `Stop` | On a panic |
|---|---|---|---|---|
| `Every(name, interval, fn)` | Every interval, with jitter, measured from the end of the last run, so runs never overlap | Logged and counted; the next tick runs as usual | Starts no new run; lets the current one finish | Crash, or with `Recover()`, counted as a failed run |
| `Loop(name, fn)` | Once, then again after backoff each time it returns before its context ends | Logged and counted; restarted after backoff | Cancel, cutting a backoff short | Crash |
| `Worker(name, fn)` | Once; never again after it returns | — (`fn` has no error to return) | **Drain**: lets it finish its input | Crash |
| `Task(name, fn) bool` | Once, when `Task` is called; refused once `Stop` has begun | Logged and counted | Lets running tasks finish | Crash |

`Stop` cancels only what never ends on its own, a loop, and lets the rest finish. Whatever is
still running when `Stop`'s own context ends is cancelled then. In every case `Stop` returns
only once each `fn` has returned, so what they use can then be closed.

What `fn` must do:

| | Signature | Returns | Must |
|---|---|---|---|
| `Every` | `func(ctx) error` | Once it has done what is due | Return promptly once its context ends; with `Recover()`, touch no shared memory |
| `Loop` | `func(ctx) error` | When its context ends, or with an error when its source breaks | Open its source, catch up, then consume; return an error only for an outage of its source, and handle a failed event itself |
| `Worker` | `func(ctx)` | Once its input is closed and drained, or when its context ends | Handle every failure itself, having no error to return; return for no other reason |
| `Task` | `func(ctx) error` | Once its work is done | Return promptly once its context ends |

Options:

- `Immediately()`: run the first iteration at `Start`, rather than one interval later.
- `Jitter(f)`: the jitter on `Every`'s interval, ±10% by default.
- `RestartBackoff(min, max)`: `Loop`'s restart delay, exponential with jitter, reset once a run lasts
  longer than `max`.
- `Recover()`: recover per iteration, as above.

`NewBackoff(min, max)` gives the same policy as `RestartBackoff` as a value, for a routine
that retries its own items: `b.Wait(ctx)` sleeps the next delay (`b.Next()` is the delay
itself), and `b.Reset()` follows a success.

An option given to a kind it does not apply to panics at registration, as does registering a
routine after `Start`.

### Loop and Worker

Both run one long call. The helper treats them differently in two ways only: whether a return
restarts it, and whether `Stop` cancels it at once or lets it drain.

| | `Loop(name, fn)` | `Worker(name, fn)` |
|---|---|---|
| **`fn`** | `func(ctx) error` | `func(ctx)` |
| **When `fn` returns** | Before its context ends: logged, counted, and restarted after backoff, a wait that `Stop` cuts short | Finished, whenever that is; whether it was early is the owner's to know |
| **On `Stop`** | **Cancel**: its context ends at once | **Drain**: waits for it to return; cancels it only once `Stop`'s deadline passes |
| **Metrics** | `restarts`, `active` | `active` |

A loop returns before `Stop` only if its source broke, since nothing else ends its context. If
the server's shutdown closed that source, as by closing the database before `g.Stop`, that
breaks rule 3; the loop's restarts and errors show it.

### How Stop drains

A group holds two contexts, and a signal to start no new work. `Stop` cancels the loops'
context at once. It cancels the other, which every other routine gets, only once `Stop`'s own
context ends:

```go
type Group struct {
	loopCtx, drainCtx        context.Context
	cancelLoops, cancelDrain context.CancelFunc
	stopping                 chan struct{} // closed by Stop: start no new run or task
	wg                       sync.WaitGroup
	// ...
}

func (g *Group) Start(ctx context.Context) {
	base := context.WithoutCancel(ctx) // keeps ctx's values, not its cancellation
	g.loopCtx, g.cancelLoops = context.WithCancel(base)
	g.drainCtx, g.cancelDrain = context.WithCancel(base)
	for _, r := range g.routines {
		ctx := g.drainCtx
		if r.kind == loop {
			ctx = g.loopCtx
		}
		g.spawn(ctx, r) // under g.wg
	}
}

func (g *Group) Stop(ctx context.Context) error {
	close(g.stopping) // Every starts no new run; Task refuses
	g.cancelLoops()

	done := make(chan struct{})
	go func() { g.wg.Wait(); close(done) }()
	select {
	case <-done: // runs, tasks and workers finished
		g.cancelDrain()
		return nil
	case <-ctx.Done(): // the deadline: cancel what is left
		g.cancelDrain()
		<-done
		return ctx.Err()
	}
}
```

`Every` waits between runs on `stopping`, not on its context, so a run in progress keeps a live
context and finishes:

```go
func (g *Group) every(ctx context.Context, r routine) {
	for {
		select {
		case <-g.stopping:
			return
		case <-time.After(r.next()):
		}
		r.run(ctx) // ctx is drainCtx
	}
}
```

The worker's side is the select in its own loop: it returns when its input closes and empties,
or when its context ends, dropping what remains:

```go
func (w *writeBehind) work(ctx context.Context) {
	for {
		select {
		case r, ok := <-w.in:
			if !ok {
				return // closed and drained: the normal end
			}
			w.write(ctx, r)
		case <-ctx.Done():
			return // Stop's deadline passed: drop what remains
		}
	}
}
```

If the owner never closes the input, the worker keeps waiting for more and returns only at the
deadline, dropping whatever is queued then. So `Stop` always returns by its deadline, but only
an owner that closes the input first gets a full drain.

### Who retries what

**`fn` returns an error only when starting over fixes the failure and loses nothing.**
Otherwise it handles the failure itself, retrying with `NewBackoff`, dropping or logging, and
carries on.

| | Return an error, and the helper… | Handle it inside `fn` |
|---|---|---|
| `Every` | Retries on the next tick. Fits most failures: the next run finds the work again | Only to carry on with the run's other items before returning |
| `Loop` | Restarts it after backoff. For an outage of its source, which reopening fixes | A failure of one event, which reopening would not fix |
| `Worker` | — (`fn` has no error to return) | Every failure: a restart would fix nothing and lose the batch in hand |
| `Task` | Logs and counts it; nothing retries a task | Whatever must be retried, since a task runs once |

Behaviour:

- **Start is staggered.** Each `Every` waits a random part of its interval before its first
  tick, unless `Immediately`, so instances started together do not run in lockstep (River's
  `StaggerStart`, Kubernetes' `JitterUntil`).
- **`Stop(ctx)` starts no new run or task**, cancels its loops, and waits for its runs, tasks
  and workers to finish. Once `ctx` ends, it cancels those too, waits for them to return, and
  returns `ctx`'s error. Calling it twice, or before `Start`, is harmless.
- **`Task` returns false** once `Stop` has begun, so the caller can do the work inline or drop
  it.
- **A `Group` stops as one.** A platform with several groups stops them in the reverse order it
  started them: after it stops accepting requests and closes its workers' inputs, and before it
  closes what they use.

**Names.** Other systems call the unit a *service* (Grafana's dskit, River, suture), a
*runnable* (controller-runtime), a *routine* (Sourcegraph) or a *task* (CockroachDB). They call
the container a *manager*, *supervisor*, *stopper* or *group*. The names chosen here avoid our
own vocabulary:

- **"Service" is taken.** Platforms that embed anfra name their domain components services (a
  `jobs.Service`, an `identity.Service`); "stop the jobs service" would become ambiguous.
- **"Job" is taken.** It names a durable job system's work, which a platform runs beside these
  routines. "Task" and "worker" are each only one of the kinds above.
- **"Group" means a set of goroutines started and waited for together,** as `errgroup.Group` and
  `oklog/run.Group` already do. A "supervisor" would promise restarts, which only `Loop` does.

Precedents for this surface, which it keeps smaller:

- River's [`startstop.Service`](https://github.com/riverqueue/river/tree/master/rivershared/startstop);
- Sourcegraph's [`PeriodicGoroutine`](https://github.com/sourcegraph/sourcegraph-public-snapshot/blob/main/internal/goroutine/periodic.go);
- Grafana's [dskit services](https://github.com/grafana/dskit/tree/main/services);
- CockroachDB's `Stopper.RunAsyncTask`.

## What it reports

Each routine reports through the global meter provider, as `apptracing` does through the global
tracer provider. Without one installed, the reports are no-ops. The `routine` attribute is
the routine's name: a fixed set, so it adds no cardinality as an installation grows.

| Metric | Kind | Attributes | Answers |
|---|---|---|---|
| `anfra.routine.runs` | counter | `routine`, `outcome` (`ok`, `error`, `panic`) | Is it running, and failing? |
| `anfra.routine.duration` | histogram, seconds | `routine` | Is it getting slow? |
| `anfra.routine.last_success` | gauge, Unix seconds | `routine` | Has it stopped succeeding? The alert: older than a few intervals |
| `anfra.routine.restarts` | counter | `routine` | Is a loop failing and restarting? |
| `anfra.routine.active` | up-down counter | `routine` | Is a loop or worker still alive? |

Logs:

- every failed run, at error level, with `routine`, `outcome` and the error;
- a panic, with its stack, before the process exits;
- a run, task or worker cancelled at `Stop`'s deadline, at error level, since it was cut short;
- each routine's start and stop, at debug level.

Traces: the helper starts no span itself, because most iterations of a frequent routine find
nothing to do, and a span for each would bury the ones that did. A routine's function starts
its steps with `apptracing` where it does work. Such a span has no parent, so it is the root
of a trace of its own, named by its step.

## Open

- A loop that runs only while something needs it, as the Data Apps file watcher runs only
  while a Data App is open: started and stopped many times in a server's life, where `Loop`
  is registered before `Start` and runs until `Stop`.
- Whether `Every` measures its interval from the end of a run (no overlap, as proposed) or from
  its start (a steady rate). Kubernetes' `JitterUntil` offers both, as `sliding`.
- Whether repeated identical failures are logged every time, or summarised after the first few.
- Whether `Stop`'s drain deadline is one for the whole group, as proposed, or one per routine.
- Whether the helper belongs beside `httpkit.Serve`, so that a server's drain and its groups'
  stops happen in one order that both repos share.
