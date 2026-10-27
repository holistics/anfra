// The canvas's state: the open board and its history, what is selected, the view, and the frames'
// agents. Every change to the board goes through `commit` (one undo step, saved) or `preview` (a
// drag in progress: shown, not yet saved).
import { reactive, shallowRef, watch } from 'vue';
import {
  addComment, feedbackMarkdown, find, merge, parseBoard, remove,
  type Anchor, type Board, type BoardObject, type Comment, type ElementRef, type Rect, type Scenarios,
} from '../model/index.ts';
import type { CanvasHost, ElementInfo, FrameAgent, FrameEvent, FrameMode } from '../frame/protocol.ts';
import type { BoardSummary } from '../vite/boards.ts';

export interface Settings {
  scenarios: Scenarios;
  prototypes: string[];
  components: string[];
  routes: string[];
  mcp: string;
  author: string;
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

export function createStore (settings: Settings) {
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
    /** A component being placed: the next pick says where. */
    placing: '',
    view: { x: 80, y: 80, zoom: 0.4 },
    /** Bumped whenever a frame's screen changes, so outlines and pins are placed again. */
    layout: 0,
    routes: {} as Record<string, string>,
    attached: {} as Record<string, boolean>,
    saving: 'saved' as 'saved' | 'saving' | 'error',
    toast: '',
    focus: '',
    viewport: { width: 0, height: 0 },
    history: { past: 0, future: 0 },
  });

  // The board is kept as an immutable value: each change is a new board, so undo is a list.
  const board = shallowRef<Board | null>(null);
  let base: Board | null = null; // the last board the server confirmed
  const past: Board[] = [];
  const future: Board[] = [];
  let dragStart: Board | null = null;
  const agents = new Map<string, FrameAgent>();

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
    if (location.hash !== `#${slug}`) history.replaceState(null, '', `#${slug}`);
  }

  async function create (slug: string, name: string) {
    const { status, data } = await api<{ message?: string }>('/boards', { method: 'POST', body: JSON.stringify({ slug, name }) });
    if (status !== 201) return flash(data.message ?? 'Couldn\'t create the board');
    await loadList();
    await open(slug);
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
  }

  // --- Changing the board -------------------------------------------------------------------------

  const synced = () => {
    state.history.past = past.length;
    state.history.future = future.length;
  };

  function commit (next: Board) {
    const prev = dragStart ?? board.value;
    dragStart = null;
    if (!prev || next === prev) return;
    past.push(prev);
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

  function undo () {
    const prev = past.pop();
    if (!prev || !board.value) return;
    future.push(board.value);
    board.value = prev;
    synced();
    scheduleSave();
  }

  function redo () {
    const next = future.pop();
    if (!next || !board.value) return;
    past.push(board.value);
    board.value = next;
    synced();
    scheduleSave();
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

  const host: CanvasHost = {
    attach (frame, agent) {
      agents.set(frame, agent);
      state.attached[frame] = true;
      state.routes[frame] = agent.route();
      agent.setMode(modeFor(frame));
      state.layout++;
    },
    detach (frame, agent) {
      if (agents.get(frame) !== agent) return;
      agents.delete(frame);
      state.attached[frame] = false;
    },
    event: (frame, e) => onFrameEvent(frame, e),
  };
  window.__anfraCanvasHost = host;

  const modeFor = (_frame: string): FrameMode => (state.panning ? 'idle' : state.placing ? 'comment' : MODES[state.tool]);
  const syncModes = () => {
    for (const [id, agent] of agents) agent.setMode(modeFor(id));
  };
  watch(() => [state.tool, state.panning, state.placing], () => {
    syncModes();
    state.hover = null;
  });

  /** Set by the viewport: turns a frame's event into the canvas's own handling. */
  const handlers = {
    wheel: (_frame: string, _e: Extract<FrameEvent, { type: 'wheel' }>) => {},
    key: (_e: Extract<FrameEvent, { type: 'key' }>) => {},
    pickedForComment: (_frame: string, _info: ElementInfo, _point: { x: number, y: number }) => {},
  };

  function onFrameEvent (frame: string, e: FrameEvent) {
    switch (e.type) {
      case 'hover':
        state.hover = e.info ? { frame, info: e.info } : state.hover?.frame === frame ? null : state.hover;
        break;
      case 'pick':
        if (state.tool === 'comment') {
          handlers.pickedForComment(frame, e.info, e.point);
        } else {
          state.picked = { frame, info: e.info };
          state.selection = [frame];
        }
        break;
      case 'activate':
        void editText(frame, e.info);
        break;
      case 'layout':
        state.layout++;
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
    const { status, data } = await api<{ message?: string }>('/edit', {
      method: 'POST',
      body: JSON.stringify({ loc: info.ref.source, edit: { kind: 'text', ...change } }),
    });
    if (status === 200) return flash('Saved to the file');
    comment({ type: 'element', ...info.ref }, `Change the text "${change.from}" to "${change.to}"`);
    flash(`${data.message ?? 'Couldn\'t write it'}. Left a comment for Claude instead`);
  }

  async function editClasses (ref: ElementRef, classes: string) {
    if (!ref.source) return;
    const { status, data } = await api<{ message?: string }>('/edit', {
      method: 'POST',
      body: JSON.stringify({ loc: ref.source, edit: { kind: 'class', to: classes } }),
    });
    flash(status === 200 ? 'Saved to the file' : data.message ?? 'Couldn\'t write it');
  }

  async function source (loc: string) {
    const { status, data } = await api<{ tag: string, opening: string, classes?: string, text?: string }>(`/source?loc=${encodeURIComponent(loc)}`);
    return status === 200 ? data : undefined;
  }

  // --- Small things -------------------------------------------------------------------------------

  let toastTimer: ReturnType<typeof setTimeout> | undefined;
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
  function zoomTo (rect: Rect | undefined, max = 1) {
    if (!rect || !state.viewport.width) return;
    const pad = 80;
    const zoom = Math.min(max, (state.viewport.width - pad * 2) / rect.width, (state.viewport.height - pad * 2) / rect.height);
    state.view.zoom = Math.max(0.05, zoom);
    state.view.x = (state.viewport.width - rect.width * state.view.zoom) / 2 - rect.x * state.view.zoom;
    state.view.y = (state.viewport.height - rect.height * state.view.zoom) / 2 - rect.y * state.view.zoom;
  }

  return {
    state, board, handlers,
    loadList, open, create, commit, preview, undo, redo, removeSelection, comment,
    locate, pickAt, editText, editClasses, source, flash, copy, copyFeedback, selected, toScreen, toWorld, zoomTo,
    endDrag: () => {
      if (dragStart && board.value) commit(board.value);
      dragStart = null;
    },
  };
}

export type Store = ReturnType<typeof createStore>;
