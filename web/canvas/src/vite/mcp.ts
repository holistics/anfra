// The canvas as MCP tools, so an agent (Claude Code, Claude Desktop) reads the feedback people
// leave, answers it, and lays out boards. Served by the dev server at /__canvas/mcp over MCP's
// streamable HTTP transport, answering each request with one JSON response. The agent changes the
// app's code with its own tools; these tools change only boards.
import { find, isShape } from '../model/ops.ts';
import * as ops from '../model/ops.ts';
import { feedbackMarkdown } from '../model/markdown.ts';
import { tidy } from '../model/layout.ts';
import type { Board, Comment, Frame, Rect, Section, Shape } from '../model/board.ts';
import type { Scenarios } from '../model/scenario.ts';
import { SLUG, type BoardStore } from './boards.ts';
import type { TemplateSummary } from '../model/template.ts';
import type { StartFrom, Started } from './templates.ts';
import type { BoardFiles } from './board-files.ts';

export const PROTOCOL_VERSION = '2025-06-18';

export interface McpContext {
  store: BoardStore;
  scenarios: () => Promise<Scenarios>;
  /** The routes a frame can show: the prototypes', then any others the app offers. */
  screens?: () => Promise<{ prototypes: string[], routes: string[] }>;
  /** What the agent is told on connecting: how prototypes are written in this app. */
  instructions: string;
  /** The canvas's address, for links to a board. */
  url: string;
  /** What deleting a board would delete, and deleting it (with its screens' folder when asked). */
  boardFiles?: (slug: string) => Promise<BoardFiles>;
  deleteBoard?: (slug: string, withFolder: boolean) => Promise<BoardFiles>;
  /** The templates to start a prototype from, when the app has any. */
  templates?: {
    list: () => Promise<TemplateSummary[]>;
    read: (id: string) => Promise<TemplateSummary>;
    start: (req: StartFrom) => Promise<Started>;
  };
}

interface Tool {
  description: string;
  input: Record<string, unknown>;
  required?: string[];
  run: (args: Record<string, any>, ctx: McpContext) => Promise<string>; // eslint-disable-line @typescript-eslint/no-explicit-any
}

const str = (description: string) => ({ type: 'string', description });
const num = (description: string) => ({ type: 'number', description });
const board = str('The board\'s name, as list_boards gives it');

const AGENT = 'Claude';

/** How the canvas works, told to every agent after the app's own instructions. */
export const GUIDE = `How the canvas works:
- A board (list_boards, get_board) holds frames, sections, notes and arrows. A frame is a live screen: the app at a route, in an iframe, in one scenario (a mock state of its API, list_scenarios).
- To start a new prototype, start from a template when the canvas has any, unless asked for a blank board: list_templates, then create_from_template. It copies the template's screens into the project, where they are yours to change, and lays them out on a board.
- The screens a frame can show are in list_screens. A route can carry ?open=<name> to show an overlay open, for apps whose overlays read it.
- To lay out a flow: create_board (with its project), add_frame for each screen, add_arrow between them with the action as the label, add_section around a happy path or edge cases, then tidy_board. Arrows are also how Present steps from screen to screen.
- People pick elements in frames and edit them; each edit is written to the .vue file. What they can't do in the panel they leave as a comment: list_feedback says what each points at (frame, route, scenario, element, file and line). Change the code, then resolve_feedback with a short note, or reply_feedback to ask.
- The canvas reloads a frame as its files change, so there is no need to tell it. Its tools change boards only: change code with your own tools.`;

function summary (b: Board) {
  const shape = (s: Shape) => ({ id: s.id, ...(s.kind === 'note' ? { text: s.text } : { name: s.name }), x: s.x, y: s.y, width: s.width, height: s.height });
  return {
    name: b.name,
    sections: b.sections.map((s) => ({ ...shape(s), contains: ops.contents(b, s).map((c) => c.id) })),
    frames: b.frames.map((f) => ({ ...shape(f), route: f.route, scenario: f.scenario ?? 'live' })),
    notes: b.notes.map(shape),
    arrows: b.arrows.map(({ from, to, label, id }) => ({ id, from, to, ...(label ? { label } : {}) })),
    comments: { open: b.comments.filter((c) => c.status === 'open').length, resolved: b.comments.filter((c) => c.status === 'resolved').length },
  };
}

