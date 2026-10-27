// The canvas as a Vite plugin, for the app's dev server only. It
//   - tags every element the Vue compiler emits with where it is written,
//   - puts the frame agent at the top of the app's page (it does nothing outside a canvas frame),
//   - serves the canvas at /__canvas/, its boards and source edits at /__canvas/api/, and its MCP
//     tools at /__canvas/mcp.
// None of it is in a production build: every hook is for `vite serve`.
import { mkdirSync, watch } from 'node:fs';
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { searchForWorkspaceRoot, type Plugin, type ResolvedConfig, type ViteDevServer } from 'vite';
import { parseBoard, type Board } from '../model/board.ts';
import type { Scenarios } from '../model/scenario.ts';
import { BoardStore, Conflict, SLUG } from './boards.ts';
import { handleMcp, type McpContext } from './mcp.ts';
import { EditError, applyEdit, describeSource, parseLoc, sourceTransform, type Edit } from './source.ts';

export interface CanvasOptions {
  /** Where boards are kept, from the Vite root. */
  boards?: string;
  /** A module exporting `scenarios`, the frames' mock API answers, from the Vite root. */
  scenarios?: string;
  /** Where the app's API is; requests below it are answered by a frame's scenario. */
  apiBase?: string;
  /** Folders of components whose inner elements a pick skips, to the place the component is used. */
  libraryDirs?: string[];
  /** Folders whose `.vue` files the canvas may edit in place. */
  editable?: string[];
  /** Stylesheets for the canvas page itself (fonts, tokens), as the app imports them. */
  styles?: string[];
  /** Folders of prototypes, whose `.vue` files are offered as frames, and the route each is served at. */
  prototypes?: { dir: string, route: string };
  /** A folder of component folders (`ui/button`, …), offered in the canvas's component list. */
  components?: string;
  /** Routes offered when adding a frame. */
  routes?: string[];
  /** What an agent is told when it connects over MCP. */
  instructions?: string;
}

const HERE = dirname(fileURLToPath(import.meta.url));
const PACKAGE = resolve(HERE, '../..');
const FRAME_ID = '/@anfra-canvas/frame';
const APP_ID = '/@anfra-canvas/app';
const BASE = '/__canvas';

