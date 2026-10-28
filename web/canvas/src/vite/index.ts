// The canvas as a Vite plugin, for the app's dev server only. It
//   - tags every element the Vue compiler emits with where it is written,
//   - puts the frame agent at the top of the app's page (it does nothing outside a canvas frame),
//   - serves the canvas at /__canvas/, its boards and source edits at /__canvas/api/, and its MCP
//     tools at /__canvas/mcp.
// None of it is in a production build: every hook is for `vite serve`.
import { mkdirSync, watch, writeFileSync } from 'node:fs';
import { mkdir, readdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { searchForWorkspaceRoot, type Plugin, type ResolvedConfig, type ViteDevServer } from 'vite';
import { parseBoard, type Board } from '../model/board.ts';
import type { Scenarios } from '../model/scenario.ts';
import { BoardStore, Conflict, SLUG, withDetails, type BoardDetails } from './boards.ts';
import { handleMcp, type McpContext } from './mcp.ts';
import { kitCatalog, palette, readCatalog, readTokens, type ComponentSpec, type Kit, type KitIcons, type PaletteItem } from './catalog.ts';
import { EditError, describeSource, editSource, parseLoc, sourceTransform, type Edit } from './source.ts';
import { CommitError, Stills, commit, readChanges, readCommit } from './changes.ts';

export type { Kit, KitComponent, KitIcons, Placement, PropSpec } from './catalog.ts';

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
  /** Folders and files under those it must leave alone (the app's shell, a shared component library). */
  readonly?: string[];
  /** Stylesheets for the canvas page itself (fonts, tokens), as the app imports them. */
  styles?: string[];
  /**
   * The prototypes, offered as frames, and the route they are served under: the `.vue` files of a
   * folder (`dir`), each at its path below `route`, or, when the app's router finds its pages itself
   * (Nuxt), their routes from `list`.
   */
  prototypes?: { route: string, dir?: string, list?: () => string[] | Promise<string[]> };
  /** A folder of component folders (`ui/button`, …), offered in the canvas's component palette. */
  components?: string;
  /**
   * A module exporting `snippets`: the markup each component folder is placed with
   * (`{ card: '<Card>…</Card>' }`), or false to leave it out of the palette. The rest get `<Name>Name</Name>`.
   */
  snippets?: string;
  /**
   * A module exporting `kit`: the components of a design system that comes as a package, with the
   * props a designer sets and the markup each is placed with, and its icons (see `Kit`).
   */
  kit?: string;
  /**
   * A module exporting `people`: who makes prototypes, by id (`{ 'thong.do': { name, avatarUrl } }`),
   * to name a board's owners on the home page. Whoever runs the canvas is matched to one by the
   * local part of their git email, else by their git name.
   */
  people?: string;
  /** Routes offered when adding a frame. */
  routes?: string[];
  /** What an agent is told when it connects over MCP. */
  instructions?: string;
  /** How a commit message from the canvas starts, for repos that check it (`feat(ui): `). */
  commitPrefix?: string;
}

const HERE = dirname(fileURLToPath(import.meta.url));
const PACKAGE = resolve(HERE, '../..');
/** The frame agent, for an app whose page Vite doesn't serve (Nuxt): import it first thing on the client. */
export const FRAME_ID = '/@anfra-canvas/frame';
const APP_ID = '/@anfra-canvas/app';
const BASE = '/__canvas';
/** The palette's previews, written where Vite compiles them like the app's own files. */
const PREVIEWS = 'node_modules/.anfra-canvas/Previews.vue';
/** Stills of the frames from before the changes, to compare with now. */
const STILLS = 'node_modules/.anfra-canvas/stills';
/** Each board's cover for the home page. */
const THUMBS = 'node_modules/.anfra-canvas/thumbs';

