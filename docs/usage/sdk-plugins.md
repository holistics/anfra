# Writing an SDK plugin

This guide is for adding a feature to the Data Apps on your own host, such as a call from the frame to your server or an API for the author. Read [designs/sdk-plugins.md](../designs/sdk-plugins.md) first for what an SDK plugin is and why it works this way; this guide uses its terms (host half, frame half, kernel, origin).

You need a host built on `@holistics/anfra-sdk/host` (appserve, or your own). Building a frame half takes Node.js and `esbuild`, and testing one takes a DOM test environment such as jsdom or happy-dom.

The example throughout is a `notes` plugin. Readers leave notes on a query, the host stores them wherever it likes, and the plugin shows them in a panel over the page. Its full code, with its tests, is in [`web/sdk/examples/notes/`](../../web/sdk/examples/notes/).

## 1. Do you need a plugin, and which halves?

| You want | You need |
|---|---|
| The frame to reach something on the host's side (your API, a file save, a model) | a host half with a call the frame may make |
| The host page to watch or change calls (logging, quota, a cache) | no plugin: middleware on `createHost` |
| An API for the author (`Anfra.x`, `query.x`) | a frame half |
| UI over the page tied to its layout (a panel by a block, a highlight) | a frame half, drawing in the plugin layer |
| A command for the host's action bar or palette, or a tab in its panel | contributions on the host half (`commands`, `tabs`): each host draws them its own way |
| UI that doesn't need the page and isn't a command or a tab (a chat window, settings) | the host's own UI, beside the frame; maybe a host half for its data |
| The host to ask the frame for something (draw, measure) | a frame half that answers the host's call |

`notes` has both halves. Its host half reads and writes notes, and its frame half adds `Anfra.notes` and `query.notes` and draws the panel.

## 2. Names

A plugin's name is lowercase letters, digits and dashes (`notes`, `acme-share`). It names everything the plugin adds: its calls and events (`notes.add`, `notes.viewed`), and its API (`Anfra.notes`, `query.notes`). `anfra` and `plugins` are reserved. Call and event names are letters and digits.

## 3. The host half

```ts
import { ValidationError } from '@holistics/anfra-sdk/common';
import { definePlugin, type FrameHalf, type Plugin } from '@holistics/anfra-sdk/host';

export interface NotesStore {                                  // where notes live: the host's to decide
  list: (about: string, signal: AbortSignal) => Promise<Note[]>;
  add: (about: string, text: string, signal: AbortSignal) => Promise<Note>;
}

export function createNotesPlugin (store: NotesStore, frame: FrameHalf): Plugin {
  return definePlugin({
    name: 'notes',
    version: '1.0.0',
    host: {
      calls: {
        list: { frame: true, handler: (request, call) => store.list(about(request), call.signal) },
        add: { frame: true, handler: (request, call) => store.add(about(request), text(request), call.signal) },
      },
      events: {
        viewed: (payload) => (isViewed(payload) ? payload : undefined),   // a frame's event, checked
      },
    },
    frame,
  });
}
```

### Calls

Each call is a handler, `(request, call) => answer`, named under the plugin: `list` here is `notes.list`.

Set `frame: true` to let the frame make the call, and leave it out for a call only the host or another plugin makes. Whatever the frame may call, any definition may call too, because the frame is the definition's realm.

Check every request, because the definition makes it up. In the example, `about()` and `text()` throw a `ValidationError` for anything that isn't a short string.

The handler's second argument, `call`, is the call's context:

| Field | Is |
|---|---|
| `signal` | Aborts when the caller cancels. Pass it on to your fetches. |
| `caller` | Who made the call: `{ kind: 'frame' }`, `{ kind: 'host' }` or `{ kind: 'plugin', plugin }`. |
| `origin`, `chain` | Who started it, and every hop since. A call your plugin makes for the frame has your plugin as `caller` and the frame as `origin`. |
| `mount` | The frame it's about: its `document` (the host page's), `call`, `emit`, `on`, `onReady`. |
| `call(name, request?, { signal?, asOrigin? })` | Make another plugin's call through the pipeline, as your plugin. With `asOrigin: true`, it is refused if the call's origin (the frame) couldn't make it itself: use it when you forward a name or request the frame chose. |

### Errors

Throw one of the SDK's classes (`ValidationError`, `QueryError`, `PermissionError`, `TransportError`, from `@holistics/anfra-sdk/common`) for a failure the author should read. Any other error reaches the frame only as `QueryError: The call failed.`, which keeps a server's message, a URL or an id from leaking.

