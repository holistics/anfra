import { describe, expect, it } from 'vitest';
import {
  CARD_GAP, CARD_SIZE, COLUMN_GAP, crossFilterLoops, layout, layoutKey, settle, shapeOf,
} from './layout';
import type { LineageGraph, LineageGraphNode, LineageKind } from './model';

const node = (kind: LineageKind, name: string): LineageGraphNode => ({ id: `${kind}:${name}`, kind, label: name });
const edge = (source: string, target: string, extra: Partial<LineageGraph['edges'][number]> = {}) => ({ id: `${source}->${target}`, source, target, fields: [], ...extra });
const unmeasured = () => undefined;

describe('layoutKey', () => {
  const graph = { nodes: [{ id: 'a' }, { id: 'b' }], edges: [{ id: 'a->b' }] };
  const sizes = (over: Record<string, { width: number, height: number }> = {}) => (id: string) => over[id] ?? CARD_SIZE;

  it('is the same for the same ids and sizes, and ignores a change under a pixel', () => {
    expect(layoutKey(graph, sizes())).toBe(layoutKey({ ...graph }, sizes()));
    expect(layoutKey(graph, sizes())).toBe(layoutKey(graph, sizes({ b: { width: 240.2, height: 80.4 } })));
  });

  it('changes when a card grows by a pixel, and with the ids and their order', () => {
    expect(layoutKey(graph, sizes())).not.toBe(layoutKey(graph, sizes({ b: { width: 240, height: 81 } })));
    const other = { nodes: [{ id: 'b' }, { id: 'a' }], edges: graph.edges };
    expect(shapeOf(graph)).not.toBe(shapeOf(other));
  });
});

describe('layout', () => {
  it('puts each kind in its own column, left to right, skipping the kinds the graph has none of', () => {
    const graph: LineageGraph = {
      nodes: [node('dataset', 'sales'), node('query', 'q'), node('block', 'b')],
      edges: [edge('block:b', 'query:q'), edge('query:q', 'dataset:sales')],
    };
    const boxes = layout(graph, unmeasured);
    expect(boxes.get('block:b')!.x).toBe(0);
    expect(boxes.get('query:q')!.x).toBe(CARD_SIZE.width + COLUMN_GAP);
    expect(boxes.get('dataset:sales')!.x).toBe(2 * (CARD_SIZE.width + COLUMN_GAP));
  });

  it('keeps a column\'s cards apart, and a column as wide as its widest card', () => {
    const graph: LineageGraph = { nodes: [node('query', 'a'), node('query', 'b'), node('dataset', 'd')], edges: [] };
    const boxes = layout(graph, (id) => (id === 'query:b' ? { width: 300, height: 120 } : undefined));
    const [a, b] = [boxes.get('query:a')!, boxes.get('query:b')!];
    expect(Math.abs(a.y - b.y)).toBeGreaterThanOrEqual(CARD_SIZE.height + CARD_GAP);
    expect(boxes.get('dataset:d')!.x).toBe(300 + COLUMN_GAP);
  });

  it('orders a column so its links cross less', () => {
    // a → y and b → x, given in the order that crosses.
    const graph: LineageGraph = {
      nodes: [node('query', 'a'), node('query', 'b'), node('metric', 'x'), node('metric', 'y')],
      edges: [edge('query:a', 'metric:y'), edge('query:b', 'metric:x')],
    };
    const boxes = layout(graph, unmeasured);
    const above = (p: string, q: string) => boxes.get(p)!.y < boxes.get(q)!.y;
    expect(above('query:a', 'query:b')).toBe(above('metric:y', 'metric:x'));
  });

  it('puts a lone card level with the one it links to', () => {
    const graph: LineageGraph = {
      nodes: [node('query', 'a'), node('query', 'b'), node('query', 'c'), node('dataset', 'd')],
      edges: [edge('query:a', 'dataset:d'), edge('query:b', 'dataset:d'), edge('query:c', 'dataset:d')],
    };
    const boxes = layout(graph, unmeasured);
    expect(boxes.get('dataset:d')!.y).toBeCloseTo(boxes.get('query:b')!.y);
  });
});

describe('settle', () => {
  it('keeps the wanted tops that have room', () => {
    expect(settle([80, 80], [0, 200])).toEqual([0, 200]);
  });

  it('spreads cards that want the same place around it', () => {
    const [a, b] = settle([80, 80], [100, 100]);
    expect(b - a).toBe(80 + CARD_GAP);
    expect((a + b) / 2).toBe(100);
  });
});

describe('crossFilterLoops', () => {
  it('nests the shorter loop inside the longer', () => {
    const graph: LineageGraph = {
      nodes: [node('query', 'a'), node('query', 'b'), node('query', 'c')],
      edges: [
        edge('query:a', 'query:c', { id: 'long', kind: 'crossFilter' }),
        edge('query:a', 'query:b', { id: 'short', kind: 'crossFilter' }),
      ],
    };
    const at = (y: number) => ({ x: 0, y, ...CARD_SIZE });
    const loops = crossFilterLoops(graph, new Map([['query:a', at(0)], ['query:b', at(100)], ['query:c', at(200)]]));
    const lane = (path: string) => Number(/Q([\d.]+),/.exec(path)![1]);
    expect(lane(loops.get('short')!)).toBeLessThan(lane(loops.get('long')!));
  });
});
