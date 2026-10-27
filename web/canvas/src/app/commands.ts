// Everything a person can do on the canvas, as commands. The command palette (⌘K) lists them all,
// with the screens, boards and components to go to or insert; the right-click menu lists the ones
// for what was clicked. A command does what its button or key does, so the three never disagree.
import {
  addNote, addSection, arrange, bounds, duplicate, find, isShape, nextSpot, remove, shapes,
  type Arrow, type Point, type Shape,
} from '../model/index.ts';
import type { PaletteItem } from '../vite/catalog.ts';
import type { IconName } from './ui/Icon.vue';
import { TOOLS, type Store } from './store.ts';

export interface Command {
  id: string;
  label: string;
  icon?: IconName;
  /** Its shortcut, as the canvas shows it. */
  keys?: string;
  /** Words it is found by in the palette, besides its label. */
  also?: string;
  danger?: boolean;
  run: () => unknown;
}

export interface CommandGroup {
  name: string;
  items: Command[];
}

const TOOL_ICONS: Record<string, IconName> = {
  select: 'select', hand: 'hand', play: 'play', comment: 'comment', frame: 'frame', section: 'section', note: 'note', arrow: 'arrow',
};

export function zoomBy (store: Store, factor: number) {
  const { state } = store;
  const mid = { x: state.viewport.width / 2, y: state.viewport.height / 2 };
  const at = store.toWorld(mid);
  state.view.zoom = Math.min(4, Math.max(0.05, state.view.zoom * factor));
  state.view.x = mid.x - at.x * state.view.zoom;
  state.view.y = mid.y - at.y * state.view.zoom;
}

/** The selected frames, sections and notes, wrapped in a new section. */
export function wrapSelection (store: Store) {
  const { state, board } = store;
  if (!board.value) return;
  const list = state.selection.map((id) => find(board.value!, id)).filter(isShape);
  const box = bounds(list);
  if (!box) return;
  const r = addSection(board.value, { name: 'Section', x: box.x - 80, y: box.y - 120, width: box.width + 160, height: box.height + 200 });
  store.commit({ ...r.board, sections: [r.section, ...r.board.sections.filter((s) => s.id !== r.section.id)] });
  state.selection = [r.section.id];
}

export function duplicateSelection (store: Store) {
  const { state, board } = store;
  if (!board.value || !state.selection.length) return;
  const r = duplicate(board.value, state.selection);
  store.commit(r.board);
  state.selection = r.ids;
}

/** The picked element: what a designer does to a layer. */
export function elementCommands (store: Store): Command[] {
  const { state } = store;
  const p = state.picked;
  if (!p) return [];
  const name = p.info.ref.component ?? p.info.ref.tag;
  const source = p.info.ref.source;
  const edit = (kind: 'duplicate' | 'remove', done: string) => () => store.edit(p.frame, p.info.ref, { kind }, done);
  const move = (by: -1 | 1) => () => store.edit(p.frame, p.info.ref, { kind: 'move', by }, '');
  return [
    { id: 'element.parent', label: 'Select parent', icon: 'parent', keys: '⇧↵', run: () => store.pickParent() },
    ...(p.info.textOnly ? [{ id: 'element.text', label: 'Edit text', icon: 'text' as const, keys: 'Double-click', run: () => store.editText(p.frame, p.info) }] : []),
    { id: 'element.comment', label: 'Comment for Claude', icon: 'comment', run: () => store.commentOn(p.frame, p.info) },
    { id: 'element.insert', label: 'Insert a component', icon: 'component', also: 'add', run: () => store.handlers.palette('components') },
    ...(source
      ? [
          { id: 'element.duplicate', label: `Duplicate ${name}`, icon: 'duplicate' as const, keys: '⌘D', run: edit('duplicate', 'Duplicated') },
          { id: 'element.up', label: 'Move up', icon: 'arrowUp' as const, keys: '⌥↑', also: 'earlier before', run: move(-1) },
          { id: 'element.down', label: 'Move down', icon: 'arrowDown' as const, keys: '⌥↓', also: 'later after', run: move(1) },
          { id: 'element.open', label: 'Open in editor', icon: 'code' as const, also: 'source file vs code', run: () => store.openInEditor(source) },
          { id: 'element.copy', label: 'Copy where it is written', icon: 'copy' as const, also: 'source file path line', run: () => store.copy(source, 'Copied where it is written') },
          { id: 'element.remove', label: `Delete ${name}`, icon: 'trash' as const, keys: '⌫', danger: true, run: edit('remove', `${name} removed`) },
        ]
      : []),
  ];
}

