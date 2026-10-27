// Changes to a board. Each returns a new board and leaves the one it was given as it was, so the
// canvas keeps earlier boards for undo, and the server and the canvas can compare two of them.
import {
  COLLECTIONS, collectionOf,
  type Arrow, type Board, type BoardObject, type Comment, type Frame, type Note, type Point, type Rect,
  type Section, type Shape,
} from './board.ts';

let counter = 0;

/** An id unique on this board and short enough to read in a file: `fr_lx2k9a3`. */
export function newId (kind: BoardObject['kind']): string {
  counter = (counter + 1) % 1296;
  return `${kind.slice(0, 2)}_${Date.now().toString(36)}${counter.toString(36).padStart(2, '0')}`;
}

export function find (board: Board, id: string): BoardObject | undefined {
  for (const key of COLLECTIONS) {
    const found = (board[key] as BoardObject[]).find((o) => o.id === id);
    if (found) return found;
  }
  return undefined;
}

export const shapes = (board: Board): Shape[] => [...board.sections, ...board.frames, ...board.notes];

export const isShape = (o: BoardObject | undefined): o is Shape =>
  o?.kind === 'frame' || o?.kind === 'section' || o?.kind === 'note';

/** Add objects, or replace those with the same id. */
export function put (board: Board, ...objects: BoardObject[]): Board {
  const next = { ...board };
  for (const o of objects) {
    const key = collectionOf(o.kind);
    const list = next[key] as BoardObject[];
    const at = list.findIndex((x) => x.id === o.id);
    (next[key] as BoardObject[]) = at < 0 ? [...list, o] : list.map((x, i) => (i === at ? o : x));
  }
  return next;
}

/** Change one object's fields. An id the board doesn't have changes nothing. */
export function update<T extends BoardObject> (board: Board, id: string, patch: Partial<Omit<T, 'id' | 'kind'>>): Board {
  const o = find(board, id);
  return o ? put(board, { ...o, ...patch } as BoardObject) : board;
}

/**
 * Remove objects, with what can't stand without them: the arrows to or from them, the comments on
 * their elements, and the replies stay with their comment.
 */
export function remove (board: Board, ids: Iterable<string>): Board {
  const gone = new Set(ids);
  return {
    ...board,
    frames: board.frames.filter((o) => !gone.has(o.id)),
    sections: board.sections.filter((o) => !gone.has(o.id)),
    notes: board.notes.filter((o) => !gone.has(o.id)),
    arrows: board.arrows.filter((o) => !gone.has(o.id) && !gone.has(o.from) && !gone.has(o.to)),
    comments: board.comments.filter((o) => !gone.has(o.id) && !(o.anchor.type === 'element' && gone.has(o.anchor.frame))),
  };
}

const inside = (inner: Rect, outer: Rect): boolean =>
  inner.x >= outer.x && inner.y >= outer.y
  && inner.x + inner.width <= outer.x + outer.width && inner.y + inner.height <= outer.y + outer.height;

/** The frames and notes lying wholly inside a section. */
export const contents = (board: Board, section: Section): Shape[] =>
  [...board.frames, ...board.notes].filter((s) => inside(s, section));

/**
 * Move shapes by an offset. A section takes what lies inside it, and a comment pinned to the
 * canvas inside a moved section moves with it.
 */
export function move (board: Board, ids: Iterable<string>, d: Point): Board {
  const moving = new Set(ids);
  for (const s of board.sections) if (moving.has(s.id)) for (const c of contents(board, s)) moving.add(c.id);
  const by = <T extends Shape>(s: T): T => (moving.has(s.id) ? { ...s, x: s.x + d.x, y: s.y + d.y } : s);
  const areas = board.sections.filter((s) => moving.has(s.id));
  return {
    ...board,
    frames: board.frames.map(by),
    sections: board.sections.map(by),
    notes: board.notes.map(by),
    comments: board.comments.map((c) => {
      const a = c.anchor;
      return a.type === 'canvas' && areas.some((s) => inside({ x: a.x, y: a.y, width: 0, height: 0 }, s))
        ? { ...c, anchor: { ...a, x: a.x + d.x, y: a.y + d.y } }
        : c;
    }),
  };
}

export const MIN_SIZE = { frame: 200, section: 160, note: 120 } as const;

export function resize (board: Board, id: string, rect: Rect): Board {
  const o = find(board, id);
  if (!isShape(o)) return board;
  const min = MIN_SIZE[o.kind];
  return put(board, { ...o, ...rect, width: Math.max(min, Math.round(rect.width)), height: Math.max(min * 0.5, Math.round(rect.height)) });
}

/**
 * Copies of shapes, offset so they don't sit on the originals, with the arrows between two copied
 * shapes copied too. Answers the board and the copies' ids.
 */
