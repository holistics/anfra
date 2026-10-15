# Data App plugins

> Status: built: the host runtime (`web/sdk/src/host/runtime.ts`, `plugin.ts`), the frame's kernel (`web/sdk/src/app/bootstrap.ts`, `kernel.ts`, `layer.ts`), the bridge (`web/sdk/src/common/bridge.ts`, `endpoint.ts`) and anfra's plugins (`web/sdk/src/plugins`). It replaced a hard-coded bridge, the `Backend` mount option and `Anfra.features`. Streamed calls come next; the AI agent plugin after them.

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

Every call that crosses the bridge is a plugin's, including running a query. anfra's features are five plugins (`web/sdk/src/plugins`):

| Plugin | Calls (who may make them) | Events | API |
|---|---|---|---|
| `core` | `core.submitQuery`, `core.fieldSuggestions` (frame) | | none: the app model is its frame side. Required. |
| `export` | `export.query` (frame): runs the export, saves the file from the host page | | `query.export()` |
| `lineage` | `lineage.trace` (frame) | | `Anfra.lineage()` |
| `render` | `render.save` (frame): byte check, rate limit; `render.draw`, answered by the frame half (host) | | `Anfra.render()`; the renderer is its frame body |
| `inspect` | `inspect.watch`, `.highlight`, `.locate`, `.pick` (host), each sent on to the frame as an event | `inspect.snapshot`, `inspect.picked` (frame) | none |

The structure, the overlay and `query.locate()` / `control.locate()` stay the kernel's: they are the app model's own parts, and `inspect` draws with them rather than owning them.

**`core` is required.** The app model runs its queries by calling `core.submitQuery`, by name, through the same pipeline as any caller: the kernel declares `core` as a requirement, as a plugin would. To point queries at another server, or answer them with fixtures in a test, a host installs another plugin named `core`, or puts middleware around it. This replaces the `Backend` mount option. A host always names it (`corePlugin(coreApiBackend(api, { datasets }))`): the SDK can't know where the host's core API is, and a host runtime without `core` refuses to start.

## Calls and events

Two primitives cross between frame, host and plugins, both named `<plugin>.<name>`:

- A **plugin call** is a request a plugin answers: once, or as a stream ending in a final value or an error. The caller may cancel it while it runs. The frame calls host halves (`core.submitQuery`); the host calls frame halves (`render.draw`, `inspect.highlight`); a host half calls another plugin's calls.
- A **bridge event** is a notice nobody answers, in either direction: the frame's inspect snapshot, a node picked.

Events are not calls nobody waits on: "the snapshot changed" is not a request, and making it one would muddle cancellation and errors.

### Who may call

**A call is host-only unless its plugin opts it in to the frame.** Whatever the frame can call, the definition can call, since frame halves share its realm; a call meant for the host (one that spends the host's credentials, say) must not be reachable by forgetting to guard it. A frame call to a host-only call is refused before any middleware or handler runs.

**Every call carries who made it, and who started it.** `caller` is who made it: the frame (and which mount), the host, or a named plugin's host half. `chain` is every hop that led to it, outermost first, and `origin` its start, the frame or the host. A query an agent plugin runs for the frame's prompt has the agent as its `caller` and the frame as its `origin`: a platform's quota counts it by `origin`, an audit log names the agent by `caller`. Without the chain, a frame-callable plugin call would launder what the frame started into the plugin's own calls.

**A plugin may hold its call to what the frame may call** (`call(name, request, { asOrigin: true })`): started by the frame, it is refused unless the frame could make it itself. It is for a handler that forwards a call, or a request, the frame chose, so the plugin can't be made the frame's deputy for a host-only call. It is not the default: an agent plugin's own calls (the one spending the host's model key) are the plugin's to make, whoever asked.

**A frame's event reaches the host only if a plugin declared it** (`host.events`), and only as that plugin's check passed it: what the check returns is what listeners get, and nothing (`undefined`, or a check that throws) drops the event. An event's payload is the definition's to make up, as a call's request is, so the plugin that owns the event is where its shape is checked, once, rather than in each host's listener. `inspect` passes a snapshot or a pick only while the host watches, and only of its shape.

**What crosses back to the frame when a call fails** is the SDK's error class and message (`ValidationError`, `QueryError`, `PermissionError`, `TransportError`), which were written for an author to read. Any other error, from a bug, a middleware or a library, crosses as a plain `QueryError` with no message of its own, since its message may hold what the frame must not see: a URL, an id, a server's error. A handler or middleware that wants the frame to read why throws one of the SDK's classes.

The kernel's own call, a plugin's frame body (`anfra.body`), is a call like any other: frame-callable, through the middleware.