/** The selected frames, sections, notes and arrows. */
export function selectionCommands (store: Store): Command[] {
  const { state, board } = store;
  const b = board.value;
  if (!b || !state.selection.length) return [];
  const list = state.selection.map((id) => find(b, id)).filter((o) => o !== undefined);
  const shaped = list.filter(isShape) as Shape[];
  const one = list.length === 1 ? list[0] : undefined;
  const out: Command[] = [];
  if (one?.kind === 'frame') {
    out.push(
      { id: 'frame.play', label: 'Play from here', icon: 'play', run: () => store.play(one.id) },
      { id: 'frame.reload', label: 'Reload', icon: 'reload', run: () => store.reload(one.id) },
      { id: 'frame.rename', label: 'Rename', icon: 'text', keys: 'Double-click', run: () => (state.renaming = one.id) },
    );
  }
  if (one?.kind === 'section') out.push({ id: 'section.rename', label: 'Rename', icon: 'text', keys: 'Double-click', run: () => (state.renaming = one.id) });
  if (one?.kind === 'arrow') {
    const a = one as Arrow;
    out.push({ id: 'arrow.reverse', label: 'Reverse', icon: 'reload', run: () => store.commit({ ...b, arrows: b.arrows.map((x) => (x.id === a.id ? { ...x, from: x.to, to: x.from } : x)) }) });
  }
  if (shaped.length) {
    out.push({ id: 'selection.zoom', label: 'Zoom to selection', icon: 'fit', keys: '⇧2', run: () => store.zoomTo(bounds(shaped), 1, true) });
  }
  if (shaped.length > 1) {
    out.push(
      { id: 'selection.left', label: 'Align left edges', icon: 'alignLeft', run: () => store.commit(arrange(b, state.selection, 'left')) },
      { id: 'selection.top', label: 'Align top edges', icon: 'alignTop', run: () => store.commit(arrange(b, state.selection, 'top')) },
      { id: 'selection.row', label: 'Lay out in a row', icon: 'row', run: () => store.commit(arrange(b, state.selection, 'row')) },
      { id: 'selection.column', label: 'Lay out in a column', icon: 'column', run: () => store.commit(arrange(b, state.selection, 'column')) },
    );
  }
  if (shaped.some((s) => s.kind !== 'section')) out.push({ id: 'selection.wrap', label: 'Put in a section', icon: 'section', also: 'group', run: () => wrapSelection(store) });
  if (shaped.length) out.push({ id: 'selection.duplicate', label: 'Duplicate', icon: 'duplicate', keys: '⌘D', run: () => duplicateSelection(store) });
  out.push({ id: 'selection.remove', label: 'Delete', icon: 'trash', keys: '⌫', danger: true, run: () => store.removeSelection() });
  return out;
}

/** Adding to the board, at a point of it or in the first free spot. */
export function addCommands (store: Store, at?: Point): Command[] {
  const { state, board } = store;
  const spot = () => at ?? (board.value ? nextSpot(board.value) : { x: 0, y: 0 });
  return [
    { id: 'add.frame', label: 'Add a screen', icon: 'frame', keys: 'F', also: 'frame route', run: () => store.handlers.addScreen(spot()) },
    {
      id: 'add.section', label: 'Add a section', icon: 'section', keys: 'S',
      run: () => {
        if (!board.value) return;
        const r = addSection(board.value, { ...spot(), width: 1600, height: 1000 });
        store.commit(r.board);
        state.selection = [r.section.id];
        state.renaming = r.section.id;
      },
    },
    {
      id: 'add.note', label: 'Add a note', icon: 'note', keys: 'N',
      run: () => {
        if (!board.value) return;
        const r = addNote(board.value, spot());
        store.commit(r.board);
        state.selection = [r.note.id];
        state.focus = r.note.id;
      },
    },
  ];
}