export function duplicate (board: Board, ids: Iterable<string>, d: Point = { x: 40, y: 40 }): { board: Board, ids: string[] } {
  const map = new Map<string, string>();
  const copies: BoardObject[] = [];
  for (const id of ids) {
    const o = find(board, id);
    if (!isShape(o)) continue;
    const copy = { ...o, id: newId(o.kind), x: o.x + d.x, y: o.y + d.y };
    if (copy.kind !== 'note') copy.name = `${copy.name} copy`;
    map.set(o.id, copy.id);
    copies.push(copy);
  }
  for (const a of board.arrows) {
    const from = map.get(a.from);
    const to = map.get(a.to);
    if (from && to) copies.push({ ...a, id: newId('arrow'), from, to });
  }
  return { board: put(board, ...copies), ids: [...map.values()] };
}

/** Line shapes up on one edge, or spread them evenly with the same gap between each. */
export function arrange (board: Board, ids: string[], how: 'left' | 'top' | 'row' | 'column', gap = 80): Board {
  const list = ids.map((id) => find(board, id)).filter(isShape);
  if (list.length < 2) return board;
  const next: Shape[] = [];
  if (how === 'left' || how === 'top') {
    const key = how === 'left' ? 'x' : 'y';
    const edge = Math.min(...list.map((s) => s[key]));
    for (const s of list) next.push({ ...s, [key]: edge });
  } else {
    const [main, size, cross] = how === 'row' ? ['x', 'width', 'y'] as const : ['y', 'height', 'x'] as const;
    const sorted = [...list].sort((a, b) => a[main] - b[main]);
    const start = Math.min(...list.map((s) => s[cross]));
    let at = sorted[0][main];
    for (const s of sorted) {
      next.push({ ...s, [main]: at, [cross]: start });
      at += s[size] + gap;
    }
  }
  return put(board, ...next);
}

/** The smallest rectangle around shapes, or undefined for none. */
export function bounds (list: Rect[]): Rect | undefined {
  if (!list.length) return undefined;
  const x = Math.min(...list.map((r) => r.x));
  const y = Math.min(...list.map((r) => r.y));
  return {
    x, y,
    width: Math.max(...list.map((r) => r.x + r.width)) - x,
    height: Math.max(...list.map((r) => r.y + r.height)) - y,
  };
}

/** A free spot for a new shape: to the right of everything, at the top. */
export function nextSpot (board: Board, gap = 120): Point {
  const all = bounds(shapes(board));
  return all ? { x: all.x + all.width + gap, y: all.y } : { x: 0, y: 0 };
}

export function addFrame (board: Board, init: Partial<Frame> & Pick<Frame, 'route'>): { board: Board, frame: Frame } {
  const at = nextSpot(board);
  const frame: Frame = { kind: 'frame', id: newId('frame'), name: init.route, width: 1280, height: 800, ...at, ...init };
  return { board: put(board, frame), frame };
}

export function addSection (board: Board, init: Partial<Section> = {}): { board: Board, section: Section } {
  const at = nextSpot(board);
  const section: Section = { kind: 'section', id: newId('section'), name: 'Section', width: 1600, height: 1000, ...at, ...init };
  return { board: put(board, section), section };
}

export function addNote (board: Board, init: Partial<Note> = {}): { board: Board, note: Note } {
  const at = nextSpot(board);
  const note: Note = { kind: 'note', id: newId('note'), text: '', width: 280, height: 180, ...at, ...init };
  return { board: put(board, note), note };
}

export function addArrow (board: Board, from: string, to: string, label?: string): { board: Board, arrow?: Arrow } {
  if (from === to || !isShape(find(board, from)) || !isShape(find(board, to))) return { board };
  const same = board.arrows.find((a) => a.from === from && a.to === to);
  if (same) return { board, arrow: same };
  const arrow: Arrow = { kind: 'arrow', id: newId('arrow'), from, to, ...(label ? { label } : {}) };
  return { board: put(board, arrow), arrow };
}

export function addComment (board: Board, init: Pick<Comment, 'anchor' | 'body' | 'author'>): { board: Board, comment: Comment } {
  const comment: Comment = { kind: 'comment', id: newId('comment'), status: 'open', createdAt: new Date().toISOString(), replies: [], ...init };
  return { board: put(board, comment), comment };
}

export function reply (board: Board, id: string, author: string, body: string): Board {
  const c = find(board, id);
  if (c?.kind !== 'comment') return board;
  return put(board, { ...c, replies: [...c.replies, { id: newId('comment'), author, body, createdAt: new Date().toISOString() }] });
}

export const setStatus = (board: Board, id: string, status: Comment['status']): Board =>
  update<Comment>(board, id, { status });
