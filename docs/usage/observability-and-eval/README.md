# Observability and eval

> **Work in progress, and experimental.** Observability and eval is an experimental part of anfra that is still being built, so its commands sit under `anfra experimental`. Its names and settings may change in any release, and the changelog says when they do.

When an agent answers a data question, you see the answer, but not how it got there. Did it use the metrics your models define, or write its own SQL? Did it pick the right fields and time range? When it got stuck, did it fix the problem, or work around anfra?

anfra's aim is that you can answer those questions for every session your agent runs: see what it did, understand why, judge whether it did well, and know what to improve.

## What observability and eval are

**Observability** is seeing what an agent did with anfra: every anfra command it ran, in order, with what each did and how it ended, for one session or across many.

**Eval**, short for evaluation, is judging whether the agent did its work well: whether it used the metrics the models define, and picked the right fields, filters and time range. You can evaluate the agent on questions you write for it, each with what a good answer looks like, or on its real work.

With both, you can find why an agent gave a wrong answer, and compare skills, prompts and models across many runs.

## Getting started

Getting started uses the [four parts of anfra's observability and eval](#how-anfra-builds-observability-and-eval) as they are now: telemetry records each command, agent tracing groups the commands by the agent's session, you evaluate the session yourself, and you read it in the tool you record to.

### Observability: see what your agent did

To see every anfra command your agent ran in a session:

1. **Record to a tool that shows traces,** such as Langfuse or Honeycomb, by setting its endpoint in the repo's `.env` ([record to a tool you already use](telemetry.md#record-to-a-tool-you-already-use)). Most hosted tools also need an API key ([a backend that needs a key](telemetry.md#a-backend-that-needs-a-key)). anfra then records a trace of every command there. The [local telemetry stack](telemetry.md#the-local-telemetry-stack) keeps traces too, but has no screen to read a session in yet. The traces go only where you record them, never to Holistics, and by default they hold your data, such as the SQL the agent ran ([what a trace holds](telemetry.md#what-a-trace-holds)).
2. **Run your agent** in the repo as usual. Agent tracing labels each command with the agent's session id. Claude Code, Codex, OpenCode and Goose need nothing set. Other agents need `ANFRA_SESSION_ID` ([agent tracing](agent-tracing.md#1-run-your-agent)).
3. **Find the session's id** ([find the session's id](agent-tracing.md#2-find-the-sessions-id)), and open the session in the tool you record to.

**To see beyond anfra,** turn on the agent's own tracing. anfra's trace covers only the commands that go through anfra. The agent's own trace adds its other steps, such as reading files or running shell commands. anfra's spans then go under the agent's, so one tree shows both ([nest anfra under the agent's own trace](agent-tracing.md#nest-anfra-under-the-agents-own-trace)).

#### Example

Say you asked your agent "What was revenue by region last quarter?". Its session might hold these commands, shown here as a list (the tool you record to lays them out its own way):

```
session 3f2a9c… (claude-code)
├─ anfra show                                                 ok
├─ anfra show orders                                          ok
├─ anfra query --dataset orders              (AQL)            failed   query_invalid
└─ anfra query --lang sql --data-source dw   (SQL it wrote)   ok       4 rows
```

From this one session you can tell that the agent looked at the datasets first, which is what you want. But when its AQL failed, it didn't fix the query. It wrote its own SQL against the warehouse instead, skipping the semantic layer, so the answer may not use the revenue metric your models define. The failed query's error tells you where to start finding out why.

Open any command, and you see its steps and how long each took, the AQL or SQL it ran, the SQL anfra ran on the warehouse, and its error's message ([what a trace holds](telemetry.md#what-a-trace-holds)).

### Eval: judge how well it did

Once you can see a session, you can judge it in two ways:

- **With your own agent.** Give a second agent the session's conversation and its anfra commands, and ask it whether the work was done well. Both carry the same session id, so the id finds both: the conversation in the agent's own records, and the commands in the tool you record to, such as through its API or an export. Questions worth asking it:
  - Did the agent query datasets with AQL, or write its own SQL?
  - Did it use the metrics the models define, or compute its own?
  - Did it pick the right fields, filters and time range for the question?
  - When a command failed, did it fix the cause, or work around anfra?
- **With an evaluation platform.** Platforms such as Langfuse and Braintrust read the same OpenTelemetry traces, so you can define and run evaluations there, such as a set of questions, each with what a good answer looks like. To tell an evaluation's runs and cases apart, tag each command with your own ids ([tag commands with your own ids](agent-tracing.md#tag-commands-with-your-own-ids)).

## How anfra builds observability and eval

anfra builds observability and eval in four parts, each adding to the one before it. Telemetry and agent tracing give you observability, and evaluation gives you eval. Reading it all in anfra brings the three together in one place.

[Getting started](#getting-started) uses each part as it is now. Each part below says what is available now, and, where there is any, what anfra is working on and what it is considering.

| Part | Step of the aim | What it tells you |
|---|---|---|
| [Telemetry](#telemetry-what-each-command-did) | See what it did | What each command did |
| [Agent tracing](#agent-tracing-why-the-agent-did-it) | Understand why | Which session each command belongs to, and, once the agent explains itself, why it ran each one |
| [Evaluation](#evaluation-whether-the-agent-did-it-well) | Judge whether it did well, and know what to improve | Whether the agent did its work well, and where it needs better skills, prompts or models |
| [Read it all in anfra](#read-it-all-in-anfra) | Every step, in one place | What the other three tell you, read through anfra instead of a separate tracing tool |

### Telemetry: what each command did

**Available now.** A trace of each command, as in [the example](#example): its steps and timing, its query and SQL, its row count, and its error code ([telemetry](telemetry.md)).

**Work in progress.**

- **Metrics and logs,** sent to the same place as the traces, such as how often each command runs and how long it takes.

### Agent tracing: why the agent did it

**Available now.** The agent's commands grouped by its session, as in step 2 of [getting started](#observability-see-what-your-agent-did). You see what the agent ran and in what order, but you infer its reasons from that. You can also tag commands with your own ids, such as an eval run's, and nest anfra's spans under the agent's own trace ([agent tracing](agent-tracing.md)).

**Work in progress.**

- **The agent explains itself.** Beside its commands, the agent records the user's question, its goal, and why it took each step, such as something it found or a change of approach. When it writes its own SQL instead of using a dataset, you read its reason right there, instead of guessing from a list of commands.

**Under consideration.**

- **A session for every agent,** inferred by anfra for an agent that doesn't give its id, so you don't have to set `ANFRA_SESSION_ID`.

### Evaluation: whether the agent did it well

**Available now.** You evaluate a session yourself, with your own agent or an evaluation platform ([eval](#eval-judge-how-well-it-did)).

**Under consideration.**

- **anfra evaluates your sessions.** anfra checks each session for where the agent:
  - skipped the semantic layer, such as writing its own SQL instead of querying a dataset
  - used the semantic layer wrong, such as picking the wrong metric, field or filter
  - couldn't get its analytics done with anfra at all

  It checks the sessions you run as an evaluation, on questions you write with what a good answer looks like, and your users' real sessions too. You find where your agent needs better skills, prompts or models without reading every session yourself.

### Read it all in anfra

**Available now.** Nothing in anfra yet. You read sessions in the tool you record to, as in step 3 of [getting started](#observability-see-what-your-agent-did).

**Work in progress.**

- **A Data App for your sessions.** A sample repo models the traces, and its Data App shows your agent's recent sessions, one session as a timeline, and errors by code. Once the agent explains itself, the Data App shows its notes beside its commands.
- **Ask about sessions in AQL.** Your agent queries the traces like any other dataset, such as "which sessions wrote their own SQL this week?". You need no separate tracing tool ([designs](../../designs/telemetry.md#querying-traces-through-anfra)).

## Next steps

- [Telemetry](telemetry.md): turn it on, the local telemetry stack, and what a trace holds.
- [Agent tracing](agent-tracing.md): sessions, tagging commands with your own ids, and nesting anfra under the agent's own trace.
- [Configs](../configs.md#telemetry): every telemetry setting.