export function anfraCanvas (options: CanvasOptions = {}): Plugin {
  const opts = {
    boards: 'canvas/boards',
    apiBase: '/api',
    libraryDirs: ['src/components/ui/'],
    editable: ['src/'],
    styles: [],
    routes: [],
    instructions: 'Open feedback is in list_feedback. Change the code it points at, then resolve each comment.',
    ...options,
  };
  let config: ResolvedConfig;
  let server: ViteDevServer | undefined;

  const abs = (p: string) => resolve(config.root, p);
  const scenarioImport = () => (opts.scenarios ? `/${opts.scenarios.replace(/^\//, '')}` : undefined);

  async function scenarios (): Promise<Scenarios> {
    const path = scenarioImport();
    if (!path || !server) return {};
    const mod = await server.ssrLoadModule(path);
    return (mod.scenarios ?? mod.default ?? {}) as Scenarios;
  }

  return {
    name: 'anfra-canvas',
    apply: 'serve',
    enforce: 'pre',

    config: (user) => {
      const root = resolve(user.root ?? process.cwd());
      return {
        server: {
          // Setting the list replaces Vite's own, so the workspace stays in it beside this package.
          fs: { allow: [searchForWorkspaceRoot(root), PACKAGE] },
          // Board files change on every save. Vite's watcher would hand them to other plugins
          // (Tailwind reloads every page when a file it scans changes), so they are watched here.
          watch: { ignored: [`${resolve(root, opts.boards)}/**`] },
        },
        resolve: { dedupe: ['vue'] },
      };
    },

    configResolved (resolved) {
      config = resolved;
      // Add the source transform to the app's own Vue plugin, keeping its options.
      const vue = resolved.plugins.find((p) => p.name === 'vite:vue') as (Plugin & { api?: { options: any } }) | undefined; // eslint-disable-line @typescript-eslint/no-explicit-any
      if (!vue?.api) {
        resolved.logger.warn('[anfra canvas] @vitejs/plugin-vue not found: elements won\'t know where they are written');
        return;
      }
      const o = vue.api.options;
      const compilerOptions = o.template?.compilerOptions ?? {};
      vue.api.options = {
        ...o,
        template: {
          ...o.template,
          compilerOptions: { ...compilerOptions, nodeTransforms: [...(compilerOptions.nodeTransforms ?? []), sourceTransform(resolved.root)] },
        },
      };
    },

    resolveId (id) {
      if (id === FRAME_ID || id === APP_ID) return `\0${id}`;
      return undefined;
    },

    async load (id) {
      const s = scenarioImport();
      const scenarioCode = s ? `import * as scenarioModule from ${JSON.stringify(s)};\nconst scenarios = scenarioModule.scenarios ?? scenarioModule.default ?? {};` : 'const scenarios = {};';
      if (id === `\0${FRAME_ID}`) {
        return [
          `import { installFrame } from ${JSON.stringify(`/@fs${PACKAGE}/src/frame/index.ts`)};`,
          scenarioCode,
          `installFrame({ scenarios, apiBase: ${JSON.stringify(opts.apiBase)}, libraryDirs: ${JSON.stringify(opts.libraryDirs)} });`,
        ].join('\n');
      }
      if (id === `\0${APP_ID}`) {
        const settings = {
          prototypes: await prototypeRoutes(),
          components: await componentNames(),
          routes: opts.routes,
          mcp: `${BASE}/mcp`,
          author: await gitUser(config.root),
        };
        return [
          ...opts.styles.map((css) => `import ${JSON.stringify(css)};`),
          `import { mount } from ${JSON.stringify(`/@fs${PACKAGE}/src/app/main.ts`)};`,
          scenarioCode,
          `mount(document.getElementById('anfra-canvas'), { ...${JSON.stringify(settings)}, scenarios });`,
        ].join('\n');
      }
      return undefined;
    },

    transformIndexHtml: {
      order: 'pre',
      handler: (_html, ctx) =>
        ctx.path.startsWith(BASE) ? [] : [{ tag: 'script', attrs: { type: 'module', src: FRAME_ID }, injectTo: 'head-prepend' }],
    },

    configureServer (dev) {
      server = dev;
      const store = new BoardStore(abs(opts.boards), (slug, board) => broadcast(slug, board));
      const broadcast = (slug: string, board: Board) => dev.ws.send({ type: 'custom', event: 'anfra-canvas:board', data: { slug, board } });

      // A board file changed by someone else (an agent editing it directly, a git checkout).
      const external = async (name: string | null) => {
        if (!name?.endsWith('.json')) return;
        const slug = name.slice(0, -5);
        if (!SLUG.test(slug)) return;
        try {
          const text = await readFile(join(store.dir, name), 'utf8');
          if (store.written.get(slug) === text) return;
          store.written.set(slug, text);
          broadcast(slug, parseBoard(JSON.parse(text)));
        } catch { /* half-written, or not a board: the next change tells */ }
      };
      mkdirSync(store.dir, { recursive: true });
      const watcher = watch(store.dir, (_event, name) => void external(name));
      dev.httpServer?.once('close', () => watcher.close());

      const mcp: McpContext = {
        store,
        scenarios,
        instructions: opts.instructions,
        url: `${BASE}/`,
      };

      dev.middlewares.use(async (req, res, next) => {
        const url = new URL(req.url ?? '/', 'http://canvas');
        if (url.pathname !== BASE && !url.pathname.startsWith(`${BASE}/`)) return next();
        try {
          if (!sameOrigin(req)) return send(res, 403, { message: 'the canvas answers its own pages and local agents only' });
          const path = url.pathname.slice(BASE.length) || '/';
          if (path === '/' && req.method === 'GET') {
            if (req.url === BASE) return redirect(res, `${BASE}/`);
            const html = await dev.transformIndexHtml(req.url ?? `${BASE}/`, CANVAS_HTML, req.originalUrl);
            res.setHeader('Content-Type', 'text/html');
            return res.end(html);
          }
          if (path === '/mcp') return await serveMcp(req, res, mcp);
          if (path === '/api/boards') {
            if (req.method === 'GET') return send(res, 200, await store.list());
            if (req.method === 'POST') {
              const { slug, name } = await body(req);
              return send(res, 201, await store.create(String(slug), String(name || slug)));
            }
          }
          const boardPath = /^\/api\/boards\/([^/]+)$/.exec(path);
          if (boardPath) {
            const slug = decodeURIComponent(boardPath[1]);
            if (req.method === 'GET') return send(res, 200, await store.read(slug));
            if (req.method === 'PUT') {
              const { board, base } = await body(req);
              try {
                return send(res, 200, await store.save(slug, parseBoard(board), Number(base)));
              } catch (e) {
                if (e instanceof Conflict) return send(res, 409, e.current);
                throw e;
              }
            }
          }
          if (path === '/api/source' && req.method === 'GET') {
            const { loc, file } = editableLoc(url.searchParams.get('loc') ?? '');
            return send(res, 200, describeSource(await readFile(file, 'utf8'), loc));
          }
          if (path === '/api/edit' && req.method === 'POST') {
            const { loc: at, edit } = await body(req) as { loc: string, edit: Edit };
            const { loc, file } = editableLoc(at);
            await writeFile(file, applyEdit(await readFile(file, 'utf8'), loc, edit));
            return send(res, 200, { ok: true });
          }
          return send(res, 404, { message: `no ${req.method} ${path}` });
        } catch (e) {
          const status = e instanceof EditError ? 422 : (e as NodeJS.ErrnoException).code === 'ENOENT' ? 404 : 400;
          return send(res, status, { message: e instanceof Error ? e.message : String(e) });
        }
      });

      const printUrls = dev.printUrls.bind(dev);
      dev.printUrls = () => {
        printUrls();
        const local = dev.resolvedUrls?.local[0]?.replace(/\/$/, '');
        if (!local) return;
        config.logger.info(`  ➜  Canvas:  ${local}${BASE}/`);
        config.logger.info(`  ➜  MCP:     claude mcp add --transport http anfra-canvas ${local}${BASE}/mcp`);
      };
    },
  };

  function editableLoc (value: string) {
    const loc = parseLoc(value);
    if (!loc) throw new Error(`"${value}" is not a place in a file`);
    const file = abs(loc.file);
    const rel = relative(config.root, file);
    if (rel.startsWith('..') || !file.endsWith('.vue') || !opts.editable.some((d) => rel.startsWith(d))) {
      throw new EditError(`the canvas edits .vue files under ${opts.editable.join(', ')} only`);
    }
    return { loc, file };
  }

  async function prototypeRoutes (): Promise<string[]> {
    if (!opts.prototypes) return [];
    const { dir, route } = opts.prototypes;
    const files = await walk(abs(dir));
    return files
      .filter((f) => f.endsWith('.vue') && !f.split(sep).some((p) => p.startsWith('_')) && !/Host\.vue$/.test(f))
      .map((f) => `${route}/${relative(abs(dir), f).slice(0, -4).split(sep).join('/')}`);
  }

  async function componentNames (): Promise<string[]> {
    if (!opts.components) return [];
    try {
      return (await readdir(abs(opts.components), { withFileTypes: true })).filter((d) => d.isDirectory()).map((d) => d.name).sort();
    } catch {
      return [];
    }
  }
}