/** A spot for a new frame inside a section: right of the frames already there, else its corner. */
function spotIn (b: Board, sectionId: string | undefined, size: { width: number, height: number }) {
  const section = sectionId ? b.sections.find((s) => s.id === sectionId) : undefined;
  if (!section) return ops.nextSpot(b);
  const inside = ops.bounds(ops.contents(b, section));
  const at = inside ? { x: inside.x + inside.width + 80, y: inside.y } : { x: section.x + 80, y: section.y + 120 };
  // A new row leaves room above for the frames' labels, which keep their size as the canvas zooms out.
  return at.x + size.width > section.x + section.width ? { x: section.x + 80, y: (inside ? inside.y + inside.height : section.y) + 160 } : at;
}

/** Make a section big enough to hold a shape put in it, with room around. */
function grow (b: Board, sectionId: string | undefined, r: Rect): Board {
  const section = sectionId ? b.sections.find((s) => s.id === sectionId) : undefined;
  if (!section) return b;
  const right = Math.max(section.x + section.width, r.x + r.width + 80);
  const bottom = Math.max(section.y + section.height, r.y + r.height + 80);
  return ops.update<Section>(b, section.id, { width: right - section.x, height: bottom - section.y });
}

function templatesOf (ctx: McpContext) {
  if (!ctx.templates) throw new Error('this canvas has no templates');
  return ctx.templates;
}

const started = (r: Started, ctx: McpContext) => [
  `Copied ${r.files.length} file${r.files.length === 1 ? '' : 's'}${r.files.length ? `: ${r.files.join(', ')}` : ' (all were there already)'}.`,
  `Frames on ${r.board}: ${Object.entries(r.frames).map(([s, f]) => `${s} → ${f}`).join(', ')}.`,
  `${ctx.url}#${r.board}`,
].join('\n');

function comment (b: Board, id: string): Comment {
  const c = find(b, id);
  if (c?.kind !== 'comment') throw new Error(`no comment ${id} on ${b.name}`);
  return c;
}

