// A board is one canvas: the frames on it (live screens of the app), the sections that group them,
// the arrows between them, notes, and the comments people leave for an agent. It is plain JSON, kept
// in the app's repo beside its prototypes, so a board is reviewed and versioned like code, and an
// agent can read or write it directly as well as through the MCP tools.

export const BOARD_VERSION = 1;

export interface Point {
  x: number;
  y: number;
}

export interface Rect extends Point {
  width: number;
  height: number;
}

/** A live screen: the app at `route`, answered by the mock `scenario` (none: the real backend). */
export interface Frame extends Rect {
  id: string;
  kind: 'frame';
  name: string;
  route: string;
  scenario?: string;
}

/** A labelled area. Moving it moves what lies inside it. */
export interface Section extends Rect {
  id: string;
  kind: 'section';
  name: string;
}

export interface Note extends Rect {
  id: string;
  kind: 'note';
  text: string;
}

/** Shows that one thing leads to another: a flow between frames, or a note pointing at one. */
export interface Arrow {
  id: string;
  kind: 'arrow';
  from: string;
  to: string;
  label?: string;
}

/**
 * An element inside a frame, as the frame described it when it was picked. `source` is where the
 * element is written (`src/pages/Home.vue:12:5`); `index` tells apart elements written once and
 * drawn many times (a list's rows), in document order.
 */
export interface ElementRef {
  frame: string;
  source?: string;
  index?: number;
  tag: string;
  component?: string;
  text?: string;
  selector?: string;
}

export type Anchor =
  | ({ type: 'element' } & ElementRef)
  | ({ type: 'canvas' } & Point);

export interface Reply {
  id: string;
  author: string;
  body: string;
  createdAt: string;
}

export interface Comment {
  id: string;
  kind: 'comment';
  anchor: Anchor;
  author: string;
  body: string;
  status: 'open' | 'resolved';
  createdAt: string;
  replies: Reply[];
}

export type Shape = Frame | Section | Note;
export type BoardObject = Shape | Arrow | Comment;

export interface Board {
  version: typeof BOARD_VERSION;
  name: string;
  /** The project it belongs to, for the boards' home; else the folder of most of its screens. */
  project?: string;
  /** Who works on it: ids of the canvas's people, or names. */
  owners?: string[];
  /** Bumped by the server on every save, so a stale save can be told apart and merged. */
  rev: number;
  frames: Frame[];
  sections: Section[];
  notes: Note[];
  arrows: Arrow[];
  comments: Comment[];
}

export const COLLECTIONS = ['frames', 'sections', 'notes', 'arrows', 'comments'] as const;
export type Collection = (typeof COLLECTIONS)[number];

export const collectionOf = (kind: BoardObject['kind']): Collection => `${kind}s` as Collection;

export function emptyBoard (name: string): Board {
  return { version: BOARD_VERSION, name, rev: 0, frames: [], sections: [], notes: [], arrows: [], comments: [] };
}

/** Frame sizes a designer picks from. */
export const FRAME_SIZES = [
  { name: 'Desktop', width: 1440, height: 900 },
  { name: 'Laptop', width: 1280, height: 800 },
  { name: 'Tablet', width: 834, height: 1112 },
  { name: 'Phone', width: 390, height: 844 },
] as const;

/**
 * A board read from disk or the network, checked and filled in. It throws on anything that isn't a
 * board, naming what is wrong, so a hand-edited file fails with its cause.
 */
export function parseBoard (value: unknown): Board {
  if (!isObject(value)) throw new Error('a board is a JSON object');
  if (value.version !== BOARD_VERSION) throw new Error(`a board's version is ${BOARD_VERSION}`);
  const owners = Array.isArray(value.owners) ? value.owners.filter((o): o is string => typeof o === 'string' && !!o.trim()) : [];
  const { version, name, ...rest } = emptyBoard(typeof value.name === 'string' ? value.name : 'Untitled');
  // The details sit under the name in the file, where a person reading it looks.
  const board: Board = {
    version,
    name,
    ...(typeof value.project === 'string' && value.project.trim() ? { project: value.project.trim() } : {}),
    ...(owners.length ? { owners } : {}),
    ...rest,
  };
  board.rev = typeof value.rev === 'number' ? value.rev : 0;
  for (const key of COLLECTIONS) {
    const list = value[key] ?? [];
    if (!Array.isArray(list)) throw new Error(`${key} is a list`);
    for (const item of list) {
      if (!isObject(item) || typeof item.id !== 'string') throw new Error(`each of ${key} has an id`);
    }
    (board[key] as unknown[]) = list.map((item) => ({ ...item, kind: key.slice(0, -1) }));
  }
  for (const c of board.comments) c.replies ??= [];
  return board;
}

const isObject = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
