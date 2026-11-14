# Design patterns

The established ideas behind how we shape anfra's code, each with what it means here. [Naming](naming.md) is where they become rules; this doc is where they come from, so a rule can be traced to its reason, and argued with.

## Baggage: what passing too much costs

**Stamp coupling and tramp data** (Yourdon and Constantine, *Structured Design*). Stamp coupling is passing a whole structure to code that uses only part of it; tramp data is data passed through code that doesn't use it, only to reach code that does. Both tie code to more than it needs, and hide what it does need. In anfra: a query's statement once carried anfra-node's whole compile result, which three steps each read a different part of. See [types: a concept, not baggage](naming.md#types-a-concept-not-baggage).

**Pass-through variables** (Ousterhout, *A Philosophy of Software Design*). The same cost, in a modern form: values threaded through layers that don't use them. Ousterhout warns that the usual cure, a context object holding everything, brings the problem back under one name. In anfra: that is why a bundle with no concept behind it is named as a bundle (`…Params`, `…Options`), so no one takes it for one.

**Data clumps, and Introduce Parameter Object** (Fowler, *Refactoring*). Values that always travel together may become a parameter object, but Fowler counts that only as a first step: it pays once the object is a real concept, with behavior of its own. In anfra: a type is a concept, whose fields mean something together; until it is one, it is a named bundle, or locals.

## Values, and where state belongs

**The value of values** (Rich Hickey, "The Value of Values", "Simple Made Easy"; data-oriented programming, Sharvit). Prefer immutable values to places that change: a value can be passed, copied and compared without anyone wondering who else changes it. Hickey calls the opposite place-oriented programming. In anfra: a struct is a value by default, built whole and never changed after; `WithLakeKey` returns a copy. See [values by default](naming.md#values-by-default-state-only-where-a-flow-needs-it).

**Functional core, imperative shell** (Gary Bernhardt). Compute in pure values, and keep effects and changing state at the edges. In anfra: where a flow needs state (callbacks that can't return values to each other), it stays local to that flow, records facts, and derives its result once: the query cache's `caching` records what happened as rows stream, and `answer()` builds the result from it.

**Value objects** (Evans, *Domain-Driven Design*). An object defined only by its attributes, immutable, replaced rather than changed. In anfra: `query.DataSourceQuery`, `CacheResult`.

**Make illegal states unrepresentable** (Yaron Minsky), **Parse, don't validate** (Alexis King). Build a value whole, so no code ever meets it half-made, and let its type carry what was checked. In anfra: a value is made in one place, never created half-filled and completed later through a pointer; `UngovernedSQL` exists only through the function that checks the caller may run SQL as written.

## Names

**Ubiquitous language, within a bounded context** (Evans, *Domain-Driven Design*). One word for each concept, used the same way everywhere it applies, and meaningful within its context rather than globally. Evans applies it to a business domain; we apply it to the code's own concepts. See [read the name where it is read](naming.md#read-the-name-where-it-is-read) and [families](naming.md#families-one-name-one-shape).

**Go's short, qualified names** (Andrew Gerrand, "What's in a name?"; Go Code Review Comments). A name is read with its package and receiver, so it can be short; its length grows with the distance between where it's declared and where it's used. In anfra: judge the qualified phrase, `query.DataSourceQuery`, not the bare word.

**Command–query separation** (Bertrand Meyer). A function either does something or answers something, not both. In anfra: one that acts is named for the operation, one that only answers for its value. See [does it act, or answer?](naming.md#functions-and-methods-does-it-act-or-answer).

**Messages to objects** (Smalltalk). A method call is a message to its receiver, and the verb supplies the roles: the receiver may be the actor (`w.Write(p)`) or what is acted on (`pool.Submit(task)`). See [naming](naming.md#functions-and-methods-does-it-act-or-answer).

## Where we differ

- **Domain-Driven Design** models a business domain with entities, aggregates and repositories. We take only its discipline about words and values, and apply it to the program's own concepts, as Ousterhout does.
- **Hickey's values** usually come as dynamic maps of data. We keep Go's static structs, and ask each one to be a concept, or to say that it's a bundle.
