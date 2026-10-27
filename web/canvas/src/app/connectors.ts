// The board's connectors, routed once for both layers that draw them (the lines under the screens,
// the labels over them). Each route is remembered with the screens near its ends, so moving one
// screen routes again only the arrows it is near, and a far screen never makes a route longer.
import { computed, effectScope, type ComputedRef } from 'vue';
import { bounds, find, isShape, type Arrow, type Point, type Rect, type Shape } from '../model/index.ts';
import { center, intersects, port, type Side } from './geometry.ts';
import { facing, ROUTE, rounded, route, runs, type Routed } from './route.ts';
import type { Store } from './store.ts';

export interface Connector {
  arrow: Arrow;
  /** The SVG path. */
  d: string;
  /** Where its label sits. */
  mid: Point;
}

export interface Connectors {
  list: ComputedRef<Connector[]>;
  /** A shape as routes avoid it: a frame with the room its name takes above it. */
  box: (s: Shape) => Rect;
}

/** How far around the two ends a route looks for screens in its way. */
const NEAR = 480;

const memo = new Map<string, Routed>();
function routeNear (p1: Point, s1: Side, p2: Point, s2: Side, ends: Rect, all: Rect[]): Routed {
  const m = NEAR + ROUTE.margin + ROUTE.stub;
  const area = { x: ends.x - m, y: ends.y - m, width: ends.width + m * 2, height: ends.height + m * 2 };
  const near = all.filter((o) => intersects(o, area));
  const key = [p1.x, p1.y, s1, p2.x, p2.y, s2, ...near.flatMap((o) => [o.x, o.y, o.width, o.height])].join(',');
  let r = memo.get(key);
  if (!r) {
    if (memo.size > 5000) memo.clear();
    r = route(p1, s1, p2, s2, near);
    memo.set(key, r);
  }
  return r;
}

const made = new WeakMap<Store, Connectors>();

export function useConnectors (store: Store): Connectors {
  const had = made.get(store);
  if (had) return had;
  // Kept for the store's life, whichever layer asked first.
  const out = effectScope(true).run(() => connectors(store))!;
  made.set(store, out);
  return out;
}

function connectors (store: Store): Connectors {
  const { state, board } = store;

  /**
   * A frame's name sits above it at a fixed size on screen, so it is taller in the world the further
   * out the view is. Routes keep clear of it; the zoom is taken in half-steps so a wheel turn doesn't
   * route every arrow again.
   */
  const zoom = computed(() => 2 ** (Math.round(Math.log2(state.view.zoom) * 2) / 2));
  const box = (s: Shape): Rect => (s.kind === 'frame' ? { ...s, y: s.y - 40 / zoom.value, height: s.height + 40 / zoom.value } : s);

  const list = computed(() => {
    const b = board.value;
    if (!b) return [];
    const obstacles = [...b.frames, ...b.notes].map(box);
    const ends = b.arrows.flatMap((a) => {
      const from = find(b, a.from);
      const to = find(b, a.to);
      if (!isShape(from) || !isShape(to) || from.id === to.id) return [];
      const r1 = box(from);
      const r2 = box(to);
      const both = bounds([r1, r2])!;
      // The pair of sides with the cheapest route between their middles.
      let pick: { s1: Side, s2: Side, cost: number } = { s1: 'right', s2: 'left', cost: Infinity };
      for (const s1 of facing(r1, r2)) {
        for (const s2 of facing(r2, r1)) {
          const { cost } = routeNear(port(r1, s1), s1, port(r2, s2), s2, both, obstacles);
          if (cost < pick.cost) pick = { s1, s2, cost };
        }
      }
      return [{ a, r1, r2, both, s1: pick.s1, s2: pick.s2 }];
    });

    // Arrows sharing a side of a shape spread along it, ordered by where their other end is, so they
    // don't leave from one point and cross each other.
    const groups = new Map<string, { i: number, key: number }[]>();
    const add = (shape: string, side: Side, i: number, other: Rect) => {
      const k = `${shape}|${side}`;
      const c = center(other);
      const list = groups.get(k) ?? [];
      list.push({ i, key: side === 'left' || side === 'right' ? c.y : c.x });
      groups.set(k, list);
    };
    ends.forEach((e, i) => {
      add(e.a.from, e.s1, i * 2, e.r2);
      add(e.a.to, e.s2, i * 2 + 1, e.r1);
    });
    const along = new Map<number, number>();
    for (const list of groups.values()) {
      list.sort((p, q) => p.key - q.key);
      list.forEach((x, n) => along.set(x.i, list.length === 1 ? 0.5 : 0.3 + (0.4 * n) / (list.length - 1)));
    }

    // Labels: on the longest straight run that's clear of screens and of the labels placed before.
    const placed: Rect[] = [];
    const z = zoom.value;
    return ends.map((e, i): Connector => {
      const { points } = routeNear(port(e.r1, e.s1, along.get(i * 2)), e.s1, port(e.r2, e.s2, along.get(i * 2 + 1)), e.s2, e.both, obstacles);
      let mid = points[Math.floor(points.length / 2)];
      if (e.a.label) {
        const w = (e.a.label.length * 7 + 20) / z;
        const h = 24 / z;
        const at = (p: Point): Rect => ({ x: p.x - w / 2, y: p.y - h / 2, width: w, height: h });
        const spots = runs(points).flatMap((r) => [0.5, 0.3, 0.7].map((t) => ({ x: r.a.x + (r.b.x - r.a.x) * t, y: r.a.y + (r.b.y - r.a.y) * t })));
        const clear = spots.find((p) => !placed.some((l) => intersects(l, at(p))) && !obstacles.some((o) => intersects(o, at(p))));
        mid = clear ?? spots[0] ?? mid;
        placed.push(at(mid));
      }
      return { arrow: e.a, d: rounded(points, 24), mid };
    });
  });

  return { list, box };
}
