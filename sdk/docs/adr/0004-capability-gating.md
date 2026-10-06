# The environment declares what the tenant allows, so a declaration can fail loudly

`SdkEnvironment.features` carries server-side capabilities the host knows about and a declaration
cannot state. Today it holds one: `dateDrill`. With it explicitly `false`, `createDateDrill` throws
a `ValidationError` naming the toggle an admin has to enable.

## Why a gate rather than letting the server decide

A tenant toggle that is off does not usually produce an error. It produces silence. With
`interactive_control:date_drill` off, a `transform_date_drill` condition still reaches the server,
the server ignores it, and the query answers at whatever grain the AQL declared. Every request
succeeds. Every result is plausible. The grain simply never changes, and an author watching their
own drill control do nothing has no way to tell a broken app from a disabled feature.

That is the failure this exists to convert. An error at the declaring line, naming the toggle, is
recoverable by the person reading it — they ask an admin, or they stop building on a control that
will not work. A silently inert control is not.

## When a toggle earns an entry here, and when it does not

The test is whether the feature genuinely fails without the toggle, not whether a toggle exists.

`viz_setting:amql_filter_conditions` is the counter-example, and it is why the test needs stating.
It looks like it gates the AQL conditions this SDK generates (ADR 0006), and it does not:
`viz_to_aql.rb` reads `viz_setting.amql.conditions` directly rather than the toggle-gated
`VizSetting#conditions` accessor, so the toggle is a frontend gate only and the server path works
with it off. An entry for it would refuse something that in fact works — a gate that lies in the
safe direction is still a gate that lies.

So: a toggle earns an entry when it is *checked on the path this SDK uses*, and the failure with it
off is silent rather than loud. Otherwise the host does not send it and the SDK does not ask.

## Absent is not false

`features` is optional and each key within it is optional. Absent means the host did not say, and
nothing is blocked. Only an explicit `false` throws.

This matters because the SDK has hosts other than the reporting environment — tests, and eventually
an embedding page — and defaulting an unspoken capability to "off" would make every one of them
have to enumerate the full list to get working behaviour. The host that knows says so; a host that
does not know gets out of the way.

## Consequences

- **The error message names an operational fact, not a code fact.** "Ask an admin to enable
  `interactive_control:date_drill`" is a different genre from every other `ValidationError`, which
  reports something the author can fix in the line they just wrote. Accepted: the alternative is a
  message that describes the symptom and leaves the author to discover the cause.
- **This list will drift from the toggles that exist.** Nothing checks that a key here still
  corresponds to a live toggle, or that a newly relevant toggle was added. The list is small enough
  for that to be a review concern rather than a mechanism, and the failure mode of a stale entry —
  refusing something that now works — is loud rather than silent, so it surfaces.
- **The host decides what to send.** The SDK defines the shape and the behaviour; which toggles the
  reporting environment actually reads and forwards is the host context's business, and lives there.
