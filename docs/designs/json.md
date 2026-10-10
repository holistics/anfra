# JSON

JSON is how anfra talks: the answers it serves over HTTP, MCP and the CLI, the inputs it reads, its RPCs with the sidecars, its state files. How a Go value becomes JSON, and back, is decided once, in `shared/jsonkit`, and every package reads and writes JSON through it.

## The rule

**Read and write JSON with `shared/jsonkit`. Don't import `encoding/json` or `encoding/json/v2`.**

`encoding/json/jsontext` is fine to import: it holds types (`jsontext.Value` for raw JSON, the `Encoder` a `MarshalJSONTo` method is given) and no behavior to get wrong.

The one exception is a third-party API that makes you name one of its `encoding/json` types. Name it in that file only, and add the file to the exceptions of the `json-through-jsonkit` rule in `.golangci.yml`, which enforces all of this. Prefer what needs no exception: `jsontext.Value` satisfies `json.Marshaler`, so it can stand in for a `json.RawMessage` you pass in.

## Why one package

How a value is encoded depends on options, and Go takes them per call: there is no process-wide default. Left to each call site, every call has to pass the same options, and one that forgets encodes differently. That is how a query with no rows came to answer `"records": null` where its schema promised a list.

So the options live in one package, and everything calls it. What they are, and why, is below.

## How jsonkit reads and writes

It is `encoding/json/v2` with these options:

| Direction | Behavior | Why |
|---|---|---|
| Write | A nil slice or map is `[]` or `{}`, never `null` (v2's default). | The schema says list or object. |
| Write | Map keys are sorted (`Deterministic`). | The same answer is the same bytes. |
| Write | Invalid UTF-8 becomes U+FFFD (`AllowInvalidUTF8`). | One bad byte doesn't fail an answer. |
| Write | `<`, `>` and `&` are written as is (v2's default). | Answers are `application/json`, never HTML. |
| Read | Field names match case-sensitively (v2's default). | The contract is snake_case, exactly, as input validation already says. |
| Read | Duplicate member names are refused (v2's default). | A validator and a decoder can never see different values. |
| Read | Invalid UTF-8 becomes U+FFFD (`AllowInvalidUTF8`). | A bad byte in a sidecar's answer doesn't fail a query. |

Struct fields are written in declaration order, whatever the options.

## Writing types

- **A list or map field is never `null`.** Leave it nil if you like; it is written as `[]` or `{}`. A field that can be absent says so with a tag; one that can be `null` is a pointer marked `nullable:"true"`.
- **Omit a zero value with `omitzero`**, not `omitempty`. v2's `omitempty` omits a value that encodes as empty JSON (`null`, `""`, `[]`, `{}`), and so keeps `0` and `false`. Use `omitempty` only on a string, list or map where empty means absent, and on a pointer.
  Command args are the exception: `omitempty` is what marks an arg optional (`internal/command/args.go`).
- **Custom encoding belongs in `MarshalJSONTo`**, encoding with `jsonkit.MarshalEncode` and the encoder it is given, so the caller's options reach inside (`anfranode.ShowObject`). A `MarshalJSON` that calls `Marshal` starts over, and whatever encodes it may be using other options.
- **Raw JSON is a `jsontext.Value`.**

## Checking it

`depguard` fails CI on any import of `encoding/json` or `encoding/json/v2` outside `shared/jsonkit` and the named exceptions. Strict mode (`apikit.Runtime.Strict`, on in tests) validates every answer an op returns against its schema, encoded by `jsonkit`, so what is validated is what is sent.
