// The canvas's state: the open board and its history, what is selected, the view, and the frames'
// agents. Every change to the board goes through `commit` (one undo step, saved) or `preview` (a
// drag in progress: shown, not yet saved).
import { markRaw, nextTick, reactive, shallowRef, watch, type Component } from 'vue';
import {
  FRAME_SIZES, addComment, bounds, feedbackMarkdown, find, merge, newId, nextSpot, parseBoard, remove, shapes, tidy,
  type Anchor, type Board, type BoardObject, type Comment, type ElementRef, type Rect, type Scenarios, type TemplateSummary,
} from '../model/index.ts';
import type { CanvasHost, ElementInfo, FrameAgent, FrameEvent, FrameMode, LayerNode } from '../frame/protocol.ts';
import type { BoardDetails, BoardSummary } from '../vite/boards.ts';
import type { Changes, FileDiff } from '../vite/changes.ts';
import type { ComponentSpec, PaletteItem, Tokens } from '../vite/catalog.ts';
import type { Edit, SourceInfo, Where } from '../vite/source.ts';
import type { StartFrom } from '../vite/templates.ts';

export interface Settings {
  scenarios: Scenarios;
  prototypes: string[];
  catalog: ComponentSpec[];
  palette: PaletteItem[];
  tokens: Tokens;
  routes: string[];
  mcp: string;
  /** The name to add its MCP server by. */
  mcpName?: string;
  author: string;
  /** How a commit message from the canvas starts, for a repo that checks it (`feat(ui): `). */
  commitPrefix: string;
  /** The kit's icons, for props that take one: their names, and the prop that names one. */
  icons?: { prop: string, names: string[] };
  /** Where the prototypes are served (`/p`): the folder after it is a screen's project. */
  prototypeRoute?: string;
}

/** What the home page lists: every board, the recent ones, or a project's. */
export interface HomeView {
  kind: 'all' | 'recent' | 'project';
  id?: string;
}

/** The id of the frame that presents a prototype full screen: its app is used, never picked in. */
export const PRESENT_FRAME = 'present';

/** Presenting one frame's prototype full screen: the frame, and the scenario it is shown in. */
export interface Present {
  frame: string;
  scenario?: string;
}

/** One undo step: the board as it was, or a file the canvas edited, before and after. */
type Step = { kind: 'board', board: Board } | { kind: 'source', file: string, before: string, after: string };

/** Where a component being dragged would land. */
export interface Drop {
  frame: string;
  info: ElementInfo;
  where: Where;
}

export type Tool = 'select' | 'hand' | 'play' | 'comment' | 'frame' | 'section' | 'note' | 'arrow';

export const TOOLS: { tool: Tool, key: string, label: string }[] = [
  { tool: 'select', key: 'v', label: 'Select and edit' },
  { tool: 'hand', key: 'h', label: 'Pan' },
  { tool: 'play', key: 'p', label: 'Use the app' },
  { tool: 'comment', key: 'c', label: 'Comment' },
  { tool: 'frame', key: 'f', label: 'Frame' },
  { tool: 'section', key: 's', label: 'Section' },
  { tool: 'note', key: 'n', label: 'Note' },
  { tool: 'arrow', key: 'a', label: 'Arrow' },
];

const MODES: Record<Tool, FrameMode> = {
  select: 'inspect', comment: 'comment', play: 'interact',
  hand: 'idle', frame: 'idle', section: 'idle', note: 'idle', arrow: 'idle',
};

export interface Flow {
  path: string[];
  via: string[];
  /** The next arrow the pointer is on, shown running before it is taken. */
  hover: string;
}

export interface Picked {
  frame: string;
  info: ElementInfo;
}

export interface Composer {
  anchor: Anchor;
  /** Where to show it, in the viewport's pixels. */
  at: { x: number, y: number };
  body: string;
}

const API = '/__canvas/api';

async function api<T> (path: string, init?: RequestInit): Promise<{ status: number, data: T }> {
  const res = await fetch(`${API}${path}`, { ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } });
  const data = await res.json().catch(() => ({}));
  return { status: res.status, data };
}

