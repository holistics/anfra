# @holistics/anfra-canvas

A prototyping canvas for a Vue app's dev server. The app's real screens, and prototypes written as
real components of the app, sit on one board as live frames, next to sections, arrows and notes that
show the flow: the happy path and each edge case. People pick an element in a frame and comment on
it, edit its text or classes in place, or copy it as Markdown. Claude reads the comments over MCP,
changes the code, and resolves them.

It is dev tooling: a Vite plugin that applies to `vite serve` only, so nothing of it reaches a build.

## Use it in an app

```ts
// vite.config.ts
import { anfraCanvas } from '@holistics/anfra-canvas/vite';

export default defineConfig({
  plugins: [
    vue(),
    anfraCanvas({
      boards: 'canvas/boards',                  // one JSON file per board, kept in the repo
      scenarios: 'src/prototypes/scenarios.ts', // mock API answers, by scenario
      styles: ['/src/main.css'],                // the app's tokens, for the canvas's own UI
      prototypes: { dir: 'src/prototypes', route: '/__proto' },
      components: 'src/components/ui',          // the palette
      instructions: 'How this app is built, for Claude',
    }),
  ],
});
```

Then open `/__canvas/` on the dev server, and connect Claude once, from the app's repo:

```sh
claude mcp add --transport http anfra-canvas http://127.0.0.1:5173/__canvas/mcp
```

The dev server prints both.

## How it works

| Part | Where | What it does |
| --- | --- | --- |
| Board model | `src/model/` | Boards, frames, sections, notes, arrows and comments as plain JSON; immutable ops; a three-way merge; feedback as Markdown; mock scenarios |
| Source tags | `src/vite/source.ts` | A Vue compiler transform that writes `data-canvas-src="file:line:col"` on every element, so a picked element knows where it is written. Text and class edits are applied to that spot in the file |
| Frame agent | `src/frame/` | Injected into the app's page. Inside a canvas frame (an iframe named by the canvas) it answers the API from the frame's scenario, reports hover, pick and layout, and edits text in place. Elsewhere it does nothing |
| Server | `src/vite/index.ts` | `/__canvas/` (the canvas), `/__canvas/api/` (boards, source, edits), `/__canvas/mcp` (MCP over streamable HTTP, JSON answers). Same origin only |
| MCP tools | `src/vite/mcp.ts` | `list_feedback`, `resolve_feedback`, `reply_feedback`, boards, frames, sections, notes, arrows, scenarios |
| Canvas UI | `src/app/` | The board: pan, zoom, select, move, resize, draw sections and arrows, comment, inspect; undo; saves with a rev and merges when Claude saved first |

Frames are same-origin iframes of the app, so the canvas and the frame agent call each other
directly (`window.__anfraCanvasHost`), with no message passing.

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
