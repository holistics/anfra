// Connectors as a designer draws them: right-angled lines with rounded corners that leave and
// enter square to a side and go around the screens between, never through them. Each one is the
// cheapest path on a sparse grid made of the obstacles' edges (A*, where a bend costs extra), so
// lines keep to the gaps between screens and turn as little as they can.
import type { Point, Rect } from '../model/board.ts';
import { center, type Side } from './geometry.ts';

const NORMAL: Record<Side, Point> = { left: { x: -1, y: 0 }, right: { x: 1, y: 0 }, top: { x: 0, y: -1 }, bottom: { x: 0, y: 1 } };
const SIDES: Side[] = ['top', 'right', 'bottom', 'left'];
const DIRS: Point[] = [{ x: 1, y: 0 }, { x: -1, y: 0 }, { x: 0, y: 1 }, { x: 0, y: -1 }];

export interface RouteOptions {
  /** Kept clear around every obstacle. */
  margin: number;
  /** How far a line goes straight out of its side before it may turn. */
  stub: number;
  /** What a bend costs, in units of length. */
  bend: number;
}

export const ROUTE: RouteOptions = { margin: 40, stub: 56, bend: 160 };

export interface Routed {
  points: Point[];
  cost: number;
}

const inside = (r: Rect, p: Point) => p.x > r.x + 0.5 && p.x < r.x + r.width - 0.5 && p.y > r.y + 0.5 && p.y < r.y + r.height - 0.5;
const grow = (r: Rect, m: number): Rect => ({ x: r.x - m, y: r.y - m, width: r.width + m * 2, height: r.height + m * 2 });

/** The sides worth trying between two boxes: those that face the other one. */
export function facing (a: Rect, b: Rect): Side[] {
  const ca = center(a);
  const cb = center(b);
  const out = SIDES.filter((s) => {
    const n = NORMAL[s];
    const edge = s === 'left' ? a.x : s === 'right' ? a.x + a.width : s === 'top' ? a.y : a.y + a.height;
    const toward = n.x ? (cb.x - edge) * n.x : (cb.y - edge) * n.y;
    return toward > 0 || (n.x ? (cb.x - ca.x) * n.x : (cb.y - ca.y) * n.y) > 0;
  });
  return out.length ? out : SIDES;
}

/**
 * The cheapest right-angled path from a port on one side to a port on another, around the
 * obstacles. Answers the corner points, the ports included.
 */
export function route (port1: Point, s1: Side, port2: Point, s2: Side, obstacles: Rect[], o: RouteOptions = ROUTE): Routed {
  // On whole pixels, as the grid is, so a run from a port to the grid is straight.
  const p1 = { x: Math.round(port1.x), y: Math.round(port1.y) };
  const p2 = { x: Math.round(port2.x), y: Math.round(port2.y) };
  const n1 = NORMAL[s1];
  const n2 = NORMAL[s2];
  const a = { x: p1.x + n1.x * o.stub, y: p1.y + n1.y * o.stub };
  const b = { x: p2.x + n2.x * o.stub, y: p2.y + n2.y * o.stub };
  const blocks = obstacles.map((r) => grow(r, o.margin));
  const free = (p: Point) => !blocks.some((r) => inside(r, p));

  // The grid: every obstacle edge, the stubs' ends, and the middle of each gap between edges.
  const axis = (pick: (r: Rect) => number[], ends: number[]) => {
    const raw = [...new Set([...ends, ...blocks.flatMap(pick)].map((v) => Math.round(v)))].sort((p, q) => p - q);
    const mids = raw.slice(1).map((v, i) => Math.round((v + raw[i]) / 2));
    return [...new Set([...raw, ...mids])].sort((p, q) => p - q);
  };
  const xs = axis((r) => [r.x, r.x + r.width], [a.x, b.x]);
  const ys = axis((r) => [r.y, r.y + r.height], [a.y, b.y]);
  const W = xs.length;
  const node = (i: number, j: number) => j * W + i;
  const at = (k: number): Point => ({ x: xs[k % W], y: ys[Math.floor(k / W)] });
  const start = node(xs.indexOf(Math.round(a.x)), ys.indexOf(Math.round(a.y)));
  const goal = node(xs.indexOf(Math.round(b.x)), ys.indexOf(Math.round(b.y)));
  const startDir = DIRS.findIndex((d) => d.x === n1.x && d.y === n1.y);
  const goalDir = DIRS.findIndex((d) => d.x === -n2.x && d.y === -n2.y);

  // A* over (node, heading). A step costs its length; a turn costs `bend` more; no U-turns.
  const H = (k: number) => Math.abs(at(k).x - b.x) + Math.abs(at(k).y - b.y);
  const best = new Map<number, number>();
  const from = new Map<number, number>();
  const heap = new Heap();
  const closed = new Set<number>();
  const s0 = start * 4 + startDir;
  best.set(s0, 0);
  heap.push(s0, H(start));
  let end = -1;
  while (heap.size) {
    const s = heap.pop();
    const k = Math.floor(s / 4);
    const dir = s % 4;
    if (closed.has(s)) continue;
    closed.add(s);
    const g = best.get(s)!;
    if (k === goal) {
      end = s;
      break;
    }
    const i = k % W;
    const j = Math.floor(k / W);
    for (let d = 0; d < 4; d++) {
      if (d === (dir ^ 1)) continue; // straight back
      const ni = i + DIRS[d].x;
      const nj = j + DIRS[d].y;
      if (ni < 0 || nj < 0 || ni >= W || nj >= ys.length) continue;
      const nk = node(ni, nj);
      const p = at(k);
      const q = at(nk);
      const mid = { x: (p.x + q.x) / 2, y: (p.y + q.y) / 2 };
      if ((nk !== goal && !free(q)) || !free(mid)) continue;
      let cost = g + Math.abs(q.x - p.x) + Math.abs(q.y - p.y) + (d === dir ? 0 : o.bend);
      if (nk === goal && d !== goalDir) cost += o.bend * 2; // entering the side askew
      const ns = nk * 4 + d;
      if (cost < (best.get(ns) ?? Infinity)) {
        best.set(ns, cost);
        from.set(ns, s);
        heap.push(ns, cost + H(nk));
      }
    }
  }

  if (end < 0) {
    // Walled in: an elbow through the middle, which is at least square to both sides.
    const m = n1.x ? { x: (a.x + b.x) / 2, y: a.y } : { x: a.x, y: (a.y + b.y) / 2 };
    const m2 = n1.x ? { x: m.x, y: b.y } : { x: b.x, y: m.y };
    return { points: simplify([p1, a, m, m2, b, p2]), cost: Infinity };
  }
  const path: Point[] = [];
  for (let s: number | undefined = end; s !== undefined; s = from.get(s)) path.unshift(at(Math.floor(s / 4)));
  return { points: simplify([p1, ...path, p2]), cost: best.get(end)! };
}