/** The whole board and the view of it. */
export function boardCommands (store: Store): Command[] {
  const { state, board } = store;
  const all = () => (board.value ? bounds(shapes(board.value)) : undefined);
  return [
    { id: 'board.tidy', label: 'Tidy up', icon: 'tidy', keys: '⌃⌥T', also: 'auto layout arrange flow', run: () => store.tidyUp() },
    { id: 'board.play', label: 'Play the flow', icon: 'play', also: 'prototype', run: () => store.play() },
    { id: 'board.fit', label: 'Fit everything', icon: 'fit', keys: '⇧1', also: 'zoom', run: () => store.zoomTo(all(), 1, true) },
    { id: 'board.all', label: 'Select all', icon: 'select', keys: '⌘A', run: () => (state.selection = board.value ? shapes(board.value).map((s) => s.id) : []) },
    { id: 'board.feedback', label: 'Copy feedback for Claude', icon: 'copy', also: 'comments markdown', run: () => store.copyFeedback() },
  ];
}

export function viewCommands (store: Store): Command[] {
  const { state } = store;
  return [
    { id: 'view.in', label: 'Zoom in', icon: 'plus', keys: '⌘+', run: () => zoomBy(store, 1.25) },
    { id: 'view.out', label: 'Zoom out', icon: 'minus', keys: '⌘−', run: () => zoomBy(store, 1 / 1.25) },
    { id: 'view.100', label: 'Zoom to 100%', icon: 'fit', also: 'actual size', run: () => zoomBy(store, 1 / state.view.zoom) },
    { id: 'history.undo', label: 'Undo', icon: 'undo', keys: '⌘Z', run: () => store.undo() },
    { id: 'history.redo', label: 'Redo', icon: 'redo', keys: '⇧⌘Z', run: () => store.redo() },
    ...TOOLS.map((t) => ({ id: `tool.${t.tool}`, label: `Tool: ${t.label}`, icon: TOOL_ICONS[t.tool], keys: t.key.toUpperCase(), run: () => (state.tool = t.tool) })),
  ];
}

/** Where the palette can take you: this board's screens, the other boards. */
export function goCommands (store: Store): CommandGroup[] {
  const { state, board } = store;
  const screens = (board.value?.frames ?? []).map((f): Command => ({
    id: `go.${f.id}`, label: f.name, icon: 'frame', also: f.route,
    run: () => {
      state.selection = [f.id];
      store.zoomTo(f, 1, true);
    },
  }));
  const boards = state.boards.filter((b) => b.slug !== state.slug).map((b): Command => ({
    id: `board.${b.slug}`, label: b.name, icon: 'layers', also: b.slug, run: () => (location.hash = b.slug),
  }));
  return [{ name: 'Screens', items: screens }, { name: 'Boards', items: boards }];
}

/** The app's components: inserted next to the picked element, or placed with the next click. */
export function componentCommands (store: Store): Command[] {
  const { state } = store;
  const p = state.picked;
  const into = (item: PaletteItem) => () => {
    if (p) return store.insert(p.frame, p.info, p.info.container ? 'inside' : 'after', item);
    store.place(item.id);
  };
  return state.settings.palette.map((item) => ({
    id: `insert.${item.id}`, label: item.name, icon: 'component' as const, also: 'insert component add', run: into(item),
  }));
}

/** Screens to add from the app's routes and prototypes. */
export function routeCommands (store: Store): Command[] {
  const { state } = store;
  return [...state.settings.prototypes, ...state.settings.routes].map((route) => ({
    id: `route.${route}`, label: route, icon: 'plus' as const, also: 'add screen frame route', run: () => store.addFrame(route),
  }));
}

/** What a right click offers: for the element, the selection, or the empty canvas under it. */
export function menuFor (store: Store, target: 'element' | 'selection' | 'canvas', at?: Point): CommandGroup[] {
  if (target === 'element') return [{ name: '', items: elementCommands(store) }];
  if (target === 'selection') return [{ name: '', items: selectionCommands(store) }];
  return [
    { name: '', items: addCommands(store, at) },
    { name: '', items: boardCommands(store) },
  ];
}

/** A command's score for a query: higher is a better match, 0 is none. */
export function score (c: Command, query: string): number {
  const q = query.trim().toLowerCase();
  if (!q) return 1;
  const label = c.label.toLowerCase();
  if (label.startsWith(q)) return 4;
  if (label.split(/[\s/:]+/).some((w) => w.startsWith(q))) return 3;
  if (label.includes(q)) return 2;
  const words = `${label} ${c.also ?? ''}`.toLowerCase();
  return q.split(/\s+/).every((w) => words.includes(w)) ? 1 : 0;
}
