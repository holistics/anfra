# A result says where it came from, and the server decides how much it may say

`QueryResult.debug` carries `executedAql`, `executedSql`, `fromCache` and `executedAt`. The two
query texts are blanked server-side for readers without `canViewGeneratedSql`. The SDK does not
re-check that permission, and does not synthesise any of these fields.

## Why provenance is part of the result rather than a separate call

The question provenance answers is "why does this number look like that", and it is asked while
looking at the number. A separate endpoint would mean the answer could disagree with what is on
screen — a second execution, a different cache state, a different reader — which is the one thing a
debugging surface must not do. Carried on the result, they are the same run by construction.

It also costs nothing to carry. `executedAql` and `executedSql` are strings the server already has
at the moment it answers.

## Absence means "not permitted", and that has to be a rule

`executedAql` and `executedSql` are optional, and there are two reasons a consumer might find them
missing: the reader is not allowed to see generated SQL, or there genuinely was none. Those need to
be distinguishable, because a panel that renders "no SQL" for a permission failure teaches its
reader something false about the query.

The rule is that absence means not permitted. Blanking happens server-side in `DebugInfo`, and the
SDK passes through whatever arrived without inspecting it.

**The SDK does not re-check the permission client-side**, even though `User.permissions` carries
`canViewGeneratedSql` and the check would be one line. A second check can only agree redundantly or
disagree wrongly, and when it disagrees the client is the one that is wrong — `User` is a
provisioned snapshot, the server's answer is current. The permission is in `User` for rendering an
affordance, not for deciding what arrived.

## `fromCache` is the caller's claim, not an inference

`fromCache` means "these rows were not produced by this request". Nothing in the SDK can determine
that, and nothing in the response shape implies it, so it must be supplied honestly by whichever
server path produced the result.

`executedAt` follows from it and is the reason it matters. On a cached result, `executedAt` is when
the *cached* run happened, which may be hours earlier. A surface that shows it as though the query
just ran tells a reader their figures are current when they are not — so the value is labelled by
`fromCache` rather than shown bare. The figures are this reader's own, correctly permission-filtered,
and still potentially stale; those are independent properties.

## Consequences

- **This is a permission the SDK surfaces twice, for two purposes.** `User.permissions.canViewGeneratedSql`
  says whether to draw the affordance; the presence of `executedAql` says whether there is anything
  behind it. They can disagree in the window between provisioning and a permission change, and when
  they do the result wins.
- **`debug` is optional on `QueryResult`.** A host that never renders it pays nothing, and a
  transport that does not supply it is not an error.
- **It is projected into the Inspect payload** (ADR 0007) rather than read live, with the same
  staleness caveat as everything else there.
- **An honest `fromCache` is load-bearing and unverifiable from this side.** If a server path
  hardcodes it, every surface downstream reports cache state wrongly and nothing in this package can
  detect it.
