import { describe, expect, it } from 'vitest';
import { rounded, route, simplify } from './route.ts';

const a = { x: 0, y: 0, width: 1280, height: 800 };
const b = { x: 1760, y: 0, width: 1280, height: 800 };
const square = (points: { x: number, y: number }[]) => points.slice(1).every((p, i) => p.x === points[i].x || p.y === points[i].y);

describe('route', () => {
  it('runs straight between two screens side by side, even from a port off the pixel grid', () => {
    const { points } = route({ x: 1280, y: 399.7 }, 'right', { x: 1760, y: 400.2 }, 'left', [a, b]);
    expect(points).toEqual([{ x: 1280, y: 400 }, { x: 1760, y: 400 }]);
  });

  it('goes around a screen in the way, square at every turn', () => {
    const c = { x: 3520, y: 0, width: 1280, height: 800 };
    const { points } = route({ x: 1280, y: 400 }, 'right', { x: 3520, y: 400 }, 'left', [a, b, c]);
    expect(square(points)).toBe(true);
    const crosses = points.slice(1).some((p, i) => {
      const q = points[i];
      const x0 = Math.min(p.x, q.x), x1 = Math.max(p.x, q.x), y0 = Math.min(p.y, q.y), y1 = Math.max(p.y, q.y);
      return x1 > b.x && x0 < b.x + b.width && y1 > b.y && y0 < b.y + b.height;
    });
    expect(crosses).toBe(false);
  });
});

describe('simplify and rounded', () => {
  it('drops points in the middle of a run and rounds each corner', () => {
    expect(simplify([{ x: 0, y: 0 }, { x: 5, y: 0 }, { x: 10, y: 0 }, { x: 10, y: 10 }])).toEqual([{ x: 0, y: 0 }, { x: 10, y: 0 }, { x: 10, y: 10 }]);
    expect(rounded([{ x: 0, y: 0 }, { x: 100, y: 0 }, { x: 100, y: 100 }], 24)).toBe('M0 0 L76 0 Q100 0 100 24 L100 100');
  });
});
