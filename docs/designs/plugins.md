# Data App plugins

> Status: designed, being built. Replaces the SDK's hard-coded bridge (`web/sdk/src/host/bridge.ts`), the `Backend` mount option and `Anfra.features`. Streamed calls come next; the AI agent plugin after them.

Everything a Data App reaches beyond its own page is a **plugin**: running a query, an export, lineage, a rendering, the inspect panel's highlight and pick. A host chooses the plugins its Data Apps get, and the same mechanism carries anfra's own features and the ones a host or a later release adds, such as an AI agent a reader prompts from the app, or a view of the rows under a chart.

Before this design, each feature was a case in one `switch` on the host, a method on `Backend`, a branch in the frame's bootstrap and a flag in `features`. Adding one meant editing all four, a host could change a feature only by wrapping `Backend`, and features leaned on each other's internals (inspect patched `createApp` to see apps appear; rendering knew the id of inspect's overlay to leave it out).

## What a plugin is

A plugin has a name, a version, the plugins it requires, and up to two halves:

| Half | Runs in | Does |
|---|---|---|
| **host half** | the host page, trusted | Answers the plugin's calls: reaches the core API, saves a file, keeps state across frames. |
| **frame half** | the frame, beside the definition | Adds the plugin's API for the author (`query.export()`), answers the calls the host makes into the frame, draws UI in the plugin layer. |

Either half may be absent: `core` has no frame half worth the name, a theming plugin might have no host half.

**The host installs plugins; a definition cannot bring its own.** A definition is untrusted ([security.md](security.md)) and carries no SDK version ([data-apps.md](data-apps.md#why-one-html-file)); letting it name plugins would mean negotiating versions and deciding what a definition may load. anfra's own features are plugins built through the same API a host uses for its own, so that API is held to what anfra itself needs.

## The kernel

What is not a plugin:

- **The plumbing**: the bridge envelope, call ids and cancellation, the sandbox, provisioning, and the plugin runtime itself.
- **The app model**: `App`, `Query`, controls, cross-filter selection, structure. No Data App exists without them, so making them optional would buy nothing.

Every call that crosses the bridge is a plugin's, including running a query. anfra's features become five plugins:

| Plugin | Calls and API |
|---|---|
| `core` | `core.submitQuery`, `core.fieldSuggestions`. Required. |
| `export` | `export.query` (runs the export, saves the file from the host page); `query.export()`. |
| `lineage` | `lineage.trace`; `Anfra.lineage()`. |
| `render` | The rendering (host→frame), the renderer's body, saving a file (byte check, rate limit); `Anfra.render()`. |
| `inspect` | The snapshot, highlight, locate, pick (both directions); `query.locate()`, `control.locate()`. |

The call names above are illustrative; the code is the reference.

**`core` is required.** The app model runs its queries by calling `core.submitQuery`, by name, through the same pipeline as any caller: the kernel declares `core` as a requirement, as a plugin would. To point queries at another server, or answer them with fixtures in a test, a host installs another plugin named `core`, or puts middleware around it. This replaces the `Backend` mount option; a host that names no `core` gets one over `coreApiBackend`.

## Calls and events

Two primitives cross between frame, host and plugins, both named `<plugin>.<name>`:

- A **plugin call** is a request a plugin answers: once, or as a stream ending in a final value or an error. The caller may cancel it while it runs. The frame calls host halves (`core.submitQuery`); the host calls frame halves (`render.draw`, `inspect.highlight`); a host half calls another plugin's calls.
- A **bridge event** is a notice nobody answers, in either direction: the frame's inspect snapshot, a node picked.

Events are not calls nobody waits on: "the snapshot changed" is not a request, and making it one would muddle cancellation and errors.

### Who may call

**A call is host-only unless its plugin opts it in to the frame.** Whatever the frame can call, the definition can call, since frame halves share its realm; a call meant for the host (one that spends the host's credentials, say) must not be reachable by forgetting to guard it. A frame call to a host-only call is refused before any middleware or handler runs.

**Every call carries who made it**: the frame (and which mount), the host, or a named plugin's host half. Middleware can treat the definition's queries apart from the ones an agent plugin runs.

## The host side

One **host runtime** per page holds the installed plugins and the middleware, and frames are mounted into it. Each host half gets a per-mount context when a frame mounts, disposed when it unmounts: per-frame state (a rate limit on saving files) lives there, state shared across frames (a cache) on the plugin. Middleware is registered once and sees every frame on the page.

### One pipeline

Every call is dispatched by name through one pipeline, whether the frame made it or another plugin's host half. That is how plugins use each other: an agent plugin runs a query by calling `core.submitQuery`, as the frame does. There are no services, no objects one plugin hands another.

### Middleware

Middleware is code the host, or a plugin's host half, puts around calls chosen by name (`*`, `core.*`, `core.submitQuery`). Like Koa's, each layer gets the call and `next`; it may change the call, answer it in place of the plugin (a cache), refuse it, or change the answer. For a streamed call it may wrap the stream itself, item by item, not only its start. Cancellation passes through every layer.

Its order is declared, not taken from registration order: every system surveyed for this design (Rollup, Vite, webpack's Tapable, Fastify) ended up adding explicit ordering when array position stopped being enough. Each call keeps a trace of the layers it went through, for debugging.

What middleware is for: logging (what appserve's and the reference host's inspectors show), adding a request id, a cache, masking, a platform checking quota or permission before a call leaves the page. It is not where anything is enforced: a platform enforces on its server, which the host half's requests reach.

### State

The SDK persists nothing. A host runtime lives as long as its page; a refresh starts again. A plugin that needs state to outlive the page, such as an agent's conversations, takes a store from the host as configuration, through an interface the plugin defines: a platform backs it with its own server, appserve with the browser or nothing. Who a reader is and where their data lives is the platform's ([data-apps.md](data-apps.md#what-is-cores-and-what-is-a-platforms)).

## The frame side

### Setup and body

A frame half reaches an opaque-origin frame as script source, so it is built to strings, as the app runtime and the renderer are today. It has two parts:

- **The setup**, inlined into the provisioned document, runs after the kernel and before the definition, in install order. It registers the plugin's API, its lifecycle subscriptions and the calls the host may make into the frame. It is small.
- **The body**, optional, is fetched over the bridge through a kernel helper on first use. Frames that never use the plugin never carry it. The renderer is the first body.

The setup is what lets the definition find `Anfra.lineage` the moment its first script runs.

### What a frame half sees

Frame halves read the kernel's app model and subscribe to its **lifecycle events**: an app created, a query's state changed. These are the kernel's, not hooks plugins define: a hook in the frame could enforce nothing, since the definition, loaded after, can patch anything in its realm.

**Frame halves don't depend on each other.** What they share is the kernel's app model and structure. That keeps install order safe and the frame free of a dependency graph.

### API for the author

A frame half adds API only under its own name: `Anfra.<plugin>`, `query.<plugin>`, `control.<plugin>`. Two plugins cannot collide, and a reader of a definition can tell which plugin a call needs.

**A plugin that is not installed adds nothing.** Its members are absent, not stubs that reject: the kernel knows no plugin by name, so it could not stub one written after it. A definition that uses an optional plugin checks first:

```js
if (Anfra.plugins.has('export')) {
  button.onclick = () => query.export({ format: 'csv' })
}
```

`Anfra.plugins` lists the installed plugins and their versions. It replaces `Anfra.features`.

### The plugin layer

The SDK supplies data and state; the author owns the page. Plugins draw only in the **plugin layer**, a layer the runtime owns above the definition's page: inspect's overlay, a modal showing a query's rows. Each plugin draws in its own shadow root, so the author's styles don't reach it and its styles don't reach the page. Plugins never insert anything into the definition's markup. A rendering never draws the plugin layer, which is how rendering leaves inspect's overlay out without knowing it.

UI that doesn't need the page's geometry (a chat panel, a settings form) belongs in the host's own page, drawn by the host with what the plugin's host half provides.

## Packaging

anfra's plugins are subpath exports of `@holistics/anfra-sdk` (`@holistics/anfra-sdk/plugins/export`, …), released with the kernel: a host always carries the frame halves it was built with, as it carries the app runtime ([data-apps.md](data-apps.md#the-pieces)), so frame and host never disagree. Each exports one plugin object:

```ts
{ name, version, requires, host, frame: { setup, body? } }   // setup and body: script source
```

The SDK exports `definePlugin` and a build helper that turns a frame half's source into its setup and body, checking it imports only `common`, as `app/bundle.test.ts` checks the app runtime. That shape is the contract for plugins outside the SDK. A first-party plugin that outgrows the SDK's release pace can move to a package of its own without changing it.

## What was decided against

- **Services** (cordis, Koishi; Fastify's decorators): plugins handing each other objects. Every use of another plugin would bypass middleware, so a platform's quota would miss the queries an agent runs; and services bring a dependency graph and lifecycle a host that installs its plugins once doesn't need.
- **Plugin-defined hooks** (Tapable, Rollup's hooks): extension points through which plugins change each other's behaviour. Middleware covers the host; in the frame a hook enforces nothing. The one need for fan-out (many plugins contributing to one, such as tools for an agent) is an extension point a plugin owns, decided with the agent plugin.
- **Hot reload**: plugins are installed when a page loads.
- **Plugins adding to the definition's markup** (a "view data" button on every block): the definition is everything the author wrote. If wanted, an attribute the author writes opts a block in.
- **Definitions declaring the plugins they need**, refused at mount when one is missing: additive, if hosts want it.

The trade-off: whatever cannot cross a call, such as a live object or a hook into each render, cannot be shared between plugins.