### Events

A frame's event reaches the host only if your plugin declares it in `events`, with a check that returns what listeners should get, or `undefined` to drop the event. The host's code listens with `mounted.on('notes.viewed', …)`.

### State

`mount(mountContext)` runs when a frame is mounted, and may return a function to run when the frame is unmounted. Keep per-frame state in a `WeakMap` keyed by the mount context, as `render`'s rate limit does. Anything that must outlive the page goes in a store the host passes in, as `notes` does.

### Commands and tabs

A host half can add UI to the host page as data, which each host draws in its own chrome: commands for its action bar and palette, and tabs for the panel it shows beside each Data App. `notes` adds a tab that lists every note, and a command that opens it:

```ts
host: {
  commands: [{ id: 'open', title: 'Notes', icon: 'note', shortcut: 'Alt+Shift+N', run: { tab: 'notes.list' } }],
  tabs: [{
    id: 'list',
    title: 'Notes',
    // Load what is heavy here: a host that never shows the tab never loads it.
    mount: async (root, context) => (await import('./list-tab')).draw(root, context),
  }],
},
```

- A tab draws into `root`, a shadow root, with any framework or none, and returns what takes it down. `context.panel` has the frame's snapshot, the node selected (shared with every other tab), and highlight, locate and pick; `context.call` makes calls as the plugin.
- Style it with the host's theme, `var(--anfra-color-bg)` and the rest (see [designs/sdk-plugins.md](../designs/sdk-plugins.md#plugin-ui)), so it looks right in every host.
- A command runs a call (`run: { call: 'notes.export' }`) or opens a tab. Give it `state` when it can be on or off, or can't run: `{ disabled: 'Sign in to export notes' }` always says why. Call `mount.commandsChanged()` when that changes.
- The host may leave any of it out (`createHost({ contributions })`), so nothing may depend on a contribution being shown: the calls stay either way.

### Middleware

Middleware a plugin brings (`host.middleware`) wraps every call on the page, including other plugins' calls. Bring it only when the plugin's job covers the whole page, as a logger's does, and otherwise leave middleware to the host.

## 4. The frame half

A frame half is a module whose default export takes the kernel. It runs once, before the definition, and imports only types, because everything it needs comes through the kernel.

```ts
import type { FrameKernel, Query } from '@holistics/anfra-sdk/app';

export default function setup (kernel: FrameKernel): void {
  const list = (about: string) => kernel.call<Note[]>('notes.list', { about });
  const add = (about: string, text: string) => kernel.call<Note>('notes.add', { about, text });

  async function show (about: string) {
    const notes = await list(about);
    const root = kernel.layer();                                  // this plugin's shadow root over the page
    const panel = kernel.window.document.createElement('aside');
    for (const note of notes) {
      const item = kernel.window.document.createElement('li');
      item.textContent = note.text;                               // text, never HTML
      panel.append(item);
    }
    root.replaceChildren(panel);
    kernel.emit('viewed', { about, count: notes.length });       // notes.viewed
  }

  kernel.provide({ list, add, show });                            // Anfra.notes
  kernel.provideOnQuery((query: Query) => ({                      // query.notes, on every query
    list: () => list(query.name),
    add: (text: string) => add(query.name, text),
    show: () => show(query.name),
  }));
}
```

What the kernel gives a setup:

| Member | Does |
|---|---|
| `provide(api)` | Adds `Anfra.<plugin>`. Once. |
| `provideOnQuery(factory)`, `provideOnControl(factory)` | Adds `query.<plugin>` / `control.<plugin>` to every one, made the first time it is read. |
| `call(name, request?, signal?)` | Makes a call by its full name (`notes.list`, `core.submitQuery`): any call the frame may make. |
| `emit(name, payload?)` | Sends the host one of this plugin's events, by its short name. |
| `handle(name, handler)` | Answers a call the host makes into the frame (`mounted.call('notes.focus')`). |
| `on(name, listener)` | Listens to an event the host sends. |
| `onApp(listener)` | Each app the definition creates, and those already created. Watch one with `app.subscribe`. |
| `sdk` | `Anfra` itself: `apps`, `datasets`, `user`. |
| `layer()` | This plugin's shadow root in the plugin layer. Draw only here. |
| `overlay`, `structure()` | The kernel's highlight overlay, and the page's structure (its marked blocks and containers). |
| `body()` | The plugin's body, fetched and run on first use. |
| `errors` | The SDK's error classes. Reject with these, so authors can tell them apart. |
| `window` | The frame's window. |