const TOOLS: Record<string, Tool> = {
  list_boards: {
    description: 'The canvas boards, with how many frames and open comments each has.',
    input: {},
    run: async (_, ctx) => JSON.stringify(await ctx.store.list(), null, 2),
  },
  get_board: {
    description: 'One board: its sections (and what lies in each), frames (route, scenario, position, size), notes and arrows.',
    input: { board },
    required: ['board'],
    run: async ({ board }, ctx) => JSON.stringify(summary(await ctx.store.read(board)), null, 2),
  },
  list_feedback: {
    description: 'Comments people left on the canvas, as Markdown: what each points at (the frame, its route and scenario, the element and the file and line it is written at) and what it asks. Act on them by changing the code, then call resolve_feedback.',
    input: {
      board: str('One board; leave out for every board'),
      status: { type: 'string', enum: ['open', 'resolved', 'all'], description: 'Which comments; open by default' },
    },
    run: async ({ board, status = 'open' }, ctx) => {
      const slugs = board ? [board] : (await ctx.store.list()).map((s) => s.slug);
      const parts: string[] = [];
      for (const slug of slugs) {
        const b = await ctx.store.read(slug);
        const list = b.comments.filter((c) => status === 'all' || c.status === status);
        if (list.length || board) parts.push(`${feedbackMarkdown(b, list)}\n\n(board: \`${slug}\`, ${ctx.url}#${slug})`);
      }
      return parts.join('\n\n---\n\n') || 'No feedback.';
    },
  },
  resolve_feedback: {
    description: 'Resolve a comment once it is handled, with a short note of what changed (shown to the person under their comment).',
    input: { board, id: str('The comment\'s id'), note: str('What you changed, in a sentence or two') },
    required: ['board', 'id', 'note'],
    run: async ({ board, id, note }, ctx) => {
      await ctx.store.change(board, (b) => (comment(b, id), ops.setStatus(ops.reply(b, id, AGENT, note), id, 'resolved')));
      return `Resolved ${id}.`;
    },
  },
  reply_feedback: {
    description: 'Reply to a comment without resolving it: to ask a question, or to say what you are about to do.',
    input: { board, id: str('The comment\'s id'), body: str('The reply') },
    required: ['board', 'id', 'body'],
    run: async ({ board, id, body }, ctx) => {
      await ctx.store.change(board, (b) => (comment(b, id), ops.reply(b, id, AGENT, body)));
      return `Replied on ${id}.`;
    },
  },
  create_board: {
    description: 'A new, empty board, with the project it belongs to, as the home page lists boards by project.',
    input: {
      board: str('Its file name: lowercase letters, digits and dashes'),
      name: str('Its title'),
      project: str('The project, usually the folder its screens are in (datasets for /p/datasets/…)'),
    },
    required: ['board'],
    run: async ({ board, name, project }, ctx) => {
      if (!SLUG.test(board)) throw new Error('a board name is lowercase letters, digits and dashes');
      await ctx.store.create(board, name ?? board, { project });
      return `Created ${board}: ${ctx.url}#${board}`;
    },
  },
  list_screens: {
    description: 'The screens a frame can show: the routes of the app\'s prototypes, then any other routes it offers.',
    input: {},
    run: async (_, ctx) => {
      const s = await ctx.screens?.();
      if (!s || (!s.prototypes.length && !s.routes.length)) return 'No screens listed: a frame takes any route the app serves.';
      return [...s.prototypes.map((r) => `- ${r}`), ...(s.routes.length ? ['', 'Other routes:', ...s.routes.map((r) => `- ${r}`)] : [])].join('\n');
    },
  },
  add_frame: {
    description: 'Put a live screen on a board: the app at a route (list_screens), answered by a mock scenario (list_scenarios), or "live" for the real backend.',
    input: {
      board,
      route: str('The app\'s path, as list_screens gives it; ?open=<name> shows an overlay open'),
      name: str('Its label on the canvas'),
      scenario: str('A scenario\'s name; "default" if left out'),
      section: str('A section\'s id to place it in'),
      width: num('In CSS pixels; 1280 if left out'),
      height: num('In CSS pixels; 800 if left out'),
    },
    required: ['board', 'route'],
    run: async ({ board, route, name, scenario = 'default', section, width = 1280, height = 800 }, ctx) => {
      let made: Frame | undefined;
      await ctx.store.change(board, (b) => {
        const r = ops.addFrame(b, { route, name: name ?? route, scenario, width, height, ...spotIn(b, section, { width, height }) });
        made = r.frame;
        return grow(r.board, section, r.frame);
      });
      return `Added frame ${made!.id} at (${made!.x}, ${made!.y}).`;
    },
  },
  add_section: {
    description: 'A labelled section on a board, e.g. "Happy path" or "Edge cases". Given frames, it is drawn around them.',
    input: { board, name: str('Its label'), around: { type: 'array', items: { type: 'string' }, description: 'Ids of frames or notes to put it around' } },
    required: ['board', 'name'],
    run: async ({ board, name, around = [] }, ctx) => {
      let id = '';
      await ctx.store.change(board, (b) => {
        const box = ops.bounds((around as string[]).map((i) => find(b, i)).filter(isShape));
        const r = ops.addSection(b, box ? { name, x: box.x - 80, y: box.y - 120, width: box.width + 160, height: box.height + 200 } : { name });
        id = r.section.id;
        return r.board;
      });
      return `Added section ${id}.`;
    },
  },
  add_note: {
    description: 'A note on a board, beside a frame or anywhere.',
    input: { board, text: str('The note'), near: str('A frame\'s id to put it under') },
    required: ['board', 'text'],
    run: async ({ board, text, near }, ctx) => {
      let id = '';
      await ctx.store.change(board, (b) => {
        const by = near ? find(b, near) : undefined;
        const r = ops.addNote(b, isShape(by) ? { text, x: by.x, y: by.y + by.height + 40 } : { text });
        id = r.note.id;
        return r.board;
      });
      return `Added note ${id}.`;
    },
  },
  add_arrow: {
    description: 'An arrow from one frame (or note or section) to another, to show a flow, with an optional label (the action that leads there).',
    input: { board, from: str('Id it starts at'), to: str('Id it points to'), label: str('e.g. "Click Invite"') },
    required: ['board', 'from', 'to'],
    run: async ({ board, from, to, label }, ctx) => {
      let id: string | undefined;
      await ctx.store.change(board, (b) => {
        const r = ops.addArrow(b, from, to, label);
        id = r.arrow?.id;
        return r.board;
      });
      if (!id) throw new Error('both ends must be frames, notes or sections on the board, and not the same one');
      return `Added arrow ${id}.`;
    },
  },
  update_object: {
    description: 'Change a frame, section, note or arrow: its name, route, scenario, text, label, position or size.',
    input: {
      board, id: str('The object\'s id'),
      name: str('Name'), route: str('Route (frames)'), scenario: str('Scenario (frames)'), text: str('Text (notes)'), label: str('Label (arrows)'),
      x: num('x'), y: num('y'), width: num('Width'), height: num('Height'),
    },
    required: ['board', 'id'],
    run: async ({ board, id, ...patch }, ctx) => {
      await ctx.store.change(board, (b) => {
        if (!find(b, id)) throw new Error(`no object ${id} on ${b.name}`);
        return ops.update(b, id, patch);
      });
      return `Updated ${id}.`;
    },
  },
  remove_objects: {
    description: 'Remove frames, sections, notes, arrows or comments from a board.',
    input: { board, ids: { type: 'array', items: { type: 'string' } } },
    required: ['board', 'ids'],
    run: async ({ board, ids }, ctx) => {
      await ctx.store.change(board, (b) => ops.remove(b, ids));
      return `Removed ${ids.length}.`;
    },
  },
  tidy_board: {
    description: 'Lay a board\'s frames out from its arrows, as the Tidy up button does: the main path left to right in the first row, each branch below the frame it leaves, each section sized to hold its frames, and notes beside what they belong to. Call it after adding frames and arrows.',
    input: { board },
    required: ['board'],
    run: async ({ board }, ctx) => {
      await ctx.store.change(board, (b) => tidy(b));
      return `Tidied ${board}: ${ctx.url}#${board}`;
    },
  },
  delete_board: {
    description: 'Delete a board from the repo: its JSON file, and with folder: true the folder of its screens too (offered only when every screen of the board is in one folder no other board shows). Only when the person asks to delete that board: it is not a cleanup step. Call it first without confirm to see what it would delete and whether each is committed (a file never committed can\'t come back with git), tell the person, then call again with confirm set to the board\'s name. The home page can undo it until the canvas server stops.',
    input: {
      board,
      folder: { type: 'boolean', description: 'Also delete the folder of its screens; false if left out' },
      confirm: str('The board\'s name again, once the person agreed; leave out to see what it would delete'),
    },
    required: ['board'],
    run: async ({ board, folder = false, confirm }, ctx) => {
      if (!ctx.boardFiles || !ctx.deleteBoard) throw new Error('this canvas can\'t delete boards');
      const files = await ctx.boardFiles(board);
      const item = (f: { path: string, git: string }) => `${f.path} (${f.git === 'new' ? 'never committed' : f.git === 'changed' ? 'changed since the last commit' : 'committed'})`;
      const plan = [`- ${item(files.board)}`, folder && files.folder ? `- ${item(files.folder)}: ${files.folder.files} files` : '']
        .filter(Boolean).join('\n');
      if (folder && !files.folder) throw new Error(`${board}'s screens aren't in one folder of their own, so only the board can be deleted`);
      if (confirm !== board) {
        return `Deleting ${board} would delete:\n${plan}${!folder && files.folder ? `\nIts screens' folder (${files.folder.path}) stays; pass folder: true to delete it too.` : ''}\nAsk the person, then call again with confirm: "${board}".`;
      }
      await ctx.deleteBoard(board, folder);
      return `Deleted:\n${plan}`;
    },
  },
  list_templates: {
    description: 'Templates to start a prototype from: real screens of the app (in its app shell, with its navigation) and the parts only they use. Start a new prototype from the closest one with create_from_template rather than from a blank file.',
    input: {},
    run: async (_, ctx) => {
      const list = await templatesOf(ctx).list();
      return list.map((t) => `- \`${t.id}\`: ${t.name}${t.area ? ` (${t.area})` : ''}. ${t.description} Screens: ${t.screens.map((s) => s.id).join(', ')}.`).join('\n') || 'No templates.';
    },
  },
  get_template: {
    description: 'One template: its screens (file, route query, scenario, size), the arrows between them, and where its cover screen is served to look at.',
    input: { template: str('The template\'s id, as list_templates gives it') },
    required: ['template'],
    run: async ({ template }, ctx) => JSON.stringify(await templatesOf(ctx).read(template), null, 2),
  },
  create_from_template: {
    description: 'Start a prototype from a template: copies its folder (screens, their own parts, app-routes.ts) into src/prototypes/<project>/, where it is yours to change, and puts its screens on a board with its arrows, laid out. The board is made if there is none. Then change the copied files, not the template.',
    input: {
      template: str('The template\'s id'),
      project: str('The folder under src/prototypes for the copy: lowercase words and dashes, nested with slashes, e.g. datasets/bulk-edit'),
      board: str('The board for its screens: lowercase letters, digits and dashes; made if there is none'),
      name: str('The board\'s title, when it is made'),
      screens: { type: 'array', items: { type: 'string' }, description: 'Only these screens on the board, by id; all if left out' },
    },
    required: ['template', 'project', 'board'],
    run: async ({ template, project, board, name, screens }, ctx) => started(await templatesOf(ctx).start({ template, project, board, name, screens }), ctx),
  },
  add_template_screen: {
    description: 'Put one more screen of a template on a board, from the copy in a project (made if it is not there yet). For a screen a prototype started from a template left off its board.',
    input: {
      board,
      template: str('The template\'s id'),
      screen: str('The screen\'s id, as get_template gives it'),
      project: str('The project folder the template was copied to'),
    },
    required: ['board', 'template', 'screen', 'project'],
    run: async ({ board, template, screen, project }, ctx) => started(await templatesOf(ctx).start({ template, project, board, screens: [screen] }), ctx),
  },
  list_scenarios: {
    description: 'The mock scenarios a frame can use, each a state of the API (signed in with data, empty, failing, …). They are written in the app\'s code; add one there for a new state.',
    input: {},
    run: async (_, ctx) => {
      const all = await ctx.scenarios();
      return Object.entries(all).map(([n, s]) => `- \`${n}\`${s.label ? `: ${s.label}` : ''}${s.description ? ` (${s.description})` : ''}`).join('\n') || 'No scenarios.';
    },
  },
};

