# The environment arrives as a provisioned SDK instance

> **Amended by ADRs 0009 and 0010** in this fork: the global is `Anfra`, and `transport` is replaced by a required `backend`.

Author code never configures the SDK. The host builds a provisioned instance —
`createSdk({ datasets, user, transport?, features? })` — and exposes it to the sandbox as
`Holistics`. Authors and agents write `Holistics.createApp({})` and nothing else about setup.

`datasets` and `user` are both required and both checked at construction. `transport` and
`features` default.

## Why

Three earlier shapes were tried and rejected. Putting datasets on `createApp` conflates declaration
with environment, which breaks the `toJSON()` round-trip that persisting a declaration depends on.
An ambient `SDK.provide(env)` leaves a window in which author code can run before provisioning, and
puts mutable state in module scope. Injecting the environment after declaration (`app.provide(env)`)
defers validation to execute time, losing the synchronous "throws at the declaring line" behaviour
that both the error contract and AI authoring lean on.

A pre-provisioned instance has none of those problems: there is no unprovisioned window, no global
mutable state, tests build their own instance with fixture datasets, and what validation remains
stays synchronous because the descriptors are present before the first `createApp`.

## The user is part of the environment, reversing an earlier position

This ADR originally recorded the opposite — that the instance deliberately does not expose the
current user, because doing so invites apps to implement access control in JavaScript that the
server does not enforce. That concern was right and is unchanged. What was wrong was concluding
that the user therefore had no place in the environment at all.

The distinction the original position missed is between authorising and rendering. An app needs to
greet someone by name, and needs to not draw an export button for a reader who cannot export —
neither is an access decision, and both are impossible without the user. Meanwhile the thing that
actually enforces access is untouched: every query is re-authorised server-side as this person, so
an app that ignores `permissions` entirely renders a control that returns blank values rather than
one that leaks. Per-user *data* behaviour still belongs in permission rules and
`matches_user_attribute`, applied server-side at query time, exactly as before.

Two properties keep the reversal honest. `User` is an explicit allowlist — `id`, `name`, `email`,
`role`, `timezone`, and `permissions { canViewGeneratedSql, canExportData }` — written out field by
field rather than projected from the host's current-user record, which carries personal settings,
client IP, shared dataset ids and tokens that have no business in front of author code. Adding a
field to that list is a decision each time, because everything in it is readable by whatever code
the author wrote. And `timezone` is available without being a default: an app that declares no
timezone still sends none, and lets the server apply the tenant's.

Being required rather than optional is the other half. Author code needs no `await` and no null
check to use it, which is the same property that makes `datasets` worth provisioning.

## Consequences

- **Two entry points for two audiences.** `createSdk` is imported by the host and never seen by the
  author; `Holistics` is a bare global in the reporting environment, declared in a shipped ambient
  `.d.ts` — which doubles as the artifact an agent reads to learn the surface.
- **Named `Holistics`, not `SDK`**, because native embed will put this in someone else's page.
- **`Holistics.datasets` is readable**, so an agent can check field names before writing them. This
  is the useful part of runtime introspection at no cost, and it is why no author-facing
  `describe()` exists. Amended by ADR 0007: the SDK does now describe a *running app*, but to the
  host rather than to author code, and the author-facing surface is unchanged.
- **Transport carries `headers` as a thunk**, so the reporting environment can supply
  `X-Holistics-Use-Session-Auth` and a rotating CSRF token, and the API-key case later is the same
  field with different values.
- **The environment grew a fourth key, and will grow more.** `features` (ADR 0004) is server-side
  capability information that no declaration can state and no author should have to ask for. It is
  the same argument `datasets` makes, and the reason to keep stating the argument is that this ADR
  originally kept the environment narrow as a goal in itself. The test is not narrowness; it is
  whether the thing varies by host and cannot be declared.
