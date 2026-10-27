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

/** A point on a side of a rectangle, `t` of the way along it (0.5 is its middle). */
export function port (r: Rect, side: Side, t = 0.5): Point {
  if (side === 'left') return { x: r.x, y: r.y + r.height * t };
  if (side === 'right') return { x: r.x + r.width, y: r.y + r.height * t };
  if (side === 'top') return { x: r.x + r.width * t, y: r.y };
  return { x: r.x + r.width * t, y: r.y + r.height };
}
