# @holistics/anfra-canvas

A prototyping canvas for a Vue app's dev server. The app's real screens, and prototypes written as
real components of the app, sit on one board as live frames, next to sections, arrows and notes that
show the flow: the happy path and each edge case. People edit one element at a time as in a design
tool, and every edit is written to the element's source: its component's props, its text, its
layout, spacing, size, type and fill (as the theme's Tailwind classes), its place among its
siblings. They drop the app's own components in from a palette that draws them, and find any layer
in each screen's tree. For a flow, or a change across screens, they leave a comment: Claude reads
the comments over MCP, changes the code, and resolves them.

It is dev tooling: a Vite plugin that applies to `vite serve` only, so nothing of it reaches a build.
anfra Cloud's web app uses it today (`pnpm dev:canvas`); any Vite + Vue app can.

## Set it up

### What you need

- **Node 22.18 or newer** (`node --version`). The canvas is TypeScript that Node runs as is, which
  older Nodes can't do. With nvm: `nvm install 22 && nvm use 22`.
- **pnpm 10**: `corepack enable` turns on the version each repo pins.
- **git**, with access to the repos you're setting up (for anfra Cloud: `holistics/anfra` and
  `holistics/anfra-cloud`).
- **Claude Code**, to have Claude work through the comments (optional: **Copy feedback** works with
  any AI).

### For anfra Cloud

The canvas lives in anfra (`web/canvas`), and anfra Cloud's web app loads it from a checkout of anfra
beside it. So clone the two repos next to each other:

```sh
mkdir -p ~/holistics && cd ~/holistics
git clone git@github.com:holistics/anfra.git
git clone git@github.com:holistics/anfra-cloud.git
```

Until the canvas is merged, it's on the `feat/canvas-mode` branch in both:

```sh
git -C anfra switch feat/canvas-mode
git -C anfra-cloud switch feat/canvas-mode
```

Install each one's packages, then start the app with the canvas:

```sh
(cd anfra && pnpm install)
cd anfra-cloud/web && pnpm install
pnpm dev:canvas
```

It prints three lines; open the **Canvas** one (`http://127.0.0.1:5173/__canvas/`, or the next free
port). The screens answer the API from mock scenarios, so nothing else needs to run: no backend, no
database. A frame set to the `live` scenario talks to a local anfra-bi instead, as `pnpm dev` does.

An anfra checkout somewhere else works too: point `ANFRA_CANVAS_DIR` at its `web/canvas`
(`ANFRA_CANVAS_DIR=~/code/anfra/web/canvas pnpm dev:canvas`).

Next time, only the last command is needed: `cd anfra-cloud/web && pnpm dev:canvas`. After pulling,
run `pnpm install` again in both if packages changed.

### In another Vite + Vue app

