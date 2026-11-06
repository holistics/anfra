# Readable prose

This doc is about the text anfra writes for someone to read. Its main concern is text that anfra's users see, whether they're people or agents:

- error messages and their help
- warnings, notices and hints
- `--help` text and flag descriptions
- command descriptions and the `doc` tags that become schema descriptions, which agents read through the API and MCP
- the skills, and the user docs

Two kinds of reader see this text, and both need to act on it. A person at a terminal has to understand what happened and what to do next. An agent has to do the same, and it will act on the text literally. If the text is unclear to a person, an agent will usually get it wrong too.

The same rules apply to text only anfra's developers read, such as design docs and code comments. Those suffer from the same mistakes, but it matters less there. Their readers can ask, or read the code. So fix user-facing text first.

## The problem

Much of anfra's prose reads like notes. It is written by someone who already knows the answer, to remind themselves of it. A reader who doesn't know the answer yet has to decode it first.

Here is the error `anfra query export --json` used to print:

> --json isn't available here: without --link it writes the file; --link writes the export's link as a document.

Every fact in it is correct. But the reader has to work out who "it" is, what "as a document" means, and how the clauses after the colon and the semicolon relate to each other. And the fix isn't stated anywhere. The reader has to infer it. Compare:

> --json only works with export --link, which answers with the export's link instead of writing the file.

The second version starts with what the reader needs to change, and then explains why in one plain clause.

The same habits show up all over the codebase:

| Where | Text |
|---|---|
| An error | `%s exists: another anfra is changing %s, or one stopped midway; remove it if none is running` |
| An error | `not a regular file, which Git does not read: add %s to the repo's ignore rules yourself` |
| A schema description | `installed, when it has a plugin or copies; or not-installed` |

There are four common mistakes. The first three matter most.

## 1. Write for the reader who is looking

Write for someone who has just hit this message or opened this doc. They don't know what you know. Usually they want one of three things: what happened, why, and what to do now.

- **Lead with what matters to the reader.** In an error, that's what went wrong or what to change. In a doc, it's what the thing is and when you'd use it. Background comes after.
- **Say what to do.** If the reader can fix it, tell them how, with the exact command or flag. Don't leave the fix to be inferred from the cause.
- **Write whole sentences.** Give each sentence a subject and a verb. "Not a regular file" leaves the reader to guess which file. "`.gitignore` is a symlink" doesn't.
- **Use the reader's words, not anfra's internals.** Words like "answers", "as a document", "door", "sidecar" and "op" mean something precise inside anfra and nothing to a newcomer. Say "prints", "returns" or "the CLI and the API", or explain the term the first time you use it.
- **Name things instead of pointing at them.** Avoid "it", "one" and "that" when the reader could pick the wrong thing. Repeating a noun is better than an ambiguous pronoun.

Before:

> %s exists: another anfra is changing %s, or one stopped midway; remove it if none is running

After:

> Can't change .anfra/config.yml, because .anfra/config.yml.lock exists. Another anfra may be changing it. If none is running, delete the lock file and try again.

## 2. Don't join ideas with colons and semicolons

A colon or a semicolon lets a writer glue two thoughts together without saying how they relate. The reader is left to work that out. Is the second part a cause? A consequence? An example? A fix?

- **Use a colon only to introduce something.** That can be a list, an example, a command to run, or a value after a label (`Repo: /tmp/acme`). Don't use one to join two clauses.
- **Don't use semicolons in messages.** Write two sentences instead.
- **Say how the ideas relate.** Use "because", "so", "but", "if", "which" or "instead of". These words are the whole point of the sentence, so don't leave them out.
- **Keep one idea per sentence.** If a sentence needs more than one comma-separated aside, split it.

Before:

> not a regular file, which Git does not read: add %s to the repo's ignore rules yourself

After:

> Can't add %s to .gitignore, because .gitignore is a symlink and Git doesn't follow it. Add %s to the repo's ignore rules yourself.

A wrapped error chain (`create the export's file: open /tmp/x: permission denied`) is the one place colons join clauses on purpose. Each part names a step, and the colon reads as "failed because". Keep each part short, so that the chain stays easy to read.

