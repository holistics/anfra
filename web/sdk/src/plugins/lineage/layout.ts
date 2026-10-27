/**
 * Where the Lineage tab puts each card: a column per kind, left to right in `LINEAGE_KINDS`'
 * order, so only the order within a column and the cards' heights are left to choose. Barycenter
 * sweeps order each column to cut crossings, then each card moves level with what it links to as
 * far as its neighbours in the column let it. Cross-filters join two queries of one column, so they
 * are left out, and drawn after as loops out of the cards' right side.
 */
import { LINEAGE_KINDS, orderForLayout, type LineageGraph } from './model';

export interface Size {
  width: number;
  height: number;
}

export interface Box extends Size {
  x: number;
  y: number;
}

/** A card before it is measured. */
export const CARD_SIZE: Size = { width: 240, height: 80 };
/** Between two columns, and between two cards of one column. */
export const COLUMN_GAP = 80;
export const CARD_GAP = 28;
const SWEEPS = 8;

interface Shaped {
  nodes: readonly { id: string }[];
  edges: readonly { id: string }[];
}

/** A graph's shape: its node and edge ids. A new shape is laid out again, and fitted to the view. */
export function shapeOf (graph: Shaped): string {
  return `${graph.nodes.map((n) => n.id).join()}|${graph.edges.map((e) => e.id).join()}`;
}

/** What a layout was made for: the shape, and each card's measured size in whole pixels. */
export function layoutKey (graph: Shaped, sizeOf: (id: string) => Size | undefined): string {
  const sizes = graph.nodes.map((n) => {
    const size = sizeOf(n.id);
    return size ? `${Math.round(size.width)}x${Math.round(size.height)}` : '?';
  });
  return `${shapeOf(graph)}|${sizes.join()}`;
}

/** Each card's box, its top left at or past 0, 0. */
export function layout (graph: LineageGraph, sizeOf: (id: string) => Size | undefined): Map<string, Box> {
  const size = (id: string) => {
    const measured = sizeOf(id);
    return measured && measured.width > 0 && measured.height > 0 ? measured : CARD_SIZE;
  };
  // A column for each kind the graph has, in the model order (queries joined by a cross-filter together).
  const columns = LINEAGE_KINDS
    .map((kind) => orderForLayout(graph).filter((n) => n.kind === kind).map((n) => n.id))
    .filter((column) => column.length > 0);
  const columnOf = new Map(columns.flatMap((column, index) => column.map((id) => [id, index] as const)));
  const left = new Map<string, string[]>();
  const right = new Map<string, string[]>();
  for (const e of graph.edges) {
    if (e.kind === 'crossFilter') continue;
    const [a, b] = [columnOf.get(e.source), columnOf.get(e.target)];
    if (a === undefined || b === undefined || a === b) continue;
    const [from, to] = a < b ? [e.source, e.target] : [e.target, e.source];
    right.set(from, [...(right.get(from) ?? []), to]);
    left.set(to, [...(left.get(to) ?? []), from]);
  }

  const y = new Map<string, number>();
  const center = (id: string) => y.get(id)! + size(id).height / 2;
  const stack = (column: readonly string[]) => {
    let at = 0;
    for (const id of column) {
      y.set(id, at);
      at += size(id).height + CARD_GAP;
    }
  };
  columns.forEach(stack);
  const barycenter = (id: string, by: Map<string, string[]>) => {
    const linked = by.get(id) ?? [];
    return linked.length ? linked.reduce((sum, other) => sum + center(other), 0) / linked.length : center(id);
  };

  // Order: sweep right then left, sorting each column by where its links sit; keep the best seen.
  let best = columns.map((column) => [...column]);
  let fewest = crossings(columns, right);
  for (let sweep = 0; sweep < SWEEPS; sweep++) {
    const toRight = sweep % 2 === 0;
    const order = toRight ? columns.keys() : [...columns.keys()].reverse();
    for (const index of order) {
      const by = toRight ? left : right;
      const keyed = columns[index].map((id, at) => ({ id, at, key: barycenter(id, by) }));
      keyed.sort((a, b) => a.key - b.key || a.at - b.at);
      columns[index] = keyed.map((k) => k.id);
      stack(columns[index]);
    }
    const count = crossings(columns, right);
    if (count < fewest) {
      fewest = count;
      best = columns.map((column) => [...column]);
    }
  }
  best.forEach(stack);

  const linked = new Map([...columnOf.keys()].map((id) => [id, [...(left.get(id) ?? []), ...(right.get(id) ?? [])]]));
  // Place: each card as level with its links as the column's order and gaps let it, both ways in turn.
  for (let sweep = 0; sweep < SWEEPS; sweep++) {
    const toRight = sweep % 2 === 0;
    const order = toRight ? best.keys() : [...best.keys()].reverse();
    for (const index of order) {
      const column = best[index];
      const wanted = column.map((id) => barycenter(id, linked) - size(id).height / 2);
      settle(column.map((id) => size(id).height), wanted).forEach((top, at) => y.set(column[at], top));
    }
  }

  const boxes = new Map<string, Box>();
  const top = Math.min(...[...y.values()]);
  let x = 0;
  for (const column of best) {
    const width = Math.max(...column.map((id) => size(id).width));
    for (const id of column) boxes.set(id, { x, y: y.get(id)! - top, ...size(id) });
    x += width + COLUMN_GAP;
  }
  return boxes;
}