/** Drop points in the middle of a straight run, and repeats. */
export function simplify (points: Point[]): Point[] {
  const out: Point[] = [];
  for (const p of points) {
    const prev = out.at(-1);
    if (prev && prev.x === p.x && prev.y === p.y) continue;
    const before = out.at(-2);
    if (before && prev && ((before.x === prev.x && prev.x === p.x) || (before.y === prev.y && prev.y === p.y))) out.pop();
    out.push(p);
  }
  return out;
}

/** An SVG path along the points, its corners rounded by up to `radius`. */
export function rounded (points: Point[], radius: number): string {
  if (points.length < 2) return '';
  let d = `M${points[0].x} ${points[0].y}`;
  for (let i = 1; i < points.length - 1; i++) {
    const [p, c, n] = [points[i - 1], points[i], points[i + 1]];
    const r = Math.min(radius, Math.hypot(c.x - p.x, c.y - p.y) / 2, Math.hypot(n.x - c.x, n.y - c.y) / 2);
    const inDir = { x: Math.sign(c.x - p.x), y: Math.sign(c.y - p.y) };
    const outDir = { x: Math.sign(n.x - c.x), y: Math.sign(n.y - c.y) };
    d += ` L${c.x - inDir.x * r} ${c.y - inDir.y * r} Q${c.x} ${c.y} ${c.x + outDir.x * r} ${c.y + outDir.y * r}`;
  }
  const last = points.at(-1)!;
  return `${d} L${last.x} ${last.y}`;
}

/** The straight runs of a path, longest first: where a label can sit. */
export function runs (points: Point[]): { a: Point, b: Point, length: number }[] {
  return points.slice(1)
    .map((b, i) => ({ a: points[i], b, length: Math.abs(b.x - points[i].x) + Math.abs(b.y - points[i].y) }))
    .sort((p, q) => q.length - p.length);
}

/** A binary min-heap of numbers by priority. */
class Heap {
  private items: { v: number, p: number }[] = [];
  get size () {
    return this.items.length;
  }

  push (v: number, p: number) {
    const a = this.items;
    a.push({ v, p });
    let i = a.length - 1;
    while (i > 0) {
      const up = (i - 1) >> 1;
      if (a[up].p <= a[i].p) break;
      [a[up], a[i]] = [a[i], a[up]];
      i = up;
    }
  }

  pop (): number {
    const a = this.items;
    const top = a[0];
    const last = a.pop()!;
    if (a.length) {
      a[0] = last;
      let i = 0;
      for (;;) {
        const l = i * 2 + 1;
        const r = l + 1;
        let m = i;
        if (l < a.length && a[l].p < a[m].p) m = l;
        if (r < a.length && a[r].p < a[m].p) m = r;
        if (m === i) break;
        [a[m], a[i]] = [a[i], a[m]];
        i = m;
      }
    }
    return top.v;
  }
}