## 3. Disclose progressively

Give the reader what they need at this moment, and make it easy to find the rest. Both too much and too little get in the way.

- **Too much at once.** A wall of text buries what the reader came for. The `--help` of `anfra publish` explains which files are sent, which aren't and why, where they go, and how the org is stored, all in one block. Someone who just wants to publish has to read all of it to find the one line they need.
- **Too little, with no way to find more.** A message that stops short without saying where to look leaves the reader stuck. An agent will guess.

To get it right:

- **Order by need.** Put the common case and the next action first. Put edge cases, reasons and background later, or somewhere else. For example, "can't change config.yml, because it's locked" comes first, and the lock file's path only at the step that has the reader delete it.
- **Point to the next layer.** End with where to learn more, such as `anfra <command> --help`, `--yaml` for the full result, or a doc's link. Name the exact command or flag.
- **Put what can be skipped in brackets.** Within a sentence, brackets mark what a reader may skip on a first read. "A column's (database-agnostic) type in the semantic layer" reads first as "a column's type in the semantic layer", and the bracketed word is there for whoever needs it. Use them for a qualifier or a detail, never for the main point or the fix.
- **Let each layer stand alone.** A one-line summary (`Short`, a command's summary, an error's message) must make sense without the detail behind it. The detail must make sense without rereading the summary.
- **Think about how the reader got here.** An agent usually meets a command through its summary in a list, then its description, then an error. Each of those steps should lead to the next one.

## 4. Use formatting

Formatting makes the shape of the text visible, so the reader doesn't have to parse it out of punctuation.

- **Paragraphs.** Give each paragraph one topic. A long paragraph is often two or three.
- **Bullets.** Use them for three or more parallel items, such as options, conditions or steps. Use numbered lists when the order matters.
- **Tables.** Use them when several things are compared along the same attributes.
- **Headings.** In a doc, use them so a reader can skim to what they need.
- **Code formatting.** Put commands, flags, paths and values in backquotes, so the reader can tell them apart from prose and copy them.

Short messages have structure too. An error has a `message` and a `help`, so put the fix in `help` when it doesn't fit in one sentence. A description can use labels, the way a list would, in a single line. Rule 2 allows this, because each colon introduces something.

Before:

> installed, when it has a plugin or copies; or not-installed

When the values' names speak for themselves, just list them:

> Whether the anfra skills are installed for this agent. Possible values: `installed`, `not-installed`.

When a value needs explaining, label each one:

> Whether the anfra skills are installed for this agent. `installed`: it has the plugin, or a copy of the skills. `not-installed`: it has neither.

## Considerations

The rules above pull against other things that matter. When they conflict, weigh them as follows.

**Be concise, but by cutting content, not context.** There are two ways to make text shorter, and only one of them helps.

- **Cut what the reader doesn't need.** That means asides, background they can't act on, restated summaries, and filler like "please" and "note that". This makes the text shorter and easier to read.
- **Cut what they already know.** Think about the least experienced reader, but weigh how rare they are, and whether they have help to fill the gap. An explanation every reader pays for should earn its place for most of them. "Requests must be application/json." reaches people and agents writing HTTP requests by hand. Nearly all of them know that's the `Content-Type` header, and the rare person who doesn't usually has an agent to ask. So the message doesn't explain it.
- **Don't compress what they do need.** Dropping subjects, connectives like "because" and "instead of", or the context a newcomer lacks makes the text shorter but harder to read. That is exactly how note-style prose comes about. It's concise for the writer, who already knows what's missing, and costly for the reader, who has to reconstruct it.