## The host side

One **host runtime** per page holds the installed plugins and the middleware, and frames are mounted into it. Each host half gets a per-mount context when a frame mounts, disposed when it unmounts: per-frame state (a rate limit on saving files) lives there, state shared across frames (a cache) on the plugin. Middleware is registered once and sees every frame on the page.

### One pipeline

Every call is dispatched by name through one pipeline, whether the frame made it or another plugin's host half. That is how plugins use each other: an agent plugin runs a query by calling `core.submitQuery`, as the frame does. There are no services, no objects one plugin hands another.

### Middleware

Middleware is code the host, or a plugin's host half, puts around calls chosen by name (`*`, `core.*`, `core.submitQuery`). Like Koa's, each layer gets the call and `next`; it may change the call, answer it in place of the plugin (a cache), refuse it, or change the answer. For a streamed call it may wrap the stream itself, item by item, not only its start. Cancellation passes through every layer.

Its order is declared, not taken from registration order: every system surveyed for this design (Rollup, Vite, webpack's Tapable, Fastify) ended up adding explicit ordering when array position stopped being enough. Each call keeps a trace of the layers it went through, for debugging.

**A plugin's middleware is as powerful as the host's**: it sees and may change every call on the page, its own plugin's or not, and so does a host half, which runs with everything the host page has (its session, its API client). Installing a plugin is trusting it with the whole page. A plugin's frame half is the other way round: it runs beside the definition, trusted by nothing.

What middleware is for: logging (what appserve's and the reference host's inspectors show), adding a request id, a cache, masking, a platform checking quota or permission before a call leaves the page. It is not where anything is enforced: a platform enforces on its server, which the host half's requests reach.

### State

The SDK persists nothing. A host runtime lives as long as its page; a refresh starts again. A plugin that needs state to outlive the page, such as an agent's conversations, takes a store from the host as configuration, through an interface the plugin defines: a platform backs it with its own server, appserve with the browser or nothing. Who a reader is and where their data lives is the platform's ([data-apps.md](data-apps.md#what-is-cores-and-what-is-a-platforms)).

## The frame side

### Setup and body

A frame half reaches an opaque-origin frame as script source, so it is built to strings, as the app runtime is. It has two parts:

- **The setup**, inlined into the provisioned document, runs after the kernel and before the definition, in install order. It registers the plugin's API, its lifecycle subscriptions and the calls the host may make into the frame. It is small. The kernel and every setup run inside one function (`composeFrameScript`), so none of them is left on the frame's window; a setup that throws is logged and skipped, and the rest run.
- **The body**, optional, is fetched with the kernel's own call, `anfra.body`, on first use. Frames that never use the plugin never carry it. The renderer is the first body.

The setup is what lets the definition find `Anfra.lineage` the moment its first script runs.

### What a frame half sees

A setup gets the **kernel** (`FrameKernel`, `web/sdk/src/app/kernel.ts`), one per plugin: the SDK instance and its apps, the error classes, the overlay and the page's structure, the plugin's calls and events, its place in the plugin layer, its body. It hears of each app the definition creates (`onApp`), and of each change through the app's own `subscribe`. These are the kernel's, not hooks plugins define: a hook in the frame could enforce nothing, since the definition, loaded after, can patch anything in its realm.

**Frame halves don't depend on each other.** What they share is the kernel's app model and structure. That keeps install order safe and the frame free of a dependency graph.

### API for the author

A frame half adds API only under its own name: `Anfra.<plugin>` (`kernel.provide`), `query.<plugin>` and `control.<plugin>` (`kernel.provideOnQuery`, `provideOnControl`, made the first time each entity's is read). Two plugins cannot collide, and a reader of a definition can tell which plugin a call needs. What a plugin adds to queries and controls is kept on the SDK instance and put on each entity as its app creates it, never on a shared prototype. Their types come from one registry: see [Types](#types).

**A plugin that is not installed adds nothing.** Its members are absent, not stubs that reject: the kernel knows no plugin by name, so it could not stub one written after it. A definition that uses an optional plugin checks first:

```js
if (Anfra.plugins.has('export')) {
  button.onclick = () => query.export({ format: 'csv' })
}
```

`Anfra.plugins` lists the installed plugins and their versions. It replaces `Anfra.features`.

### Types

The members plugins add are dynamic, but an author writing TypeScript still sees them, typed and optional, as they are at runtime. Types only matter while writing: nothing here changes what runs.

**One registry, `AnfraPlugins`** (`web/sdk/src/app/pluginTypes.ts`), maps each plugin's name to what it adds where: `anfra` for `Anfra.<plugin>`, `query` for `query.<plugin>`, `control` for `control.<plugin>`. `Sdk`, `Query` and `Control` take their plugin members from it: each optional and named after its plugin, so the types say what the runtime does, a member is there only where its plugin is. anfra's own plugins are in it. A plugin outside the SDK adds its entry from the declarations its package ships, by module augmentation:

```ts
declare module '@holistics/anfra-sdk/app' {
  interface AnfraPlugins {
    notes: { anfra: { add: (text: string) => Promise<void> }, query: (text: string) => void };
  }
}
```

and `Anfra.notes` and `query.notes` are typed wherever that file is in the author's project, beside `@holistics/anfra-sdk/ambient`. One entry per plugin keeps the name it registers under and the names of its members from drifting apart, as they would if each plugin declared its members on `Sdk` and `Query` by hand.

**Checking the member narrows it.** `if (Anfra.render) Anfra.render(…)` and `query.export?.()` type-check under `strict`, and are right at runtime too. `Anfra.plugins.has(name)` completes the names in the registry and takes any other, but narrows nothing: a check on one object can't make another's member present, and a query's members would stay optional anyway. So `has` is for deciding what to offer (a button, a menu item), and the member's own check is for calling it. No accessor (`Anfra.plugins.get('export')`) is added for this: the member check already does it, with nothing new to learn.

**Later, if wanted:**

- **A host declaring what it installs.** A host that always installs `export` could ship declarations marking it present (`interface AnfraInstalled { export: true }`), so its members become required. A definition relying on that is type-safe only on hosts that install them, which is the trade-off `has` already makes at runtime. Additive to the registry.
- **Where TypeScript is compiled.** A definition is one HTML file of plain scripts, so TypeScript needs a build step, the author's or the host's. That touches "nothing to build or deploy" ([data-apps.md](data-apps.md#why-one-html-file)) and is a decision of its own. The types above serve either.

### The plugin layer

The SDK supplies data and state; the author owns the page. Plugins draw only in the **plugin layer**, a layer the runtime owns above the definition's page: inspect's overlay, a modal showing a query's rows. Each plugin draws in its own shadow root, so the author's styles don't reach it and its styles don't reach the page. Plugins never insert anything into the definition's markup. A rendering never draws the plugin layer, which is how rendering leaves inspect's overlay out without knowing it.

UI that doesn't need the page's geometry (a chat panel, a settings form) belongs in the host's own page, drawn by the host with what the plugin's host half provides.

## Packaging

anfra's plugins are `@holistics/anfra-sdk/plugins` (`corePlugin`, `exportPlugin`, …), released with the kernel: a host always carries the frame halves it was built with, as it carries the app runtime ([data-apps.md](data-apps.md#the-pieces)), so frame and host never disagree. Each is a plugin object:

```ts
{ name, version, requires, host: { calls, middleware, mount }, frame: { setup, body? } }   // setup and body: script source
```

Their frame halves are built by the SDK's build (`web/sdk/tsup.config.ts`): `src/plugins/<name>/frame.ts` as an IIFE leaving its exports in `__anfraPlugin`, `body.ts` in `__anfraBody`. `definePlugin` (`@holistics/anfra-sdk/host`) checks a plugin's names; a plugin outside the SDK builds its frame half the same way (the SDK README says how). That shape is the contract for plugins outside the SDK. A first-party plugin that outgrows the SDK's release pace can move to a package of its own without changing it.

## What was decided against

- **Services** (cordis, Koishi; Fastify's decorators): plugins handing each other objects. Every use of another plugin would bypass middleware, so a platform's quota would miss the queries an agent runs; and services bring a dependency graph and lifecycle a host that installs its plugins once doesn't need.
- **Plugin-defined hooks** (Tapable, Rollup's hooks): extension points through which plugins change each other's behaviour. Middleware covers the host; in the frame a hook enforces nothing. The one need for fan-out (many plugins contributing to one, such as tools for an agent) is an extension point a plugin owns, decided with the agent plugin.
- **Hot reload**: plugins are installed when a page loads.
- **Scoping a plugin's middleware to its own calls**: a plugin is trusted with the page anyway (its host half could make any call itself), so a scope would read as a boundary it isn't. A host that installs a plugin it trusts less wraps it, or doesn't install it.
- **Plugins adding to the definition's markup** (a "view data" button on every block): the definition is everything the author wrote. If wanted, an attribute the author writes opts a block in.
- **Definitions declaring the plugins they need**, refused at mount when one is missing: additive, if hosts want it.

The trade-off: whatever cannot cross a call, such as a live object or a hook into each render, cannot be shared between plugins.