export function anfraCanvas (options: CanvasOptions = {}): Plugin {
  const opts = {
    boards: 'canvas/boards',
    apiBase: '/api',
    libraryDirs: ['src/components/ui/'],
    editable: ['src/'],
    readonly: [] as string[],
    styles: [],
    routes: [],
    instructions: 'Open feedback is in list_feedback. Change the code it points at, then resolve each comment.',
    ...options,
  };
  let config: ResolvedConfig;
  let server: ViteDevServer | undefined;
  /** Hold a file's change until the canvas has stills of how it looked before (set by the server). */
  let hold: (file: string) => Promise<void> = async () => {};

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
        const kit = await loadKit();
        const fromKit = kitCatalog(kit);
        const catalog = [...await components(), ...fromKit.catalog];
        const items = palette(catalog, { ...fromKit.snippets, ...await snippets() });
        writePreviews(items, kit.icons);
        const settings = {
          prototypes: await prototypeRoutes(),
          catalog,
          palette: items,
          tokens: readTokens((await Promise.all(opts.styles.map((css) => cssText(abs(css.replace(/^\//, '')))))).join('\n')),
          routes: opts.routes,
          mcp: `${BASE}/mcp`,
          author: await gitUser(config.root),
          people: await people(),
          me: await whoAmI(),
          prototypeRoute: opts.prototypes?.route ?? '',
          commitPrefix: opts.commitPrefix ?? '',
          icons: kit.icons ? { prop: kit.icons.prop ?? 'name', names: kit.icons.names } : undefined,
        };
        return [
          ...opts.styles.map((css) => `import ${JSON.stringify(css)};`),
          `import { mount } from ${JSON.stringify(`/@fs${PACKAGE}/src/app/main.ts`)};`,
          `import Previews from ${JSON.stringify(`/@fs${abs(PREVIEWS)}`)};`,
          scenarioCode,
          `mount(document.getElementById('anfra-canvas'), { ...${JSON.stringify(settings)}, scenarios }, Previews);`,
        ].join('\n');
      }
      return undefined;
    },

    // The first change to an app file since the last commit waits for the canvas to keep a still of
    // each frame it may change, so the canvas can show the frame before and after.
    async handleHotUpdate ({ file }) {
      const rel = relative(config.root, file);
      if (/\.(vue|[cm]?[jt]sx?|s?css)$/.test(rel) && !rel.startsWith('..') && !rel.includes('node_modules')) await hold(rel);
    },

    transformIndexHtml: {
      order: 'pre',
      handler: (_html, ctx) =>
        ctx.path.startsWith(BASE) ? [] : [{ tag: 'script', attrs: { type: 'module', src: FRAME_ID }, injectTo: 'head-prepend' }],
    },

    configureServer (dev) {
      server = dev;
      /** When a canvas last asked for changes: only then does a change wait for its stills. */
      let canvasSeen = 0;
      /** The app's files changed since the last commit, as the canvas last read them. */
      let dirty = new Set<string>();
      /** Changes held until the canvas has its stills of the frames they are in. */
      const waiting = new Map<string, () => void>();
      hold = async (file) => {
        if (Date.now() - canvasSeen > 120_000 || dirty.has(file)) return;
        dirty.add(file);
        dev.ws.send({ type: 'custom', event: 'anfra-canvas:changing', data: { file } });
        await new Promise<void>((done) => {
          waiting.set(file, done);
          setTimeout(done, 600);
        });
        waiting.delete(file);
      };
      const store = new BoardStore(abs(opts.boards), (slug, board) => broadcast(slug, board));
      const stills = new Stills(abs(STILLS));
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

      // A component's file changed (an edit from the canvas, the agent, an editor): the inspector
      // reads the picked element's source again. Not on every redraw: a spinner redraws each frame.
      dev.watcher.on('change', (file) => {
        if (file.endsWith('.vue')) dev.ws.send({ type: 'custom', event: 'anfra-canvas:source', data: { file: relative(config.root, file) } });
      });
      // Any of the app's files changed, came or went: the Changes tab reads git again.
      for (const event of ['change', 'add', 'unlink'] as const) {
        dev.watcher.on(event, (file) => {
          const rel = relative(config.root, file);
          if (!rel.startsWith('..') && !rel.includes('node_modules')) dev.ws.send({ type: 'custom', event: 'anfra-canvas:files', data: { file: rel } });
        });
      }

      const mcp: McpContext = {
        store,
        scenarios,
        screens: async () => ({ prototypes: await prototypeRoutes(), routes: opts.routes }),
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
              const { slug, name, project, owners } = await body(req) as { slug: string, name?: string, project?: string, owners?: string[] };
              return send(res, 201, await store.create(String(slug), String(name || slug), { project, owners }));
            }
          }
          const boardPath = /^\/api\/boards\/([^/]+)$/.exec(path);
          if (boardPath) {
            const slug = decodeURIComponent(boardPath[1]);
            if (req.method === 'GET') return send(res, 200, await store.read(slug));
            // The board's details (name, project, owners), from its card on the home page.
            if (req.method === 'PATCH') {
              const details = await body(req) as BoardDetails;
              return send(res, 200, await store.change(slug, (b) => withDetails(b, details)));
            }
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
            const before = await readFile(file, 'utf8');
            const done = editSource(before, loc, edit);
            await writeFile(file, done.code);
            const where = done.at && `${done.at.file}:${done.at.line}:${done.at.column}`;
            return send(res, 200, { file: loc.file, before, after: done.code, ...(where ? { at: where } : {}) });
          }
          if (path === '/api/changes' && req.method === 'GET') {
            canvasSeen = Date.now();
            const changes = await readChanges(config.root, url.searchParams.get('scope') === 'branch' ? 'branch' : 'head');
            dirty = new Set(changes.pending);
            if (!changes.pending.length) await stills.clear(changes.head);
            changes.stills = await stills.list(changes.head);
            return send(res, 200, changes);
          }
          const commitPath = /^\/api\/commits\/([0-9a-f]+)$/.exec(path);
          if (commitPath && req.method === 'GET') return send(res, 200, await readCommit(config.root, commitPath[1]));
          if (path === '/api/commit' && req.method === 'POST') {
            try {
              const done = await commit(config.root, await body(req) as { message: string, branch?: string, files?: string[] });
              dirty = new Set();
              dev.ws.send({ type: 'custom', event: 'anfra-canvas:files', data: { file: '' } });
              return send(res, 200, done);
            } catch (e) {
              if (e instanceof CommitError) return send(res, 422, { message: e.message });
              throw e;
            }
          }
          // A board's cover for its card on the home page: a still of its first screen, kept
          // locally (not committed) each time the board is opened.
          if (path === '/api/thumb') {
            const board = url.searchParams.get('board') ?? '';
            if (!SLUG.test(board)) return send(res, 400, { message: 'Which board?' });
            const file = join(abs(THUMBS), `${board}.svg`);
            if (req.method === 'GET') {
              const svg = await readFile(file, 'utf8').catch(() => undefined);
              if (!svg) return send(res, 404, { message: 'No cover for this board yet' });
              res.setHeader('Content-Type', 'image/svg+xml');
              res.setHeader('Cache-Control', 'no-store');
              return res.end(svg);
            }
            if (req.method === 'PUT') {
              await mkdir(abs(THUMBS), { recursive: true });
              await writeFile(file, await text(req));
              return send(res, 200, { kept: true });
            }
          }
          // A frame's still from before the changes: kept once, read to compare.
          if (path === '/api/still') {
            const board = url.searchParams.get('board') ?? '';
            const frame = url.searchParams.get('frame') ?? '';
            if (!SLUG.test(board) || !/^[\w-]+$/.test(frame)) return send(res, 400, { message: 'Which board and frame?' });
            const head = await gitHead(config.root);
            if (req.method === 'GET') {
              const svg = await stills.read(head, board, frame).catch(() => undefined);
              if (!svg) return send(res, 404, { message: 'No still of this frame' });
              res.setHeader('Content-Type', 'image/svg+xml');
              res.setHeader('Cache-Control', 'no-store');
              return res.end(svg);
            }
            if (req.method === 'PUT') return send(res, 200, { kept: await stills.keep(head, board, frame, await text(req)) });
          }
          // The canvas took its stills of the frames a file is in, before the file's change shows.
          if (path === '/api/ready' && req.method === 'POST') {
            const { file } = await body(req) as { file: string };
            waiting.get(file)?.();
            return send(res, 200, { ok: true });
          }
          // Undo and redo of an edit: the file put back, if no one changed it since.
          if (path === '/api/restore' && req.method === 'POST') {
            const { file: rel, content, expect } = await body(req) as { file: string, content: string, expect: string };
            const { file } = editableLoc(`${rel}:1:1`);
            if (await readFile(file, 'utf8') !== expect) throw new EditError(`${rel} changed since: undo it in your editor`);
            await writeFile(file, content);
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
    if (opts.readonly.some((d) => rel === d || rel.startsWith(d))) throw new EditError(`${rel} is the app's own, not a prototype's: the canvas leaves it alone`);
    return { loc, file };
  }

  async function people (): Promise<Record<string, Person>> {
    if (!opts.people || !server) return {};
    try {
      const mod = await server.ssrLoadModule(`/${opts.people.replace(/^\//, '')}`);
      return (mod.people ?? mod.default ?? {}) as Record<string, Person>;
    } catch (e) {
      config.logger.warn(`[anfra canvas] couldn't read the people in ${opts.people}: ${e instanceof Error ? e.message : e}`);
      return {};
    }
  }

  /** Whoever runs the canvas: one of the people when their git email or name says so. */
  async function whoAmI (): Promise<string> {
    const [name, email] = await Promise.all([gitConfig(config.root, 'user.name'), gitConfig(config.root, 'user.email')]);
    const known = await people();
    const local = email.split('@')[0].toLowerCase();
    const id = Object.keys(known).find((k) => k.toLowerCase() === local)
      ?? Object.entries(known).find(([, p]) => p.name.toLowerCase() === name.toLowerCase())?.[0];
    return id ?? (name || local || 'Designer');
  }

  async function prototypeRoutes (): Promise<string[]> {
    if (!opts.prototypes) return [];
    const { dir, route, list } = opts.prototypes;
    if (list) return [...await list()].sort();
    if (!dir) return [];
    const files = await walk(abs(dir));
    return files
      .filter((f) => f.endsWith('.vue') && !f.split(sep).some((p) => p.startsWith('_')) && !/Host\.vue$/.test(f))
      .map((f) => `${route}/${relative(abs(dir), f).slice(0, -4).split(sep).join('/')}`);
  }

  async function components (): Promise<ComponentSpec[]> {
    if (!opts.components) return [];
    return readCatalog(abs(opts.components), importPath(abs(opts.components)));
  }

  async function snippets (): Promise<Record<string, string | false>> {
    if (!opts.snippets || !server) return {};
    try {
      const mod = await server.ssrLoadModule(`/${opts.snippets.replace(/^\//, '')}`);
      return (mod.snippets ?? mod.default ?? {}) as Record<string, string | false>;
    } catch (e) {
      config.logger.warn(`[anfra canvas] couldn't load ${opts.snippets}: ${(e as Error).message}`);
      return {};
    }
  }

  async function loadKit (): Promise<Kit> {
    if (!opts.kit || !server) return {};
    try {
      const mod = await server.ssrLoadModule(`/${opts.kit.replace(/^\//, '')}`);
      return (mod.kit ?? mod.default ?? {}) as Kit;
    } catch (e) {
      config.logger.warn(`[anfra canvas] couldn't load ${opts.kit}: ${(e as Error).message}`);
      return {};
    }
  }

  /** How the app imports a folder: through its alias (`@/components/ui`), else from the root. */
  function importPath (dir: string): string {
    for (const a of config.resolve.alias) {
      if (typeof a.find !== 'string' || typeof a.replacement !== 'string') continue;
      const to = a.replacement.replace(/\/$/, '');
      if (dir === to || dir.startsWith(`${to}/`)) return `${a.find}${dir.slice(to.length)}`;
    }
    return `/${relative(config.root, dir).split(sep).join('/')}`;
  }

  /**
   * One component showing any palette item, by its id: the palette renders the app's own components.
   * `icon:<name>` shows that icon, drawn by the kit's icon component, for the icon picker.
   */
  function writePreviews (items: PaletteItem[], icons?: KitIcons) {
    const imports = new Map(items.flatMap((i) => i.imports.map((x) => [x.name, x.from] as const)));
    const branches = items.map((i) => [`id === ${JSON.stringify(i.id).replace(/"/g, '\'')}`, i.markup]);
    if (icons) {
      imports.set(icons.component, icons.from);
      branches.unshift(['id.startsWith(\'icon:\')', `<${icons.component} :${icons.prop ?? 'name'}="id.slice(5)" />`]);
    }
    const code = [
      '<script setup lang="ts">',
      ...[...imports].map(([name, from]) => `import { ${name} } from ${JSON.stringify(from)};`),
      'defineProps<{ id: string }>();',
      '</script>',
      '',
      '<template>',
      ...branches.map(([test, markup], n) => `  <template ${n ? 'v-else-if' : 'v-if'}="${test}">${markup}</template>`),
      '</template>',
      '',
    ].join('\n');
    mkdirSync(dirname(abs(PREVIEWS)), { recursive: true });
    writeFileSync(abs(PREVIEWS), code);
  }
}

/** A stylesheet's text, with the local files it `@import`s (`./tokens.css`) in it, where tokens may be. */
async function cssText (file: string, seen = new Set<string>()): Promise<string> {
  if (seen.has(file)) return '';
  seen.add(file);
  const text = await readFile(file, 'utf8').catch(() => '');
  const local = [...text.matchAll(/@import\s+(?:url\()?['"](\.{1,2}\/[^'"]+\.css)['"]/g)].map((m) => resolve(dirname(file), m[1]));
  return [text, ...await Promise.all(local.map((f) => cssText(f, seen)))].join('\n');
}

/** Who comments: the git user of the app's repo, as the person's commits name them. */
/** One of the people who make prototypes. */
export interface Person {
  name: string;
  avatarUrl?: string;
}

async function gitConfig (cwd: string, key: string): Promise<string> {
  try {
    const { stdout } = await promisify(execFile)('git', ['config', key], { cwd });
    return stdout.trim();
  } catch {
    return '';
  }
}

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

async function text (req: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const c of req) chunks.push(c as Buffer);
  return Buffer.concat(chunks).toString('utf8');
}

async function body (req: IncomingMessage): Promise<any> { // eslint-disable-line @typescript-eslint/no-explicit-any
  const raw = await text(req);
  return raw ? JSON.parse(raw) : {};
}

async function gitHead (cwd: string): Promise<string> {
  try {
    const { stdout } = await promisify(execFile)('git', ['rev-parse', 'HEAD'], { cwd });
    return stdout.trim() || 'none';
  } catch {
    return 'none';
  }
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