So measure conciseness by the reader's effort, not by word count. Twelve clear words beat eight cryptic ones. If a message is still too long once the clear version is written, it's carrying too much. Move some of it to the next layer ([progressive disclosure](#3-disclose-progressively)) rather than squeezing it.

**Every word costs the reader something.** A person has to read and hold it in mind. An agent pays for it in context, and some text is paid for many times over. Command descriptions and `doc` tags are loaded into an agent's context on every session that uses MCP, whether or not the agent needs them. So:

- Keep summaries short, and put the detail where it's fetched only when needed (`--help`, an op's usage, a doc).
- Explain why only when the reason helps the reader act, or stops them from doing something harmful.
- Don't repeat what the reader already has. A field's description shouldn't restate its name, and a detail shouldn't restate its summary.

**Don't distract.** Every hint, notice and aside competes with the one thing the reader came for. Leave out anything they can't act on now, even if it's true and interesting.

**Agents act literally.** An instruction in an error may be followed without judgement. Make sure following it is safe, and state the condition it depends on ("If none is running, delete the lock file"). Don't tell an agent to do something only a person should decide.

**The same text reaches different places.** A message may be printed in a terminal, logged as one line, returned as JSON over HTTP, or shown as an MCP tool result. Formatting must degrade well:

- Errors, notices and `doc` tags are plain text. Keep them to sentences, with no bullets or line breaks a log or JSON string would mangle. Put structure in the error's fields (`help`, violations) instead.
- Name things as the reader typed them. An error from a core command can reach the CLI, where the reader passed `--dataset`, and the API, where they sent `dataset`. Name the input in words both can recognise, and let the violation's field carry the exact name.
- Bullets, tables and headings belong in `--help`, skills and docs.

**Be exact.** Plain words must not make the text wrong. If the simple version is wrong, keep the precise term and explain it.

**Write for what really happens.** Being exact doesn't mean covering every case equally. Think about the situations readers realistically end up in, and write for those. Knowing how the code behaves tells you which situations are realistic. For example, anfra locks `.anfra/config.yml` only while it writes the file, which always takes a moment. So a lock that's still there a moment later was almost certainly left by a crash. "If none is running, remove the lock" covered every case accurately, but it asked the reader to check something they had no easy way to check. The message now says to try again in a moment, and to delete the lock if it's still there.

**State the situation, not the mechanism.** The mechanism is how you work out what happened, but the reader needs only what happened: "anfra 0.19.0 was just released, and its build for linux/amd64 isn't uploaded yet", not how releases are built. Give the mechanism only when the reader needs it to get the fix right, as in "AGENTS.md is a symlink, which anfra doesn't follow". When you can't be sure of the situation, say what anfra saw instead, such as "invalid or has expired".

**Sometimes the best fix is in the code.** When a message is hard to write, because it needs a caveat, an exception, or a list of manual steps, the behavior may be what's awkward. Fix that rather than the wording, and don't cut a true, actionable fact just to make the text cleaner.

- *A missing command.* A repo without `.anfra/data_sources.yml` used to get a YAML sample to copy. Now `anfra init --data-sources` creates the file, and the error just names that command.
- *Inconsistent behavior.* An org address was refused for an upper-case org name, though the server's case was ignored. The message would have needed "(the org's name in lowercase)". Lowercasing the name, as the server is already lowercased, removed the need for it.

Weigh a code change like any other: it's more to build and keep working. Make it when readers realistically run into the problem, not just to shorten one message.

**Stay open to how agents write.** The style this guide pushes against is common in text written by agents like Claude, and agents may find parts of it easy and cheap to read. So don't treat every trace of it as a mistake to coerce away. Some of its habits may serve both kinds of reader, such as labels instead of sentences, or terse lists of values. When one does, adopt it on purpose and add it to this guide, the way rule 2 allows a colon that introduces something.

That isn't the same as accepting the style as it is. A habit earns its place by working for the reader, and that includes people. So the guide is meant to change: refine it as rewrites and agent checks show what works for people and agents together.

**Work with the error flow, not around it.** An error message is usually a chain, with each part written by the function that raised or wrapped it: `create the exports folder in the temp folder (TMPDIR): mkdir /tmp/anfra-exports-1000: permission denied`. Don't write the ideal final message first and then bend the code to produce it, with special cases, ad-hoc string building, or one layer rewording another's message. Instead:

1. **Make each part good on its own.** Each function writes its part as well as it can, from what it knows, the way it handles its own inputs and outputs. That part should read well whoever wraps it, and wherever it ends up.
2. **Then give the error structure.** If the chain still reads badly, let the error carry what a caller needs (a code, details, a violation, help), so the caller who shows it can present it. For example, the CLI can add help to any `sidecar_unavailable` error, where the sidecar package can't know about the CLI. If it's missing what anfra was doing, make that operation a public step, which shows as the error's context ([steps](errors.md#steps-context-written-once)), rather than writing a "couldn't …" prefix into the message. For example, a failed write to anfra's folder while renewing a login shows "renew the login to …" as its context.

The final message and the code's error handling meet in the middle. Neither is forced to fit the other.

**Rewrites have a cost.** Changing text means updating the tests that match on it and regenerating the API contract when a description changes. Make the change when it helps a reader, not to polish text that already reads well.

## Words

Use one word for each concept, and use it everywhere. Readers, and agents especially, learn the words anfra uses and assume a new word means a new thing.

Prefer a plain word to one of anfra's own. Words used inside anfra's code tend to leak into what users read, where they mean nothing to a newcomer. Whether a word fits depends on who is reading and why, so there's no fixed list of replacements. Some examples:

- **answer.** "Write the answer as YAML" reads better as "write the result as YAML". As a verb, "answers with the export's link" reads better as "returns the export's link", or "prints" at the CLI.
- **refuse.** "Refuse what would need an answer" reads better as "fail instead of asking".
- **warm server.** "Whether a warm server is running" reads better as "whether `anfra serve` is running".
- **document.** "`--json`: a JSON document" says no more than "`--json`: JSON".
- **take.** A suggestion: a command *takes* a flag or an argument ("anfra serve doesn't take --json"). "Support" suggests a missing feature, so keep it for that: "validating a SQL query is not supported yet". "Allow" suggests a rule.
- **contract.** It fits where the reader cares about compatibility, such as a platform embedding anfra, which relies on the core API not changing under it. It doesn't fit `anfra openapi --help`, where the reader just wants the API's OpenAPI document.

## Words we teach

Some of anfra's own words are worth teaching, because they name something users need to know about and no plain word does. Use these as they're written here, and explain each one where a reader first meets it.

| Word | Meaning | Explained in |
|---|---|---|
| repo | The folder anfra works in, with its models, datasets and Data Apps. | `anfra init`, the root `--help` |
| core API, core commands | The commands that power what anfra does: querying, validating, and showing and searching the semantic layer. `anfra serve` also serves them over HTTP and MCP, as the core API. The other commands, such as `init`, `publish` and `update`, are CLI-only, and set up or manage anfra itself. | The root `--help` |
| sidecar | An embedded process that anfra starts and manages. `anfra-node` compiles AML, and `canal-query` runs queries. | `anfra status`, `anfra serve --help`, the `sidecar_unavailable` error |
| pretty format | A result laid out for reading at a terminal, with tables and colour (`--pretty`). | `--pretty` |
| semantic type | A column's (database-agnostic) type in the semantic layer: `number`, `date`, `truefalse` and so on. | The field that carries it |

Standard terms need no explaining for the readers who meet them, such as upstream (in lineage), Arrow type, OpenAPI and MCP.

To add a word to this table, check first that no plain word would do, and decide where it's explained.

## By kind of text

**Error messages.** Say what went wrong in the reader's terms. Then say why, if the reason helps them fix it. Then say what to do. Name the flag, file or value at fault.

**Warnings and notices.** Say what the reader should know and the one thing to do about it. A notice the reader can't act on is noise.

**Flag and command help.** Start with a verb that says what it does ("Writes the result as JSON"). Mention the default when there is one.

**Schema descriptions (`doc` tags).** Agents read these to decide what to send and how to read what comes back. Say what the field is. List its values, and explain one only when its name doesn't say enough. Say when the field is absent.

**Design docs.** Open with what the thing is and why it exists, for someone who has never seen it. Put decisions and their reasons in paragraphs, and use lists and tables for the details.

## Before you commit

Read the text as someone who has just hit it, with no context:

1. Can they tell what happened?
2. Can they tell what to do, without guessing?
3. Does every colon introduce something, and is every semicolon gone?
4. Does every word earn its place?
5. Does it lead with what they need now, and point to where the rest is?
6. Would a list, a table or a second paragraph make it easier to read?
