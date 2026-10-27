import { describe, expect, it } from 'vitest';
import { addArrow, addComment, addFrame, addNote, addSection, emptyBoard, find, TIDY, tidy, type Board, type Frame, type Rect } from './index.ts';

const size = { width: 1280, height: 800 };

/** A board of frames named by letter, scattered, with arrows between them. */
function flow (names: string[], arrows: [string, string][]): { board: Board, f: Record<string, Frame> } {
  let board = emptyBoard('Flow');
  const f: Record<string, Frame> = {};
  names.forEach((name, i) => {
    const added = addFrame(board, { route: `/${name}`, name, x: (i * 7919) % 5000, y: (i * 104729) % 3000, ...size });
    board = added.board;
    f[name] = added.frame;
  });
  for (const [a, b] of arrows) board = addArrow(board, f[a].id, f[b].id).board;
  return { board, f };
}

const at = (board: Board, f: Frame) => find(board, f.id) as Frame;

describe('tidy', () => {
  it('lays the main path out in the first row, left to right', () => {
    const { board, f } = flow(['a', 'b', 'c'], [['a', 'b'], ['b', 'c']]);
    const out = tidy(board);
    const [a, b, c] = [at(out, f.a), at(out, f.b), at(out, f.c)];
    expect(a.y).toBe(b.y);
    expect(b.y).toBe(c.y);
    expect(a.x).toBeLessThan(b.x);
    expect(b.x).toBeLessThan(c.x);
  });

  it('puts a branch under the screen it leaves, and its siblings beside it', () => {
    const { board, f } = flow(['a', 'b', 'c', 'd', 'x', 'y'], [['a', 'b'], ['b', 'c'], ['c', 'd'], ['b', 'x'], ['b', 'y']]);
    const out = tidy(board);
    const [first, second] = [at(out, f.x), at(out, f.y)].sort((p, q) => p.x - q.x);
    expect(first.x).toBe(at(out, f.b).x);
    expect(first.y).toBeGreaterThan(at(out, f.b).y);
    expect(second.y).toBe(first.y);
    expect(second.x).toBe(at(out, f.c).x);
  });

  it('follows the longest way as the main path', () => {
    const { board, f } = flow(['a', 'short', 'b', 'c'], [['a', 'short'], ['a', 'b'], ['b', 'c']]);
    const out = tidy(board);
    expect(at(out, f.b).y).toBe(at(out, f.a).y);
    expect(at(out, f.short).y).toBeGreaterThan(at(out, f.a).y);
  });

  it('copes with a cycle', () => {
    const { board, f } = flow(['a', 'b', 'c'], [['a', 'b'], ['b', 'c'], ['c', 'a']]);
    const out = tidy(board);
    expect(at(out, f.a).x).toBeLessThan(at(out, f.b).x);
    expect(at(out, f.b).x).toBeLessThan(at(out, f.c).x);
  });

  it('leaves no frames overlapping, and puts frames on no arrow in a grid below', () => {
    const { board, f } = flow(['a', 'b', 'p', 'q'], [['a', 'b']]);
    const out = tidy(board);
    expect(overlapping(out.frames)).toBe(false);
    expect(at(out, f.p).y).toBeGreaterThan(at(out, f.a).y);
    expect(at(out, f.q).y).toBe(at(out, f.p).y);
  });

  it('keeps frames in their sections and sizes each section to hold them', () => {
    let { board, f } = flow(['a', 'b', 'x'], [['a', 'b'], ['a', 'x']]);
    board = addSection(board, { name: 'Happy', x: -5000, y: -5000, width: 20000, height: 5000 - 1 }).board;
    board = { ...board, frames: board.frames.map((fr) => (fr.id === f.x.id ? fr : { ...fr, y: -4000 })) };
    board = addSection(board, { name: 'Edge', x: -5000, y: 0, width: 20000, height: 10000 }).board;
    const out = tidy(board);
    const [happy, edge] = out.sections;
    for (const id of [f.a.id, f.b.id]) expect(inside(at(out, { id } as Frame), happy)).toBe(true);
    expect(inside(at(out, f.x), edge)).toBe(true);
    expect(edge.y).toBeGreaterThan(happy.y + happy.height);
    expect(happy.width).toBeLessThan(5000);
  });

  it('moves a note on an arrow, and a comment pinned on a frame, with the frame', () => {
    let { board, f } = flow(['a', 'b'], [['a', 'b']]);
    const note = addNote(board, { x: f.b.x + 100, y: f.b.y - 300, width: 200, height: 120 });
    board = addArrow(note.board, note.note.id, f.b.id).board;
    const pin = addComment(board, { anchor: { type: 'canvas', x: f.b.x + 10, y: f.b.y + 10 }, author: 'h', body: 'here' });
    const out = tidy(pin.board);
    const b = at(out, f.b);
    expect(find(out, note.note.id)).toMatchObject({ x: b.x + 100, y: b.y - 300 });
    expect(out.comments[0].anchor).toMatchObject({ x: b.x + 10, y: b.y + 10 });
  });

  it('leaves a tidy board as it is, with each edge case under the step it leaves', () => {
    let board = emptyBoard('Getting started');
    const section = (name: string, y: number) => {
      board = addSection(board, { name, x: 0, y, width: 4640, height: 1000 }).board;
    };
    const frame = (name: string, x: number, y: number) => {
      const added = addFrame(board, { route: '/', name, x, y, ...size });
      board = added.board;
      return added.frame;
    };
    section('Happy path', 0);
    section('Edge cases', 1400);
    const start = frame('New', 80, 120);
    const sources = frame('Sources', 1680, 120);
    const done = frame('Done', 3280, 120);
    const error = frame('Error', 80, 1520);
    const slow = frame('Slow', 1680, 1520);
    const explorer = frame('Explorer', 3280, 1520);
    board = addNote(board, { x: 4800, y: 120, width: 360, height: 220 }).board;
    for (const [a, b] of [[start, sources], [sources, done], [start, error], [start, slow], [start, explorer]]) {
      board = addArrow(board, a.id, b.id).board;
    }
    const once = tidy(board);
    expect(tidy(once)).toEqual(once);
    expect(at(once, error).x).toBe(at(once, start).x);
    expect(at(once, slow).x).toBe(at(once, sources).x);
    expect(at(once, explorer).x).toBe(at(once, done).x);
    expect(at(once, error).y).toBe(at(once, slow).y);
    const [happy, edge] = once.sections;
    expect(inside(at(once, start), happy) && inside(at(once, error), edge)).toBe(true);
    expect(find(once, board.notes[0].id)).toMatchObject({ x: happy.x + happy.width + TIDY.gapX / 2 });
  });
});

const inside = (r: Frame, s: { x: number, y: number, width: number, height: number }) =>
  r.x >= s.x && r.y >= s.y && r.x + r.width <= s.x + s.width && r.y + r.height <= s.y + s.height;

const overlapping = (list: Rect[]) =>
  list.some((a, i) => list.slice(i + 1).some((b) =>
    a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height));