Draw only in `layer()`. It is a shadow root, so the page's styles don't reach your UI and yours don't reach the page, and renderings leave it out. Never insert anything into the definition's markup.

Put code too big to load in every frame, such as a renderer or a chart library, in a body: a module whose default export is what `kernel.body()` resolves to. Call `kernel.body()` when the code is first needed.

### Build it

A frame half reaches the frame as script source. Build it with `buildFrameHalf` (needs `esbuild` installed), and give the result to the plugin as `frame`:

```js
// build.mjs, run before your package's own build
import { writeFile } from 'node:fs/promises';
import { buildFrameHalf } from '@holistics/anfra-sdk/build';

const frame = await buildFrameHalf({ setup: 'src/frame.ts' });   // body: 'src/body.ts' if it has one
await writeFile('src/frame-half.generated.ts', `export default ${JSON.stringify(frame)};\n`);
```

```ts
// src/index.ts, the package's entry
import frame from './frame-half.generated';
import { createNotesPlugin, type NotesStore } from './plugin';

export const notesPlugin = (store: NotesStore) => createNotesPlugin(store, frame);
```

`buildFrameHalf` refuses a frame half that imports the core API's client or the host's code (`@holistics/anfra-sdk/api`, `/host`, `/plugins`), since the frame may reach data only over the bridge.

## 5. Types

Ship an entry in the SDK's plugin registry with your package's types, so a TypeScript author sees `Anfra.notes` and `query.notes`:

```ts
declare module '@holistics/anfra-sdk/app' {
  interface AnfraPlugins {
    notes: { anfra: NotesApi, query: QueryNotes };   // anfra: Anfra.notes; query, control: on each one
  }
}
```

The members are optional, as they are at runtime, and an author checks a member before using it: `if (Anfra.notes) …`, `query.notes?.add(…)`.

## 6. Install it

```ts
import { createHost } from '@holistics/anfra-sdk/host';
import { corePlugin } from '@holistics/anfra-sdk/plugins';
import { notesPlugin } from '@acme/anfra-notes';

const host = createHost({
  plugins: [corePlugin(backend), notesPlugin(myNotesStore)],
});
const mounted = host.mount({ container, definition, datasets, user });
mounted.on('notes.viewed', ({ about, count }) => analytics.track('notes viewed', { about, count }));
```

A plugin that needs another lists it in `requires` (`requires: ['export']`), and the host refuses to start without it. In the frame, an author checks `Anfra.plugins.has('notes')` before offering anything that needs the plugin.

## 7. Test it

Test the host half through a host runtime: `host.call(name, request)` runs the call through the pipeline as the host would. Call an event's check directly.

```ts
const host = createHost({ plugins: [corePlugin(backend), createNotesPlugin(store, { setup: '' })] });

await host.call('notes.add', { about: 'revenue', text: 'Q3 includes the refund.' });
await expect(host.call('notes.add', { about: 'revenue', text: '' })).rejects.toBeInstanceOf(ValidationError);
```

Test the frame half in a test frame from `@holistics/anfra-sdk/testing`. It runs your setup on the real kernel and bridge, against a stand-in host whose answers you give.

```ts
import { testFrame, flush } from '@holistics/anfra-sdk/testing';
import setup from './frame';

const frame = testFrame({
  setups: [['notes', setup]],
  answers: { 'notes.list': () => notes },                       // what the host answers
});

await frame.anfra.notes!.show('revenue');
await flush();
expect(frame.calls).toEqual([{ name: 'notes.list', request: { about: 'revenue' } }]);
expect(frame.events).toEqual([{ name: 'notes.viewed', payload: { about: 'revenue', count: 1 } }]);
```

`testFrame` also takes `datasets`, `user`, and `bodies`, which gives what each body exports, since a test DOM runs no injected script. Use `frame.call` and `frame.emit` to act as the host calling into the frame or sending it an event.

## 8. Before you ship

- [ ] Only the calls the frame needs have `frame: true`.
- [ ] Every frame-callable handler checks its request, and every declared event checks its payload.
- [ ] A handler that forwards a name or request the frame chose calls with `{ asOrigin: true }`.
- [ ] Failures the author should read throw the SDK's error classes, since other errors reach the frame without their message.
- [ ] The frame half draws only in `layer()`, sets text with `textContent`, and imports only types.
- [ ] Anything to outlive the page is in a store the host passes in.
- [ ] The host knows that installing the plugin trusts its host half with the whole page.
