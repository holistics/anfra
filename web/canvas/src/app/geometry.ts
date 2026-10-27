import type { Point, Rect } from '../model/board.ts';

export const center = (r: Rect): Point => ({ x: r.x + r.width / 2, y: r.y + r.height / 2 });

export const contains = (r: Rect, p: Point): boolean => p.x >= r.x && p.x <= r.x + r.width && p.y >= r.y && p.y <= r.y + r.height;

export const intersects = (a: Rect, b: Rect): boolean =>
  a.x < b.x + b.width && a.x + a.width > b.x && a.y < b.y + b.height && a.y + a.height > b.y;

/** The rectangle between two corners, whichever way they were dragged. */
export const between = (a: Point, b: Point): Rect => ({
  x: Math.min(a.x, b.x), y: Math.min(a.y, b.y), width: Math.abs(a.x - b.x), height: Math.abs(a.y - b.y),
});

/** Where the line from a rectangle's center towards a point leaves the rectangle. */
export function edgePoint (r: Rect, toward: Point): Point {
  const c = center(r);
  const dx = toward.x - c.x;
  const dy = toward.y - c.y;
  if (!dx && !dy) return c;
  const sx = dx ? r.width / 2 / Math.abs(dx) : Infinity;
  const sy = dy ? r.height / 2 / Math.abs(dy) : Infinity;
  const s = Math.min(sx, sy);
  return { x: c.x + dx * s, y: c.y + dy * s };
}

export type Side = 'left' | 'right' | 'top' | 'bottom';

const NORMAL: Record<Side, Point> = { left: { x: -1, y: 0 }, right: { x: 1, y: 0 }, top: { x: 0, y: -1 }, bottom: { x: 0, y: 1 } };

/**
 * The sides a connector leaves one rectangle by and enters another by: across the wider gap
 * between them, so a flow reads left to right or top to bottom the way the shapes are laid out.
 */
export function sides (a: Rect, b: Rect): [Side, Side] {
  const gapX = Math.max(b.x - (a.x + a.width), a.x - (b.x + b.width));
  const gapY = Math.max(b.y - (a.y + a.height), a.y - (b.y + b.height));
  const ca = center(a);
  const cb = center(b);
  if (gapX >= gapY) return cb.x >= ca.x ? ['right', 'left'] : ['left', 'right'];
  return cb.y >= ca.y ? ['bottom', 'top'] : ['top', 'bottom'];
}

/** A point on a side of a rectangle, `t` of the way along it (0.5 is its middle). */
export function port (r: Rect, side: Side, t = 0.5): Point {
  if (side === 'left') return { x: r.x, y: r.y + r.height * t };
  if (side === 'right') return { x: r.x + r.width, y: r.y + r.height * t };
  if (side === 'top') return { x: r.x + r.width * t, y: r.y };
  return { x: r.x + r.width * t, y: r.y + r.height };
}

export interface Curve {
  d: string;
  mid: Point;
}

/** A smooth connector between two ports, leaving and entering square to their sides. */
export function curve (p1: Point, s1: Side, p2: Point, s2: Side): Curve {
  const reach = Math.max(48, Math.hypot(p2.x - p1.x, p2.y - p1.y) * 0.4);
  const n1 = NORMAL[s1];
  const n2 = NORMAL[s2];
  const c1 = { x: p1.x + n1.x * reach, y: p1.y + n1.y * reach };
  const c2 = { x: p2.x + n2.x * reach, y: p2.y + n2.y * reach };
  return {
    d: `M${p1.x} ${p1.y} C${c1.x} ${c1.y} ${c2.x} ${c2.y} ${p2.x} ${p2.y}`,
    // The curve's point at t = 0.5
    mid: { x: (p1.x + 3 * c1.x + 3 * c2.x + p2.x) / 8, y: (p1.y + 3 * c1.y + 3 * c2.y + p2.y) / 8 },
  };
}