export function createStore (settings: Settings, previews?: Component) {
  const state = reactive({
    settings,
    boards: [] as BoardSummary[],
    slug: '',
    error: '',
    tool: 'select' as Tool,
    /** Space held: pan whatever the tool. */
    panning: false,
    /** A drag is on: frames stop taking the pointer. */
    dragging: false,
    selection: [] as string[],
    picked: null as Picked | null,
    hover: null as Picked | null,
    composer: null as Composer | null,
    /** A palette item being placed: the next pick says where. */
    placing: '',
    /** Where a dragged palette item would land, shown on the frame. */
    drop: null as Drop | null,
    /** The open layers: frames and elements, by key. */
    layersOpen: {} as Record<string, boolean>,
    view: { x: 80, y: 80, zoom: 0.4 },
    /** Each frame's redraws, counted, so its outlines and pins are placed again: only its own. */
    layouts: {} as Record<string, number>,
    /** Bumped when a source file changes, so the inspector reads the picked element again. */
    sources: 0,
    routes: {} as Record<string, string>,
    attached: {} as Record<string, boolean>,
    /** The command palette, when open: everything, or the components to insert. */
    palette: '' as '' | 'all' | 'components',
    /** A frame or section whose name is being edited on the canvas. */
    renaming: '',
    /** Each frame's reloads, counted: a new count loads its screen again. */
    reloads: {} as Record<string, number>,
    /** The frames whose app is running: near the view, or not let go yet. */
    alive: {} as Record<string, boolean>,
    saving: 'saved' as 'saved' | 'saving' | 'error',
    toast: '',
    focus: '',
    viewport: { width: 0, height: 0 },
    history: { past: 0, future: 0 },
    /** Playing the board as a prototype: the frames walked, the arrows taken, the choice hovered. */
    flow: null as Flow | null,
    /** The tab beside the board. */
    panel: 'comments' as 'comments' | 'layers' | 'components' | 'changes',
    /** What changed in the app's files, from git: since the last commit, or on this branch. */
    changes: null as Changes | null,
    changesScope: 'head' as Changes['scope'],
    /** Outline the changed elements on the screens, while the Changes tab is open. */
    outlineChanges: true,
    /** A frame shown before its changes and now, split at `at` (0 to 1 of its width). */
    compare: null as { frame: string, at: number } | null,
    /** The home page, when no board is open: what it lists (all, recent, a project). */
    home: null as HomeView | null,
    /** Presenting a frame's prototype full screen, over the board. */
    present: null as Present | null,
  });

  // The board is kept as an immutable value: each change is a new board, so undo is a list.
  const board = shallowRef<Board | null>(null);
  let base: Board | null = null; // the last board the server confirmed
  const past: Step[] = [];
  const future: Step[] = [];
  let dragStart: Board | null = null;
  const agents = new Map<string, FrameAgent>();
  /** Each frame's redraws, counted: an edit waits for the screen to redraw before picking in it. */
  const redraws = new Map<string, number>();

  // --- Loading and saving -------------------------------------------------------------------------

  async function loadList () {
    const { data } = await api<BoardSummary[]>('/boards');
    state.boards = Array.isArray(data) ? data : [];
  }

  async function open (slug: string) {
    const { status, data } = await api<Board>(`/boards/${encodeURIComponent(slug)}`);
    if (status !== 200) {
      state.error = (data as unknown as { message?: string }).message ?? `Couldn't open ${slug}`;
      return;
    }
    state.error = '';
    state.slug = slug;
    base = parseBoard(data);
    board.value = base;
    past.length = 0;
    future.length = 0;
    synced();
    state.selection = [];
    state.picked = null;
    if (decodeURIComponent(location.hash.slice(1)).split('/')[0] !== slug) history.replaceState(null, '', `#${slug}`);
  }

  async function create (slug: string, name: string, details: BoardDetails = {}) {
    const { status, data } = await api<{ message?: string }>('/boards', { method: 'POST', body: JSON.stringify({ slug, name, ...details }) });
    if (status !== 201) return flash(data.message ?? 'Couldn\'t create the board');
    await loadList();
    location.hash = slug;
  }

  /** The templates a board can start from (the New board dialog's picker). */
  async function templates (): Promise<TemplateSummary[]> {
    const { status, data } = await api<TemplateSummary[]>('/templates');
    return status === 200 ? data : [];
  }

  /** Start a board from a template: its folder copied into the project, its screens on the board. */
  async function startFrom (req: StartFrom) {
    const { status, data } = await api<{ message?: string }>('/templates/start', { method: 'POST', body: JSON.stringify(req) });
    if (status !== 201) return flash(data.message ?? 'Couldn\'t start from the template');
    await loadList();
    location.hash = req.board;
  }

  /** Change a board's name or project, from its card on the home page. */
  async function updateDetails (slug: string, details: BoardDetails) {
    const { status, data } = await api<{ message?: string }>(`/boards/${encodeURIComponent(slug)}`, { method: 'PATCH', body: JSON.stringify(details) });
    if (status !== 200) return flash(data.message ?? 'Couldn\'t change the board');
    await loadList();
  }

  /** Keep a still of the board's first screen as its cover on the home page. */
  function keepCover () {
    const slug = state.slug;
    const first = board.value?.frames[0];
    const agent = first && agents.get(first.id);
    if (!slug || !agent) return;
    try {
      void fetch(`${API}/thumb?board=${encodeURIComponent(slug)}`, { method: 'PUT', body: agent.snapshot() });
    } catch { /* a screen that is loading */ }
  }

  let timer: ReturnType<typeof setTimeout> | undefined;
  let inflight = false;

  function scheduleSave () {
    state.saving = 'saving';
    clearTimeout(timer);
    timer = setTimeout(save, 400);
  }

  async function save () {
    if (inflight) return scheduleSave();
    const sent = board.value;
    if (!sent || !base || !state.slug) return;
    inflight = true;
    try {
      const { status, data } = await api<Board>(`/boards/${encodeURIComponent(state.slug)}`, {
        method: 'PUT',
        body: JSON.stringify({ board: sent, base: base.rev }),
      });
      if (status === 200) {
        base = parseBoard(data);
        if (board.value === sent) {
          board.value = base;
          state.saving = 'saved';
        } else {
          // Changed while saving: put what changed since onto the saved board, and save that.
          board.value = merge(sent, board.value!, base);
          scheduleSave();
        }
      } else if (status === 409) {
        // Someone (the agent) saved first: put my changes on top of theirs, and save that.
        const theirs = parseBoard(data);
        board.value = merge(base, board.value!, theirs);
        base = theirs;
        scheduleSave();
      } else {
        state.saving = 'error';
        flash((data as unknown as { message?: string }).message ?? 'Couldn\'t save the board');
      }
    } catch {
      state.saving = 'error';
    } finally {
      inflight = false;
      const next = waiting;
      waiting = undefined;
      if (next) received(next.slug, next.board);
    }
  }

  /** A board that came while saving, seen once the save is answered: it may be the save itself. */
  let waiting: { slug: string, board: Board } | undefined;

  /** A board the server says changed: an agent's change over MCP, or the file edited directly. */
  function received (slug: string, incoming: Board) {
    if (slug !== state.slug) return void loadList();
    if (inflight) {
      waiting = { slug, board: incoming };
      return;
    }
    if (!base || incoming.rev <= base.rev) return;
    const mine = board.value!;
    const clean = mine === base;
    board.value = clean ? incoming : merge(base, mine, incoming);
    base = incoming;
    if (!clean) scheduleSave();
    void loadList();
  }

  if (import.meta.hot) {
    import.meta.hot.on('anfra-canvas:board', ({ slug, board: b }: { slug: string, board: Board }) => received(slug, parseBoard(b)));
    import.meta.hot.on('anfra-canvas:source', () => state.sources++);
    import.meta.hot.on('anfra-canvas:files', () => readChangesSoon());
    import.meta.hot.on('anfra-canvas:changing', ({ file }: { file: string }) => void keepStills(file));
  }

  // --- Changes ------------------------------------------------------------------------------------

  async function loadChanges () {
    const { status, data } = await api<Changes>(`/changes?scope=${state.changesScope}`);
    if (status === 200) state.changes = data;
  }
  let changesTimer: ReturnType<typeof setTimeout> | undefined;
  const readChangesSoon = () => {
    clearTimeout(changesTimer);
    changesTimer = setTimeout(loadChanges, 300);
  };
  watch(() => state.changesScope, () => void loadChanges());
  // Read again each minute while the canvas is seen: a commit made in a terminal shows, and the
  // server knows the canvas is open (only then does a change wait for the stills of its screens).
  setInterval(() => {
    if (document.visibilityState === 'visible') void loadChanges();
  }, 60_000);
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') readChangesSoon();
  });

  const onBoard = (frame: string) => !!board.value?.frames.some((f) => f.id === frame);

  /** The frames of this board whose screens draw an element written in a file. */
  function framesShowing (file: string): string[] {
    return [...agents].filter(([id, agent]) => {
      try {
        return onBoard(id) && agent.shows(file);
      } catch {
        return false;
      }
    }).map(([id]) => id);
  }

  /**
   * A file is about to change for the first time since the last commit: keep a still of each screen
   * it may change (those that draw it, or every screen for a script or a style), to compare with
   * after. The server holds the change until the stills are taken.
   */
  async function keepStills (file: string) {
    const slug = state.slug;
    const ids = file.endsWith('.vue') ? framesShowing(file) : [...agents.keys()].filter(onBoard);
    const shots = ids.flatMap((id) => {
      try {
        return [[id, agents.get(id)!.snapshot()] as const];
      } catch {
        return []; // a screen that is loading
      }
    });
    void api('/ready', { method: 'POST', body: JSON.stringify({ file }) });
    await Promise.all(shots.map(([id, svg]) => fetch(`${API}/still?board=${encodeURIComponent(slug)}&frame=${id}`, { method: 'PUT', body: svg })));
    if (shots.length) readChangesSoon();
  }

  const hasStill = (frame: string) => !!state.changes?.stills[state.slug]?.includes(frame);
  const stillUrl = (frame: string) => `${API}/still?board=${encodeURIComponent(state.slug)}&frame=${frame}&at=${state.changes?.head ?? ''}`;

  /** Where the changed elements of a frame's screen are drawn now. */
  function changedRects (frame: string): Rect[] {
    const marks = state.changes?.marks;
    const agent = agents.get(frame);
    if (!marks || !agent) return [];
    try {
      return agent.rects(Object.values(marks).flat());
    } catch {
      return [];
    }
  }

  /** Commit the changed files; answers why it couldn't, or nothing when it did. */
  async function commitChanges (message: string, branch?: string): Promise<string | undefined> {
    const { status, data } = await api<{ sha: string, branch: string, message?: string }>('/commit', {
      method: 'POST',
      body: JSON.stringify({ message, ...(branch ? { branch } : {}) }),
    });
    if (status !== 200) return data.message ?? 'Couldn\'t commit';
    state.compare = null;
    flash(`Committed ${data.sha} on ${data.branch}`);
    await loadChanges();
    return undefined;
  }

  const readCommit = async (sha: string): Promise<FileDiff[]> => {
    const { status, data } = await api<FileDiff[]>(`/commits/${sha}`);
    return status === 200 && Array.isArray(data) ? data : [];
  };

  // --- Changing the board -------------------------------------------------------------------------

  const synced = () => {
    state.history.past = past.length;
    state.history.future = future.length;
  };

  function commit (next: Board) {
    const prev = dragStart ?? board.value;
    dragStart = null;
    if (!prev || next === prev) return;
    past.push({ kind: 'board', board: prev });
    if (past.length > 100) past.shift();
    future.length = 0;
    board.value = next;
    synced();
    scheduleSave();
  }

  function preview (next: Board) {
    dragStart ??= board.value;
    board.value = next;
  }

  async function undo () {
    const step = past.pop();
    if (!step || !board.value) return;
    if (step.kind === 'board') {
      future.push({ kind: 'board', board: board.value });
      board.value = step.board;
      scheduleSave();
    } else if (await restore(step.file, step.before, step.after)) {
      future.push(step);
    }
    synced();
  }

  async function redo () {
    const step = future.pop();
    if (!step || !board.value) return;
    if (step.kind === 'board') {
      past.push({ kind: 'board', board: board.value });
      board.value = step.board;
      scheduleSave();
    } else if (await restore(step.file, step.after, step.before)) {
      past.push(step);
    }
    synced();
  }

  /** A file the canvas edited, put back as it was; refused when it changed since. */
  async function restore (file: string, content: string, expect: string): Promise<boolean> {
    const { status, data } = await api<{ message?: string }>('/restore', { method: 'POST', body: JSON.stringify({ file, content, expect }) });
    if (status !== 200) flash(data.message ?? 'Couldn\'t undo it');
    state.picked = null;
    return status === 200;
  }

  function removeSelection () {
    if (!board.value || !state.selection.length) return;
    commit(remove(board.value, state.selection));
    state.selection = [];
    state.picked = null;
  }

  function comment (anchor: Anchor, body: string): Comment | undefined {
    if (!board.value || !body.trim()) return undefined;
    const r = addComment(board.value, { anchor, body: body.trim(), author: settings.author });
    commit(r.board);
    return r.comment;
  }

  // --- Frames -------------------------------------------------------------------------------------

  const redrawn = (frame: string) => {
    state.layouts[frame] = (state.layouts[frame] ?? 0) + 1;
  };

  const host: CanvasHost = {
    attach (frame, agent) {
      agents.set(frame, agent);
      state.attached[frame] = true;
      state.routes[frame] = agent.route();
      agent.setMode(modeFor(frame));
      redrawn(frame);
    },
    detach (frame, agent) {
      if (agents.get(frame) !== agent) return;
      agents.delete(frame);
      state.attached[frame] = false;
    },
    event: (frame, e) => onFrameEvent(frame, e),
  };
  window.__anfraCanvasHost = host;

  const modeFor = (frame: string): FrameMode => (frame === PRESENT_FRAME ? 'interact' : state.panning ? 'idle' : state.placing ? 'comment' : MODES[state.tool]);
  watch(() => state.tool, () => {
    state.placing = '';
  });
  const syncModes = () => {
    for (const [id, agent] of agents) agent.setMode(modeFor(id));
  };
  // Choosing another tool ends a flow being played.
  watch(() => state.tool, (tool) => {
    if (tool !== 'play' && state.flow) state.flow = null;
  });
  watch(() => [state.tool, state.panning, state.placing], () => {
    syncModes();
    state.hover = null;
  });

  /** Set by the viewport: turns a frame's event into the canvas's own handling. */
  const handlers = {
    wheel: (_frame: string, _e: Extract<FrameEvent, { type: 'wheel' }>) => {},
    key: (_e: Extract<FrameEvent, { type: 'key' }>) => {},
    pickedForComment: (_frame: string, _info: ElementInfo, _point: { x: number, y: number }) => {},
    /** A right click on an element of a frame, at a point of the frame's viewport. */
    menu: (_frame: string, _point: { x: number, y: number }) => {},
    /** Ask where a new screen goes, and which route, at a point of the board. */
    addScreen: (_at: { x: number, y: number }) => {},
    /** Open the command palette: everything, or the components to insert. */
    palette: (mode: 'all' | 'components') => {
      state.palette = mode;
    },
  };

  function onFrameEvent (frame: string, e: FrameEvent) {
    switch (e.type) {
      case 'hover':
        state.hover = e.info ? { frame, info: e.info } : state.hover?.frame === frame ? null : state.hover;
        break;
      case 'pick':
        if (state.placing) {
          const item = settings.palette.find((i) => i.id === state.placing);
          state.placing = '';
          if (item) void insert(frame, e.info, whereIn(e.info, e.point.y), item);
        } else if (state.tool === 'comment') {
          handlers.pickedForComment(frame, e.info, e.point);
        } else {
          state.picked = { frame, info: e.info };
          state.selection = [frame];
        }
        break;
      case 'activate':
        void editText(frame, e.info);
        break;
      case 'menu':
        state.picked = { frame, info: e.info };
        state.selection = [frame];
        handlers.menu(frame, e.point);
        break;
      case 'layout':
        redraws.set(frame, (redraws.get(frame) ?? 0) + 1);
        redrawn(frame);
        break;
      case 'route':
        state.routes[frame] = e.route;
        break;
      case 'wheel':
        handlers.wheel(frame, e);
        break;
      case 'key':
        handlers.key(e);
        break;
    }
  }

  const locate = (ref: ElementRef): ElementInfo | undefined => agents.get(ref.frame)?.locate(ref);
  const pickAt = (frame: string, x: number, y: number) => agents.get(frame)?.pickAt(x, y);

  /** The layer tree of a frame's screen, or nothing while it loads. */
  const tree = (frame: string): LayerNode[] => agents.get(frame)?.tree() ?? [];
  const snapshot = (frame: string): string | undefined => {
    try {
      return agents.get(frame)?.snapshot();
    } catch {
      return undefined;
    }
  };

  /** Pick an element of a frame, as clicking it on the screen does. */
  function pick (frame: string, ref: ElementRef) {
    const info = locate(ref);
    if (!info) return;
    state.picked = { frame, info };
    state.selection = [frame];
  }

  /** Pick the element around the picked one. */
  function pickParent () {
    const p = state.picked;
    const info = p && agents.get(p.frame)?.parent(p.info.ref);
    if (p && info) state.picked = { frame: p.frame, info };
  }

  /**
   * Pick the element written at a place once the frame shows it: after the edit redraws the screen,
   * and drawn as what was written there (`name`, a component or tag), not what stood there before.
   */
  function pickWhenShown (frame: string, source: string, name: string, since: number) {
    const until = Date.now() + 4000;
    const look = () => {
      const info = (redraws.get(frame) ?? 0) > since ? locate({ frame, tag: '', source }) : undefined;
      if (info && (!name || (info.ref.component ?? info.ref.tag) === name)) {
        state.picked = { frame, info };
        state.selection = [frame];
      } else if (Date.now() < until) {
        setTimeout(look, 120);
      }
    };
    setTimeout(look, 120);
  }

  /** Make an edit to the source of an element, as one undo step; answers whether it was written. */
  async function edit (frame: string, ref: ElementRef, change: Edit, done = 'Saved to the file'): Promise<boolean> {
    if (!ref.source) {
      flash('This element is drawn at run time: comment on it for Claude to change');
      return false;
    }
    const since = redraws.get(frame) ?? 0;
    const { status, data } = await api<{ message?: string, file: string, before: string, after: string, at?: string }>('/edit', {
      method: 'POST',
      body: JSON.stringify({ loc: ref.source, edit: change }),
    });
    if (status !== 200) {
      flash(data.message ?? 'Couldn\'t write it');
      return false;
    }
    past.push({ kind: 'source', file: data.file, before: data.before, after: data.after });
    if (past.length > 100) past.shift();
    future.length = 0;
    synced();
    if (change.kind === 'remove') state.picked = null;
    const name = change.kind === 'insert' ? /^\s*<([\w-]+)/.exec(change.markup)?.[1] ?? '' : ref.component ?? ref.tag;
    if (data.at) pickWhenShown(frame, data.at, name, since);
    redrawn(frame);
    if (done) flash(done);
    return true;
  }

  /** Before, after or inside an element, from where the pointer is on it (in the frame's viewport). */
  function whereIn (info: ElementInfo, y: number): Where {
    const t = (y - info.rect.y) / Math.max(1, info.rect.height);
    if (info.container && t > 0.25 && t < 0.75) return 'inside';
    return t < 0.5 ? 'before' : 'after';
  }

  async function insert (frame: string, target: ElementInfo, where: Where, item: PaletteItem) {
    await edit(frame, target.ref, { kind: 'insert', where, markup: item.markup, imports: item.imports }, `${item.name} added`);
  }

  /** Edit an element's text where it is drawn, and write it to its file; else leave it to the agent. */
  async function editText (frame: string, info: ElementInfo) {
    const agent = agents.get(frame);
    if (!agent) return;
    if (!info.textOnly || !info.ref.source) {
      flash('This element has more than plain text in it: comment on it for Claude to change');
      return;
    }
    const change = await agent.editText(info.ref);
    if (!change) return;
    if (await edit(frame, info.ref, { kind: 'text', ...change })) return;
    comment({ type: 'element', ...info.ref }, `Change the text "${change.from}" to "${change.to}"`);
    flash('Couldn\'t write it. Left a comment for Claude instead');
  }

  async function source (loc: string): Promise<SourceInfo | undefined> {
    const { status, data } = await api<SourceInfo>(`/source?loc=${encodeURIComponent(loc)}`);
    return status === 200 ? data : undefined;
  }

  // --- Small things -------------------------------------------------------------------------------

  let toastTimer: ReturnType<typeof setTimeout> | undefined;
  /** A comment on an element, written beside it. */
  function commentOn (frame: string, info: ElementInfo) {
    const f = board.value?.frames.find((x) => x.id === frame);
    if (!f) return;
    state.composer = { anchor: { type: 'element', ...info.ref }, at: toScreen({ x: f.x + info.rect.x + info.rect.width, y: f.y + info.rect.y }), body: '' };
  }

  /** Open the file an element is written in, at its line, in the editor the dev server knows. */
  const openInEditor = (source: string) => fetch(`/__open-in-editor?file=${encodeURIComponent(source)}`);

  const reload = (frame: string) => {
    state.reloads[frame] = (state.reloads[frame] ?? 0) + 1;
  };

  /** Place a palette item: the next element picked says where it goes. */
  async function place (id: string) {
    const item = settings.palette.find((i) => i.id === id);
    if (!item || state.placing === id) {
      state.placing = '';
      return;
    }
    if (state.tool !== 'select') {
      state.tool = 'select';
      await nextTick(); // changing the tool stops placing
    }
    state.placing = id;
    flash(`Click where the ${item.name} goes on a screen`);
  }

  /** A screen of a route, in the first free spot, shown. */
  function addFrame (route: string) {
    if (!board.value) return;
    const size = FRAME_SIZES[1];
    const name = route.split('/').filter(Boolean).pop() ?? route;
    const frame = { kind: 'frame' as const, id: newId('frame'), name, route, scenario: 'default', ...nextSpot(board.value), width: size.width, height: size.height };
    commit({ ...board.value, frames: [...board.value.frames, frame] });
    state.selection = [frame.id];
    zoomTo(frame, 1, true);
  }

  function flash (message: string) {
    state.toast = message;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (state.toast = ''), 4000);
  }

  async function copy (text: string, what = 'Copied') {
    try {
      await navigator.clipboard.writeText(text);
      flash(what);
    } catch {
      flash('The browser refused the clipboard');
    }
  }

  const copyFeedback = (comments?: Comment[]) => board.value && copy(feedbackMarkdown(board.value, comments), 'Feedback copied as Markdown');

  const selected = (): BoardObject[] => (board.value ? state.selection.map((id) => find(board.value!, id)).filter((o): o is BoardObject => !!o) : []);

  /** Screen point (in the viewport) of a world point. */
  const toScreen = (p: { x: number, y: number }) => ({ x: p.x * state.view.zoom + state.view.x, y: p.y * state.view.zoom + state.view.y });
  const toWorld = (p: { x: number, y: number }) => ({ x: (p.x - state.view.x) / state.view.zoom, y: (p.y - state.view.y) / state.view.zoom });

  /** Show a rectangle of the world, as large as fits. */
  // --- The camera --------------------------------------------------------------------------------

  let glide = 0;
  const still = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  const easeInOut = (t: number) => (t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2);

  /**
   * Fit a rectangle in the viewport. `smooth` glides there: the zoom changes geometrically and the
   * point in the middle moves straight, so the camera reads as one move, not a pan and a zoom.
   */
  /** Tidy up: lay the screens out from the flow, then show the whole board. */
  function tidyUp () {
    if (!board.value) return;
    const next = tidy(board.value);
    if (JSON.stringify(next) !== JSON.stringify(board.value)) commit(next);
    zoomTo(bounds(shapes(board.value)), 1, true);
  }

  function zoomTo (rect: Rect | undefined, max = 1, smooth = false, below = 0) {
    if (!rect || !state.viewport.width) return;
    const W = state.viewport.width;
    const H = state.viewport.height - below; // what lies under a bar at the bottom is not in view
    const pad = 64;
    const zoom = Math.max(0.05, Math.min(max, (W - pad * 2) / rect.width, (H - pad * 2) / rect.height));
    const to = { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
    cancelAnimationFrame(glide);
    const put = (c: { x: number, y: number }, z: number) => {
      state.view.zoom = z;
      state.view.x = W / 2 - c.x * z;
      state.view.y = H / 2 - c.y * z;
    };
    if (!smooth || still()) return put(to, zoom);
    const z0 = state.view.zoom;
    const from = { x: (W / 2 - state.view.x) / z0, y: (H / 2 - state.view.y) / z0 };
    const start = performance.now();
    const step = (now: number) => {
      const t = Math.min(1, (now - start) / 380);
      const e = easeInOut(t);
      put({ x: from.x + (to.x - from.x) * e, y: from.y + (to.y - from.y) * e }, z0 * (zoom / z0) ** e);
      if (t < 1) glide = requestAnimationFrame(step);
    };
    glide = requestAnimationFrame(step);
  }

  // --- Playing a flow ------------------------------------------------------------------------------

  /** A frame with its label, which the camera keeps in view. */
  const framed = (id: string): Rect | undefined => {
    const f = board.value?.frames.find((x) => x.id === id);
    return f && { x: f.x, y: f.y - 40 / state.view.zoom, width: f.width, height: f.height + 40 / state.view.zoom };
  };
  /** The flow bar's height, kept clear under the frame being played. */
  const BAR = 136;

  /** Where a flow starts when nothing is chosen: a frame arrows leave and none arrive at, top left first. */
  function startFrame (): string | undefined {
    const b = board.value;
    if (!b?.frames.length) return undefined;
    const chosen = state.selection.find((id) => b.frames.some((f) => f.id === id));
    if (chosen) return chosen;
    const into = new Set(b.arrows.map((a) => a.to));
    const out = new Set(b.arrows.map((a) => a.from));
    const order = [...b.frames].sort((p, q) => p.y - q.y || p.x - q.x);
    return (order.find((f) => out.has(f.id) && !into.has(f.id)) ?? order[0]).id;
  }

  function play (from = startFrame()) {
    if (!from) return;
    state.flow = { path: [from], via: [], hover: '' };
    state.selection = [];
    state.picked = null;
    state.composer = null;
    state.tool = 'play';
    zoomTo(framed(from), 1, true, BAR);
  }

  function playNext (arrowId: string) {
    const f = state.flow;
    const a = board.value?.arrows.find((x) => x.id === arrowId);
    if (!f || !a || !board.value?.frames.some((x) => x.id === a.to)) return;
    state.flow = { path: [...f.path, a.to], via: [...f.via, a.id], hover: '' };
    zoomTo(framed(a.to), 1, true, BAR);
  }

  function playBack () {
    const f = state.flow;
    if (!f || f.path.length < 2) return;
    state.flow = { path: f.path.slice(0, -1), via: f.via.slice(0, -1), hover: '' };
    zoomTo(framed(f.path[f.path.length - 2]), 1, true, BAR);
  }

  function stopPlay () {
    const at = state.flow?.path.at(-1);
    state.flow = null;
    state.tool = 'select';
    if (at) state.selection = [at];
  }

  return {
    state, board, handlers, previews: previews && markRaw(previews),
    loadList, open, create, templates, startFrom, updateDetails, keepCover, commit, preview, undo, redo, removeSelection, comment,
    loadChanges, framesShowing, hasStill, stillUrl, changedRects, commitChanges, readCommit,
    locate, pickAt, tree, snapshot, commentOn, openInEditor, reload, place, addFrame, pick, pickParent, edit, insert, whereIn, editText, source, flash, copy, copyFeedback, selected, toScreen, toWorld, zoomTo, tidyUp,
    play, playNext, playBack, stopPlay, startFrame,
    endDrag: () => {
      if (dragStart && board.value) commit(board.value);
      dragStart = null;
    },
  };
}

export type Store = ReturnType<typeof createStore>;
