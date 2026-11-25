# Agent tracing

How to see every anfra command a coding agent ran for one piece of work, in order, with what each did and how it ended. This guide is for developers who run coding agents on anfra and want to debug or evaluate them. Where sessions fit in observability and eval is in the [overview](README.md).

> **Work in progress, and experimental.** Agent tracing is an experimental part of anfra that is still being built. Its names and settings may change in any release, and the changelog says when they do. Each part that isn't built yet is marked where it sits.

You need telemetry on for the repo ([telemetry](telemetry.md#turn-telemetry-on)). Agent tracing has no setup of its own: it labels the traces that telemetry already records.

## 1. Run your agent

Run your agent in the repo as usual. anfra labels each command the agent runs with the agent's session, as `gen_ai.conversation.id` and `session.id` on every span, and with the agent's name, as `gen_ai.agent.name`.

**Claude Code, Codex, OpenCode and Goose need nothing.** They give their session id to every command they run, and anfra reads it.

**Another agent,** such as Gemini CLI, Cursor or Amp, doesn't give its id. Start it with `ANFRA_SESSION_ID` set, and every command it runs inherits it:

```sh
ANFRA_SESSION_ID=$(uuidgen) amp
```

For a detected agent, `ANFRA_SESSION_ID` changes nothing, because the agent's own id wins. To label its commands with more ids, such as an eval run's, see [tag commands with your own ids](#tag-commands-with-your-own-ids).

A command you run yourself, at a terminal, has no session, so it's recorded alone.

## 2. Find the session's id

The id is the agent's own, so it matches the agent's own records. Ask the agent to print it:

| Agent | Variable |
|---|---|
| Claude Code | `CLAUDE_CODE_SESSION_ID` |
| Codex | `CODEX_SESSION_ID` |
| OpenCode | `OPENCODE_SESSION_ID` |
| Goose | `AGENT_SESSION_ID` |
| Another agent | `ANFRA_SESSION_ID`, which you set |

The id changes when the agent's does. Claude Code, for example, starts a new one after `/clear`. Codex's subagents share their session's id. An agent run by another agent inherits the outer agent's id too, so its commands may carry the outer agent's session.

## 3. Mark each message's work as a task

A session holds everything the agent did, for every message you sent it. A **task** groups the commands it ran for one message, so you can read and evaluate its work message by message. The anfra block of the repo's `AGENTS.md`, which `anfra agent setup` writes, tells the agent to mark tasks, so you set nothing.

For each message that asks for data, the agent:

1. Runs `anfra experimental telemetry task start "<your message>"`, which prints the task's id, such as `t_7f3a91c0d2e4`.
2. Passes `--task <id>` on every anfra command it runs for the message. Over MCP, it passes the id as the `task` argument of each tool call.
3. Gives a subagent its own task under it: the subagent runs `task start "<its part>" --parent <id>`, and passes its own id.

The agent can also record what it found, a change of approach, or its answer, for you to read later:

```sh
anfra experimental telemetry task note --task t_7f3a91c0d2e4 --kind summary "Revenue last month was $1.2M."
```

A task has no end. It is the commands that carry its id. A `--task` that isn't an id that `task start` printed fails the command, so a typo shows at once. With telemetry off, `task start` still prints an id, and says that nothing was recorded, so the agent skips tasks for the rest of the session.

## 4. Read the session back (coming soon)

Reading a session through anfra isn't built yet. It comes with the trace sample: a model of the spans, with AQL queries and a Data App to read a session by ([designs](../../designs/telemetry.md#querying-traces-through-anfra)).

Until then, a session's spans are in the [local telemetry stack](telemetry.md#the-local-telemetry-stack), or the tool you record to. Each command is its own trace, and the session's id is what links them: they are the spans whose `gen_ai.conversation.id` is the id. `anfra.log` records the id too, as `gen_ai_conversation_id` on each request.

## What a trace holds

Agent tracing adds these to every span of a command:

| Attribute | Holds |
|---|---|
| `gen_ai.conversation.id`, `session.id` | The session's id, under both names: OpenTelemetry's for an agent's conversation, and the general one that Langfuse and Claude Code's own telemetry group by |
| `gen_ai.agent.name` | The agent anfra detected, such as `claude-code`. Absent for an id from `ANFRA_SESSION_ID`, and on the spans `anfra serve` records for a command it runs, because only the session's id reaches the server. |
| `anfra.task.id` | The task a command ran for, from its `--task`, on every span of the command, those `anfra serve` records included |
| `anfra.task.parent_id`, `anfra.task.goal` | On `task start`'s span: the task's parent, for a subtask, and its goal, which is data |
| `anfra.task.note.kind`, `anfra.task.note.text` | On `task note`'s span: the note's kind, and its text, which is data |
| Your own ids, such as `run.id` | The baggage keys you let in ([tag commands with your own ids](#tag-commands-with-your-own-ids)) |

Everything else is what telemetry records for any command: each step and its timing, its error code, and by default your data, such as what the agent asked for, the SQL it ran and what came back ([what a trace holds](telemetry.md#what-a-trace-holds)). What an agent asks for often repeats the user's own question, and `ANFRA_EXPERIMENTAL_TRACE_DATA=0` leaves it out. The ids above are always recorded.

## Advanced: add more to your traces

### Tag commands with your own ids

The session groups an agent's commands, but you may need finer labels, such as the eval run and the case a command belongs to. Tag commands with your own ids, in W3C baggage, and anfra records them on every span of each command.

1. Let your keys in, in the repo's `.env`. anfra records only the baggage keys that [`ANFRA_EXPERIMENTAL_TRACE_BAGGAGE_INCLUDE`](../configs.md#telemetry) names, and by default it names `gen_ai.conversation.id` alone:

   ```sh
   ANFRA_EXPERIMENTAL_TRACE_BAGGAGE_INCLUDE=gen_ai.conversation.id,run.id,case.id
   ```

2. Send the ids where the commands start:

   - **An agent:** start it with `BAGGAGE` set, and every command it runs inherits it.

     ```sh
     BAGGAGE=run.id=nightly-17,case.id=revenue-by-region claude -p "..."
     ```

   - **A program calling the HTTP API:** the `baggage` header, such as `baggage: run.id=nightly-17`.
   - **A program calling the MCP server:** `_meta.baggage` on each tool call, in the same format.

A program can name its session the same way, as `gen_ai.conversation.id` in its baggage. `anfra serve` never takes a session id from its own environment, because one server serves many callers.

### Nest anfra under the agent's own trace

If the agent traces itself and passes its trace context to the commands it runs, anfra's spans go under the agent's tool call, so one tree shows the agent's steps and anfra's work. Nothing is set in anfra. Claude Code passes it to its commands when its own tracing is on ([Claude Code's monitoring](https://code.claude.com/docs/en/monitoring-usage)), and Codex passes it on MCP tool calls.