interface Message {
  jsonrpc: '2.0';
  id?: string | number;
  method: string;
  params?: Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any
}

/** Answer one JSON-RPC message; undefined for a notification, which has no answer. */
export async function handleMcp (msg: Message, ctx: McpContext): Promise<object | undefined> {
  if (msg.id === undefined) return undefined;
  const ok = (result: object) => ({ jsonrpc: '2.0', id: msg.id, result });
  const fail = (code: number, message: string) => ({ jsonrpc: '2.0', id: msg.id, error: { code, message } });
  switch (msg.method) {
    case 'initialize':
      return ok({
        protocolVersion: typeof msg.params?.protocolVersion === 'string' ? msg.params.protocolVersion : PROTOCOL_VERSION,
        capabilities: { tools: {} },
        serverInfo: { name: 'anfra-canvas', version: '0.0.0' },
        instructions: [ctx.instructions, GUIDE].filter(Boolean).join('\n\n'),
      });
    case 'ping':
      return ok({});
    case 'tools/list':
      return ok({
        tools: Object.entries(TOOLS).map(([name, t]) => ({
          name,
          description: t.description,
          inputSchema: { type: 'object', properties: t.input, ...(t.required ? { required: t.required } : {}) },
        })),
      });
    case 'tools/call': {
      const tool = TOOLS[msg.params?.name];
      if (!tool) return fail(-32602, `no tool ${String(msg.params?.name)}`);
      try {
        const text = await tool.run(msg.params?.arguments ?? {}, ctx);
        return ok({ content: [{ type: 'text', text }] });
      } catch (e) {
        return ok({ content: [{ type: 'text', text: e instanceof Error ? e.message : String(e) }], isError: true });
      }
    }
    default:
      return fail(-32601, `no method ${msg.method}`);
  }
}