/** Who comments: the git user of the app's repo, as the person's commits name them. */
async function gitUser (cwd: string): Promise<string> {
  try {
    const { stdout } = await promisify(execFile)('git', ['config', 'user.name'], { cwd });
    return stdout.trim() || 'Designer';
  } catch {
    return 'Designer';
  }
}

async function walk (dir: string): Promise<string[]> {
  try {
    const entries = await readdir(dir, { withFileTypes: true });
    const nested = await Promise.all(entries.map((e) => (e.isDirectory() ? walk(join(dir, e.name)) : [join(dir, e.name)])));
    return nested.flat();
  } catch {
    return [];
  }
}

/**
 * A page of the dev server, or a local program (an agent's MCP client sends no Origin). Anything
 * else, a web page on another site above all, is refused: it must not read or edit the app.
 */
function sameOrigin (req: IncomingMessage): boolean {
  const origin = req.headers.origin;
  if (!origin) return true;
  try {
    return new URL(origin).host === req.headers.host;
  } catch {
    return false;
  }
}

async function serveMcp (req: IncomingMessage, res: ServerResponse, ctx: McpContext) {
  if (req.method !== 'POST') {
    res.setHeader('Allow', 'POST');
    return send(res, 405, { message: 'the canvas answers MCP requests by POST, one JSON response each' });
  }
  const msg = await body(req);
  const batch = Array.isArray(msg) ? msg : [msg];
  const answers = (await Promise.all(batch.map((m) => handleMcp(m, ctx)))).filter(Boolean);
  if (!answers.length) {
    res.statusCode = 202;
    return res.end();
  }
  return send(res, 200, Array.isArray(msg) ? answers : answers[0]);
}

async function body (req: IncomingMessage): Promise<any> { // eslint-disable-line @typescript-eslint/no-explicit-any
  const chunks: Buffer[] = [];
  for await (const c of req) chunks.push(c as Buffer);
  const text = Buffer.concat(chunks).toString('utf8');
  return text ? JSON.parse(text) : {};
}

function send (res: ServerResponse, status: number, value: unknown) {
  res.statusCode = status;
  res.setHeader('Content-Type', 'application/json');
  res.end(JSON.stringify(value));
}

function redirect (res: ServerResponse, to: string) {
  res.statusCode = 302;
  res.setHeader('Location', to);
  res.end();
}

const CANVAS_HTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Canvas</title>
  </head>
  <body>
    <div id="anfra-canvas"></div>
    <script type="module" src="${APP_ID}"></script>
  </body>
</html>
`;
