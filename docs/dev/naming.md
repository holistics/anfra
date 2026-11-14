# Naming

How we name things in anfra's code, and when a group of values deserves a type at all. One test runs through all of it: **read the name where it is read.** The established ideas these rules come from (stamp coupling, values over places, ubiquitous language, and others) are in [design patterns](design-pattern.md).

## Read the name where it is read

A name is read with what qualifies it: its package, its receiver, the struct it's a field of, its arguments. Go keeps names short by relying on that, so a name needn't be unique or meaningful on its own; asking that of every name ends in Java's names. What must be meaningful, and unambiguous, is the phrase a reader actually meets:

- **a type with its package:** `query.DataSourceQuery`, "a query in its data source";
- **a call with its receiver and arguments:** `executor.Submit(run)`, "submit the run to the executor"; `w.Write(p)`, "the writer writes p";
- **a path through fields:** `st.query.SQL()`, "the statement's query's SQL".

Read it aloud. If it says what it holds, or what happens, the names are right.

Context can make a name worse, not only better. A reader brings the meanings a word already has in that kind of code: in code that runs SQL, "prepared" means a prepared statement, so `query.Prepared`, a query in its data source, misled. It became `query.DataSourceQuery`.

Three ways a name reads wrong in its context:

- **It reads as a verb and as a noun.** In code that caches, `cacheResult(l, w)` reads as "cache the result" as easily as "the cache's result"; so do words like run, query, record and answer. A function that builds a value is `newCacheResult(l, w)`: "new" says it builds one, and the rest names what. A function that acts keeps its verb first, so it can't be read as a noun: `runAndCache`.
- **It doesn't say its object, where there are several.** In `package query`, which looks up data sources, datasets and cached results, `r.lookUp(…)` doesn't say where it looks; `r.lookUpInCache(…)` does. Likewise `readCacheEntry`, not `readEntry`.
- **A short name means something else close by.** A one-letter name is fine when nothing near it could be the same letter: `q` for the query run beside `st.query` read as the query itself, so the run became `r`. Judge it in every function it appears in, not only where it's declared.

## Types: a concept, not baggage

**A type is a concept: its fields mean something together.** `query.DataSourceQuery` is a query in its data source; `CacheResult` is what the cache did for a query. Its name says what it is, and a reader can tell what each field is for from the name alone.

**A bundle of values that is no concept is baggage**, however convenient passing it is: once it's passed along, every receiver gets every field, and no one can tell who needs what. Values used once, in order, within one function stay locals; a reader then sees where each comes from and where it goes, a few lines apart. Baggage is acceptable only

- **when it is local enough** that a reader sees it filled and used at once,
- **or when no cleaner concept exists**, as for a call's many inputs. Then its name says it is a bundle: `QueryRequestParams`, `localquerycache.Options`, `querycache.KeyInput`. A reader then knows not to look for a meaning beyond "what this takes".

Baggage that isn't named as such shows itself in the paths a flow builds through it: `statement.prepared.st` meant nothing as a phrase, though each link had made sense where it was written. A reader then opens every type in the chain to learn what it holds, and finds they aren't concepts, only containers passed along. Fix the types, not the names. In the query cache's run, `statement` carried anfra-node's whole compile result, which compiling, announcing and the cache each read a part of at a different time; it became the four fields the cache needs, and the compile result stayed a local of the one function that needs all of it.

A type may be named for a state (`Pending`, `Committed`) when the state is clear in its context.

### Values by default; state only where a flow needs it

A struct is a value by default: built whole, in one place, passed and copied, never changed after. `statement`, `CacheResult` and `query.DataSourceQuery` are values; `q.WithLakeKey(key)` returns a changed copy rather than changing `q`. Don't create a value half-filled and complete it later through a pointer: code between the two steps has to know which fields are missing yet.

A struct that changes as a flow runs, a state, is the exception. Use one only when both hold:

- **It is local to one flow:** made and used within a function, the helpers it calls and the callbacks it hands out, and never stored or handed out beyond them.
- **Passing values instead would hurt:** the flow is driven by hooks or callbacks that can't return values to each other (a stream handler's `Header` and `Row`, called by someone else), or threading the values through would make the flow harder to read, or measurably slower where speed matters.