/** How many links cross, between each pair of columns they join. */
function crossings (columns: readonly (readonly string[])[], right: ReadonlyMap<string, string[]>): number {
  const position = new Map(columns.flatMap((column) => column.map((id, at) => [id, at] as const)));
  const columnOf = new Map(columns.flatMap((column, index) => column.map((id) => [id, index] as const)));
  const links = columns.flatMap((column) => column.flatMap((id) => (right.get(id) ?? []).map((to) => ({
    from: columnOf.get(id)!, to: columnOf.get(to)!, a: position.get(id)!, b: position.get(to)!,
  }))));
  let count = 0;
  for (let i = 0; i < links.length; i++) {
    for (let j = i + 1; j < links.length; j++) {
      const [p, q] = [links[i], links[j]];
      if (p.from !== q.from || p.to !== q.to) continue;
      if ((p.a - q.a) * (p.b - q.b) < 0) count++;
    }
  }
  return count;
}

/**
 * The tops closest to `wanted` (least squares) that keep the cards in order, `CARD_GAP` apart:
 * pool adjacent violators, over each top less the room the cards above it take.
 */
export function settle (heights: readonly number[], wanted: readonly number[]): number[] {
  const offsets: number[] = [];
  let at = 0;
  for (const height of heights) {
    offsets.push(at);
    at += height + CARD_GAP;
  }
  const blocks: { sum: number, count: number }[] = [];
  wanted.forEach((top, index) => {
    blocks.push({ sum: top - offsets[index], count: 1 });
    while (blocks.length > 1) {
      const [a, b] = blocks.slice(-2);
      if (a.sum / a.count <= b.sum / b.count) break;
      blocks.splice(-2, 2, { sum: a.sum + b.sum, count: a.count + b.count });
    }
  });
  return blocks.flatMap((block) => Array<number>(block.count).fill(block.sum / block.count)).map((base, index) => base + offsets[index]);
}

export interface Point {
  x: number;
  y: number;
}

/** A plain edge: out of the source's right side, into the target's left, as a curve. */
export function curve (from: Box, to: Box): { path: string, end: Point } {
  const [sx, sy] = [from.x + from.width, from.y + from.height / 2];
  const [tx, ty] = [to.x, to.y + to.height / 2];
  const bend = Math.max(40, Math.abs(tx - sx) / 2);
  return { path: `M${sx},${sy} C${sx + bend},${sy} ${tx - bend},${ty} ${tx},${ty}`, end: { x: tx, y: ty } };
}

const RADIUS = 8;

/**
 * The cross-filters' loops: each card gets a point on its right side per cross-filter it touches,
 * spread evenly; each edge runs out to its lane and back into the target. The shorter the loop,
 * the closer its lane, so loops nest instead of crossing.
 */
export function crossFilterLoops (graph: LineageGraph, boxes: ReadonlyMap<string, Box>): Map<string, string> {
  const edges = graph.edges.filter((e) => e.kind === 'crossFilter' && boxes.has(e.source) && boxes.has(e.target));
  const slots = new Map<string, string[]>();
  for (const e of edges) {
    slots.set(e.source, [...(slots.get(e.source) ?? []), `out:${e.id}`]);
    slots.set(e.target, [...(slots.get(e.target) ?? []), `in:${e.id}`]);
  }
  const point = (id: string, slot: string): Point => {
    const box = boxes.get(id)!;
    const all = slots.get(id)!;
    return { x: box.x + box.width, y: box.y + ((all.indexOf(slot) + 1) / (all.length + 1)) * box.height };
  };
  const span = (e: (typeof edges)[number]) => Math.abs(boxes.get(e.source)!.y - boxes.get(e.target)!.y);
  const ranked = [...edges].sort((a, b) => span(a) - span(b));
  return new Map(ranked.map((e, rank) => {
    const from = point(e.source, `out:${e.id}`);
    const to = point(e.target, `in:${e.id}`);
    const lane = Math.max(from.x, to.x) + 18 + 12 * rank;
    const down = to.y >= from.y ? 1 : -1;
    const r = Math.min(RADIUS, Math.abs(to.y - from.y) / 2);
    const path = [
      `M${from.x},${from.y}`,
      `H${lane - r}`,
      `Q${lane},${from.y} ${lane},${from.y + down * r}`,
      `V${to.y - down * r}`,
      `Q${lane},${to.y} ${lane - r},${to.y}`,
      `H${to.x}`,
    ].join(' ');
    return [e.id, path];
  }));
}
