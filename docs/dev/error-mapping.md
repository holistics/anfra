# Mapping an operation's error to the one its caller sees

How to decide, in one place, which error an operation's caller is answered.

## The problem

The error an operation returns is not always the one its caller should see. Which one they
should see depends on what the operation itself does not know: the state around it, the host's
naming, what happened to it from outside. Decided inline, at each place the operation's answer
is returned, one decision is spread over several places, and they diverge.

## The pattern

One function per operation boundary takes the operation's error — and whatever state decides
it — and returns the error its caller sees:

```go
// mapRunError is the error a run's caller sees, given err, the one its work
// returned on ctx: … (what it maps, and why: the function's own decision)
func mapRunError(ctx context.Context, err error) error
```

## Rules

1. **Nil passes through.** It maps errors; it never makes a success a failure.
2. **What it does not map, it returns unchanged** — no wrapping, no reclassifying an error it
   has no decision about.
3. **One function, called at every exit.** Every path that answers the operation's caller goes
   through it, not an inline check repeated at each.
4. **It lives with the code that owns the caller's answer**, so that a new mapping joins it
   rather than appearing elsewhere.
5. **Named `map<Operation>Error`**, as the standard library names the pattern. Its doc says what
   it maps to what, and why: that decision is the function's own, and its doc is where it is
   recorded.
6. **Its test** covers each mapping, and nil and an unmapped error passing through unchanged.

Which error a caller should see — a cause, a host's code, something else — is not this
guideline's to say: each mapping decides it, on its own grounds.

## Precedent

- `net/http`: `(*persistConn).mapRoundTripError` "returns the appropriate error value for
  persistConn.roundTrip".
- apikit: `withTimeout` maps an op's error to the host's timeout error (`Runtime.Timeout`) when
  its deadline passed.
- anfra-cloud: `mapRunError` (`internal/bi/jobs`) maps a run's error to its cancel's when it was
  stopped.