`caching` is such a state: `runCached` makes it, canal-query's handler callbacks record the header and the rows' count and size into it as they stream, and nothing outside `cache.go` sees it. So is the tracker that times a run's steps from its progress callbacks.

Within a state, **record what happened, and derive a result from it once.** `caching` records facts (the key, the entry hit, the write committed, a warning), and `answer()` builds the `CacheResult` value from them at the end. When several steps each edit a result instead, and some undo what others wrote, a reader has to replay them all to know what it holds.

## Functions and methods: does it act, or answer?

A function that **acts** (changes state, calls out to something else, may be refused or fail) is a verb, named for the operation, not for what it returns: `executor.Submit(run)` returns an execution, but isn't `CreateExecution`, since it hands the work over, and that may be refused or held. A name taken from the result hides what happens on the way. `c.Open(ctx)` opens the store, creating or migrating it, so it's named for the opening, though it returns the store.

A function that only **answers** (computes a value from its arguments, with no effect) is named for that value, a noun or an adjective: `Len`, `Owner`, `answer()`, `filled(seeds)`, `CacheReadQuery(lakeKey)`. A copy with one thing changed is `WithX`, as everywhere in Go: `q.WithLakeKey(key)`. Don't use `WithX` for something that acts: `InDataSource(ctx, sources, compiled)` asks the host for the data source, which can refuse it, so it isn't `WithDataSource`.

Some functions sit between the two: they look something up and change nothing, but can fail. Name them for what they answer when that's what the caller cares about, and let their `ctx` and error say that something is asked: `InDataSource` answers the query in its data source. Avoid a verb that promises an effect they don't have: `Connect` would have suggested a warehouse connection is opened, which canal-query does, later.

A method is a message to its receiver, and the verb supplies the roles; the receiver may be the actor (`w.Write(p)`, `client.Send(msg)`) or what is acted on (`pool.Submit(task)`, `stack.Push(x)`). Both are idiomatic; the call site says which, read aloud.

**A hook is named for what the host does.** A hook is a function field a host fills in and the framework calls: `Mounting.Admit`, `Mounting.Authorize`, `Mounting.Bind`. There's no receiver to read the roles from, and the reader is the host writing it, so it says what the host does when called: it admits the caller, authorizes the call, binds the caller to an invocation. `Mounting.Submit` would read backwards: the host being submitted to, among fields that are each the host acting.

## Families: one name, one shape

- **One action keeps one name across layers.** `io.Reader.Read` on a file, a buffer and a gzip reader; `http.Handler.ServeHTTP` on each middleware, calling the next's. A run is executed by `Page` or `Stream`, on the op's `Run` and on the executor's `Execution` that wraps it. A new name per layer (`read`, `deliver`, `perform`) hides that it is one action, and invites the question of what each means.
- **Siblings share a shape.** `InDataSource` and `InDataSourceUngoverned` do one thing in two modes, so they read alike; `Prepare` beside `ConnectUngoverned` made them look unrelated.
- **A renamed thing takes its namesakes with it:** its variables, receivers, comments and tests. A `p` left from `Prepared` misleads as much as the old name did.

## Go's own conventions

Beside these, what Go's style guides say (Effective Go, Code Review Comments):

- no `Get` prefix on a getter: `Owner()`, not `GetOwner()`;
- a one-method interface is the method plus "-er": `Reader`, `Writer`;
- MixedCaps; short names; no repeating the package's name (`jobs.Service`, not `jobs.JobsService`).

## Before you add a name or a type

1. Read the phrases it will appear in aloud: `pkg.Type`, `receiver.Method(args)`, `x.field.field`. They should say what they hold, or what happens, with the meanings a reader of that code brings: one reading only, its object named, and no neighbor it could be taken for.
2. Does the function act, or only answer? Name the operation, or the value.
3. Does it have siblings, or the same action in another layer? Give it their shape, and their name.
4. Do its fields mean something together? If the only honest name for it is "what this function needs", use locals instead, or, when there's no cleaner concept, name it as the bundle it is (`…Params`, `…Options`, `…Input`).
5. Is it a value? If it must change as a flow runs, is it local to that flow, and would passing values instead really hurt?
