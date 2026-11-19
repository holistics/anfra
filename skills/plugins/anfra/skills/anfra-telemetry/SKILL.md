---
name: anfra-telemetry
description: Set up telemetry for an Anfra repo on this machine — write the repo's local telemetry stack with `anfra experimental telemetry init`, start it with Docker, point the repo's commands at it in `.env`, and check it with `anfra status`. Use when the user asks to trace, record or look back at what anfra commands (theirs or an agent's) did.
---

# Setting up telemetry for a repo

Telemetry records what each anfra command in the repo does, the commands you run included: its
steps, how long each took, and whether it failed. The records go to a local telemetry stack the
user runs with Docker, one per repo. Nothing is sent to Holistics.

Done means: `anfra status` shows the repo's `telemetry` as `on`.

Only set it up when the user asks. Never point the repo at an endpoint the user didn't give you,
and never set ANFRA_EXPERIMENTAL_TRACE_DATA for them: the settings decide where the user's data goes.

## 1. Check Docker

```sh
docker info
```

If it fails, Docker isn't installed or isn't running. Tell the user, and stop: the stack needs it.

## 2. Write the stack's compose file

From the repo's folder:

```sh
anfra experimental telemetry init
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

## The user's data

By default a trace holds the user's data: what each command asked for, the SQL it ran, what it
answered, and an error's message. Never a query's rows. `anfra status`'s `data` section shows
whether it is on for the repo.

- **To leave it out,** set `ANFRA_EXPERIMENTAL_TRACE_DATA=0` in the same `.env`.
- **If anfra warns that traces record data and go to another machine,** tell the user and ask
  which they want: `0` keeps their data out, `1` sends it. Never choose for them.

## More

* `anfra experimental telemetry --help`, for what a trace holds and how to record your data.
* `anfra experimental telemetry init --help`, for what init writes.
