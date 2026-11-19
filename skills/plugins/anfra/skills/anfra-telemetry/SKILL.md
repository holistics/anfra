---
name: anfra-telemetry
description: Set up telemetry for an Anfra repo on this machine — write the repo's local telemetry stack with `anfra telemetry init`, start it with Docker, point the repo's commands at it in `.env`, and check it with `anfra status`. Use when the user asks to trace, record or look back at what anfra commands (theirs or an agent's) did.
---

# Setting up telemetry for a repo

Telemetry records what each anfra command in the repo does, the commands you run included: its
steps, how long each took, and whether it failed. The records go to a local telemetry stack the
user runs with Docker, one per repo. Nothing is sent to Holistics.

Done means: `anfra status` shows the repo's `telemetry` as `on`.

Only set it up when the user asks. Never point the repo at an endpoint the user didn't give you,
and never turn on more data than they asked for: the settings decide where the user's data goes.

## 1. Check Docker

```sh
docker info
```

If it fails, Docker isn't installed or isn't running. Tell the user, and stop: the stack needs it.

## 2. Write the stack's compose file

From the repo's folder:

```sh
anfra telemetry init
```

It writes the compose file and starts nothing. Its `hints` give the command that starts the
stack, and the endpoint line for `.env`. Follow them in steps 3 and 4.

## 3. Start the stack

Run the `docker compose ... up -d` command from init's hints. The first time, Docker downloads the
images, which takes a minute.

## 4. Point the repo at it

Add init's endpoint line, `OTEL_EXPORTER_OTLP_ENDPOINT=...`, to the `.env` at the repo's root.
Create the file if it isn't there. `anfra init` git-ignores it; never commit it.

If init's hints say another endpoint is set already, ask the user which one they want, rather than
replacing it.

If `anfra serve` runs for this repo, restart it, as init's hints say.

## 5. Check it

```sh
anfra status
```

Its `telemetry` section should be `state: on`, `set_by: OTEL_EXPORTER_OTLP_ENDPOINT in .env`. If
it says `failing`, the stack isn't running: check `docker compose ... ps` with the file from step 2.

## Recording more

By default a trace holds none of what a command asked for or answered, and masks SQL and error
messages. `anfra telemetry --help` lists each setting and its values; `anfra status`'s `data`
section shows what applies to the repo.

- **Turn a setting up only when the user asks,** in the same `.env`. Inputs, answers and raw SQL
  can hold their data, such as filter values.
- **To group traces by dataset and data source,** or see what each command asked for, inputs is
  the one to turn on: `ANFRA_TRACE_INPUTS=raw`.
- **Never turn one up when the endpoint isn't on this machine.** anfra warns when that happens;
  tell the user rather than ignore it.

## More

* `anfra telemetry --help`, for what a trace holds and each setting.
* `anfra telemetry init --help`, for what init writes.
* [](../anfra-docs/), the telemetry page: which span names and attributes stay the same across
  releases. Build queries and Data Apps over traces on those.