The app needs Vite 7 or newer, Vue 3.5 and, for the inspector's styling controls, Tailwind. Add the
plugin to its Vite config, loaded from an anfra checkout (the package isn't published), and only
when asked for, so `pnpm dev` stays as it was:

```ts
// vite.config.ts
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

async function canvas () {
  if (!process.env.ANFRA_CANVAS) return [];
  const dir = resolve(process.env.ANFRA_CANVAS_DIR ?? '../anfra/web/canvas');
  const { anfraCanvas } = await import(pathToFileURL(join(dir, 'src/vite/index.ts')).href);
  return [
    anfraCanvas({
      boards: 'canvas/boards',                  // one JSON file per board, committed
      scenarios: 'src/prototypes/scenarios.ts', // mock API answers, by scenario (optional)
      apiBase: '/api',                          // requests below it are answered by a frame's scenario
      styles: ['/src/main.css'],                // the app's tokens and fonts, for the canvas's own UI
      prototypes: { dir: 'src/prototypes', route: '/__proto' }, // optional, see below
      components: 'src/components/ui',          // the palette: one folder per component
      snippets: 'canvas/snippets.ts',           // optional: the markup each palette item inserts
      routes: ['/', '/settings'],               // offered when adding a screen
      instructions: 'How this app is built, for Claude',
    }),
  ];
}

export default defineConfig(async () => ({
  plugins: [vue(), tailwindcss(), ...(await canvas())],
}));
```

Then add `"dev:canvas": "ANFRA_CANVAS=1 vite"` to the app's `package.json` scripts and run it. The
plugin applies to `vite serve` only, so nothing of it reaches a build.

- **Prototypes** (screens that aren't in the app yet) are `.vue` files in `prototypes.dir`. The app
  serves them at `prototypes.route` with a dev-only route that renders the file named by the path,
  inside the app's layout if you mount it there: see anfra Cloud's `src/prototypes/PrototypeHost.vue`
  and the `__proto` routes in its `src/router.ts`.
- **Scenarios** answer the app's API in frames, so screens show any state without a backend: see
  [Scenarios](#scenarios). Without them, frames call the real API through the dev server.
- Everything else is optional; the comments a person leaves for Claude work on any screen.

### Connect Claude

From the app's repo, once, with the line the dev server printed:

```sh
claude mcp add --transport http anfra-canvas http://127.0.0.1:5173/__canvas/mcp
```

Then, in Claude Code in that repo, ask it to "handle the open feedback on the canvas". It reads the
comments, changes the code they point at, and resolves each one with a note. Without Claude Code,
**Copy feedback** puts every open comment on the clipboard as one Markdown task for any AI.

### Your first five minutes

1. Pick the board at the top left, or make one with **+**.
2. Press **F** and click the board to add a screen: a route of the app, a scenario, a size.
3. Click any element in a screen to pick it. The panel on the right edits its props, text, layout,
   spacing, size, type and fill, and each change is written to the `.vue` file it's in. **⌘Z** undoes
   it.
4. Press **C** and click an element to leave a comment for Claude.
5. Press **A** and drag from one screen to another to draw the flow; **⌃⌥T** tidies the board from
   the arrows, and **Play** walks it like a prototype.
6. **⌘K** finds every command, screen, board and component; right-click shows what you can do with
   what's under the pointer.

### When something is off

| What you see | Why, and what to do |
| --- | --- |
| `pnpm dev:canvas` fails with `ERR_UNKNOWN_FILE_EXTENSION ".ts"` or a syntax error in `src/vite/index.ts` | Node is older than 22.18. Update it |
| `Cannot find module …/web/canvas/src/vite/index.ts` | The anfra checkout isn't beside the app (`../anfra`). Clone it there, or set `ANFRA_CANVAS_DIR` |
| `/__canvas/` shows the app, or its 404 | The dev server was started with `pnpm dev`, not `pnpm dev:canvas` |
| A screen is blank or stuck loading | A call its scenario doesn't answer: the browser console names the op (a 501). Add it to the scenario, or pick a scenario that has it |
| Errors about missing packages in the canvas | Run `pnpm install` in the anfra checkout too, not only in the app |
| Claude doesn't see the comments | The MCP server is added per repo: run the `claude mcp add` line in the app's repo, with the dev server running, and check `claude mcp list` |
| A picked element shows only **Comment**, no controls | It's drawn at run time (text from data, say) rather than written in a template, or its file is outside `src/`: leave a comment for Claude instead |

## Set it up with an AI

Paste this into Claude Code (or another coding agent that can run commands on your computer). It sets
the canvas up for anfra Cloud and tells you how to open it.

```text
Set up the anfra canvas for me on this computer: anfra's prototyping canvas, run inside the anfra Cloud
web app. Do each step, check it worked, and fix what you can before moving on.

1. Check the tools: Node 22.18 or newer (if older, install Node 22 with nvm, or tell me how), pnpm 10
   (run `corepack enable` if pnpm is missing), git, and that I can reach github.com/holistics/anfra and
   github.com/holistics/anfra-cloud (`git ls-remote` on each). If I can't, stop and tell me to ask for
   access.
2. Find or make the checkouts. If I already have anfra and anfra-cloud cloned, use those; otherwise
   clone both into ~/holistics. The two folders must sit side by side (anfra-cloud/../anfra), or set
   ANFRA_CANVAS_DIR to anfra's web/canvas when you run it.
3. In both, switch to the branch feat/canvas-mode if it exists (otherwise stay on main). Don't throw
   away any changes of mine: if a checkout has uncommitted work, stop and ask me first.
4. Install packages: `pnpm install` at the root of anfra, then `pnpm install` in anfra-cloud/web.
5. Start it in the background: `pnpm dev:canvas` in anfra-cloud/web. Wait for the line starting with
   "Canvas:" and note its URL and the "MCP:" line under it.
6. Check it: fetch <canvas URL>api/boards and expect a JSON list. If the server failed, read its output,
   fix the cause (often the Node version or a missing install), and start it again.
7. Connect Claude Code: run the "MCP:" line (claude mcp add --transport http anfra-canvas …) from the
   anfra-cloud folder, then confirm with `claude mcp list`.
8. Open the canvas URL in my browser.

Don't edit, commit or push anything in either repo. When you're done, tell me in a few lines: the URL,
the one command to start it next time, and how to ask Claude to handle my comments on the canvas.
```

For another Vite + Vue app, ask the agent to "add the anfra canvas to this app following
web/canvas/README.md in holistics/anfra, section *In another Vite + Vue app*", with the anfra checkout's
path.

## How it works

| Part | Where | What it does |
| --- | --- | --- |
| Board model | `src/model/` | Boards, frames, sections, notes, arrows and comments as plain JSON; immutable ops; a three-way merge; feedback as Markdown; mock scenarios |
| Source tags | `src/vite/source.ts` | A Vue compiler transform that writes `data-canvas-src="file:line:col"` on every element, so a picked element knows where it is written. Edits (text, classes, attributes and props, remove, duplicate, move, insert with its imports) are applied to that spot in the file |
| Catalog | `src/vite/catalog.ts` | The app's components (props from `defineProps`, variants from `cva`), the palette, and the theme's tokens from its CSS |
| Frame agent | `src/frame/` | Injected into the app's page. Inside a canvas frame (an iframe named by the canvas) it answers the API from the frame's scenario, reports hover, pick and layout, and edits text in place. Elsewhere it does nothing |
| Server | `src/vite/index.ts` | `/__canvas/` (the canvas), `/__canvas/api/` (boards, source, edits), `/__canvas/mcp` (MCP over streamable HTTP, JSON answers). Same origin only |
| MCP tools | `src/vite/mcp.ts` | `list_feedback`, `resolve_feedback`, `reply_feedback`, boards, frames, sections, notes, arrows, `tidy_board`, scenarios |
| Canvas UI | `src/app/` | The board: pan, zoom, select, move, resize, draw sections and arrows, comment; the element inspector, layers and palette; the command palette (⌘K) and right-click menus (`commands.ts`); undo across board and source edits; saves with a rev and merges when Claude saved first |
| UI kit | `src/app/ui/` | The canvas's design system: its tokens and every control in one sheet (`ui.css`), and the components panels are built from (Button, Field, Select, Segmented, Switch, MenuItem, TreeRow, Section, PropRow, Tooltip). See below |
| Connectors | `src/app/route.ts`, `connectors.ts` | Right-angled routes around the screens (A* on a sparse grid, where a bend costs extra), each remembered with the screens near it |
| Tidy up | `src/model/layout.ts` | Lays the screens out from the arrows: the main path in a row, each branch under the screen it leaves, sections sized to hold theirs |

Only screens near the view run. A frame's app starts when it first comes near the view and stops
drawing while it is far; past twelve running, one far for a while is let go and a still of it
(`src/frame/snapshot.ts`) stays in its place.

Frames are same-origin iframes of the app, so the canvas and the frame agent call each other
directly (`window.__anfraCanvasHost`), with no message passing.

## The UI kit

Every panel, bar, menu and popover is built from `src/app/ui/`, and no component has a style block of
its own. `ui/ui.css` holds the tokens (taken from the host app's `--anfra-color-*`, so the canvas wears
its theme) and every control; `canvas.css` only lays the app out and draws the board.

- Two control sizes: 32px in panels and popovers, 24px (`small`) on the board's own labels.
- One edge for every button, field and select (`--ac-line-control`), darker on hover. A field or
  select shows focus on its edge; anything else takes the 2px violet outline.
- Three type sizes (12, 14, 20), three radii (6, 10, 14), e1 resting and e2 floating.
- Tooltips are the kit's, never a `title`: a Button's `label` or `tip` with its `keys`, or `v-tip` on
  anything else, shows the name and the shortcut.
- A property in a panel is a `PropRow` (its name in a 72px column) in a `.ac-props` group, inside a
  `Section`.

To add a control, add it to the kit first, then use it.

## Scenarios

A scenario answers the app's API ops (`POST /api/<op>` or `/api/o/<org>/<op>`) with fixed bodies,
or with a function of the request. `{ $status, $body, $delay }` answers an error or a slow call.
Every scenario extends `default` unless it says `extends: false`. An op a scenario doesn't answer
returns 501 and a warning names it. The scenario `live` turns mocks off and uses the real backend.

## Develop

```sh
pnpm --filter @holistics/anfra-canvas test
pnpm --filter @holistics/anfra-canvas typecheck
```

The package is TypeScript run as is (Node's type stripping and Vite), with no build step: keep to
erasable syntax and import with `.ts` extensions.
