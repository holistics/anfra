import {
  afterEach, describe, expect, it,
} from 'vitest';
import { createSdk } from './sdk';
import {
  buildStructure, describeNode, markersFor, parseRef, usageKey,
} from './structure';
import { salesDataset, stubBackend, testUser } from './testSupport';
import type { App } from './app';
import type { StructureNode } from '../common/types';

function build () {
  return createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend });
}

function revenueApp (app: App) {
  const revenue = app.createQuery('revenue', {
    dataset: 'sales',
    aql: 'explore { dimensions { region: users.region } measures { total: orders | sum(orders.amount) } }',
  });
  const region = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
  return { revenue, region };
}

function html (markup: string): void {
  document.body.innerHTML = markup;
}

/** The tree as `kind id|name` lines, indented by depth, for readable assertions. */
function outline (nodes: StructureNode[], depth = 0): string[] {
  return nodes.flatMap((node) => [
    `${'  '.repeat(depth)}${node.kind} ${node.id ?? node.name}${node.problems.length ? ' !' : ''}`,
    ...outline(node.children, depth + 1),
  ]);
}

afterEach(() => { document.body.innerHTML = ''; });

describe('parseRef', () => {
  it('reads a bare name as app 0, and app/name as written', () => {
    expect(parseRef('revenue')).toEqual({ app: 0, name: 'revenue' });
    expect(parseRef('2/revenue')).toEqual({ app: 2, name: 'revenue' });
    expect(parseRef('a/b/c')).toBeUndefined();
    expect(parseRef('')).toBeUndefined();
  });
});

describe('buildStructure', () => {
  it('reads containers, blocks and markers off the page in DOM order, nested as they are', () => {
    const sdk = build();
    revenueApp(sdk.createApp());
    html(`
      <section data-anfra-container="page" data-anfra-label="Page">
        <div data-anfra-block="kpis" data-anfra-query="revenue"></div>
        <div data-anfra-block="chart">
          <div data-anfra-query="revenue"></div>
          <div data-anfra-control="region"></div>
        </div>
      </section>
      <div data-anfra-control="region"></div>
    `);
    const structure = buildStructure(document, sdk.apps);
    expect(outline(structure.nodes)).toEqual([
      'container page',
      '  block kpis',
      '    query revenue',
      '  block chart',
      '    query revenue',
      '    control region',
      'control region',
    ]);
    expect(structure.nodes[0].label).toBe('Page');
    expect(structure.nodes[0].children[0].key).toBe('0.0');
    expect(structure.nodes[0].children[1].children[1].key).toBe('0.1.1');
    expect(structure.usage).toEqual({
      'query:0/revenue': { markers: 2, blocks: ['kpis', 'chart'] },
      'control:0/region': { markers: 2, blocks: ['chart'] },
    });
  });

  it('maps each key to its element, and an element to every node it stands for', () => {
    const sdk = build();
    revenueApp(sdk.createApp());
    html('<div id="tile" data-anfra-block="kpis" data-anfra-query="revenue"></div>');
    const structure = buildStructure(document, sdk.apps);
    const tile = document.getElementById('tile')!;
    expect(structure.elements.get('0')).toBe(tile);
    expect(structure.elements.get('0.0')).toBe(tile);
    expect(structure.keysOf.get(tile)).toEqual(['0', '0.0']);
  });

  it('is empty for a page that marks nothing, at no cost to the app', () => {
    const sdk = build();
    revenueApp(sdk.createApp());
    html('<div><canvas></canvas></div>');
    expect(buildStructure(document, sdk.apps)).toMatchObject({ nodes: [], usage: {} });
  });

  it('reports problems on the node they concern, and never throws', () => {
    const sdk = build();
    revenueApp(sdk.createApp());
    html(`
      <div data-anfra-block="outer">
        <div data-anfra-block="inner"></div>
        <div data-anfra-container="c"></div>
      </div>
      <div data-anfra-block="outer"></div>
      <div data-anfra-block=""></div>
      <div data-anfra-container="both" data-anfra-block="both"></div>
      <div data-anfra-query="nope"></div>
      <div data-anfra-query="3/revenue"></div>
      <div data-anfra-control="bad/ref/here"></div>
    `);
    const { nodes, usage } = buildStructure(document, sdk.apps);
    const problems = Object.fromEntries(
      (function walk (list: StructureNode[]): [string, string[]][] {
        return list.flatMap((node) => [[`${node.key} ${node.kind} ${node.id ?? node.name}`, node.problems], ...walk(node.children)]);
      }(nodes)).filter(([, p]) => p.length),
    );
    expect(problems).toEqual({
      '0.0 block inner': ['Inside block "outer", but a block holds no container or block.'],
      '0.1 container c': ['Inside block "outer", but a block holds no container or block.'],
      '1 block outer': ['Another block already has the id "outer".'],
      '2 block ': ['An empty block id.'],
      '3 container both': ['Marked as both a container and a block; read as a container.'],
      '4 query nope': ['App 0 has no query named "nope".'],
      '5 query revenue': ['There is no app 3 in this frame.'],
      '6 control bad/ref/here': ['"bad/ref/here" is not a reference: write "name" or "app/name".'],
    });
    expect(usage).toEqual({});
  });

  it('names an entity of a later app by its index', () => {
    const sdk = build();
    revenueApp(sdk.createApp());
    sdk.createApp().createQuery('other', { dataset: 'sales', aql: 'explore { measures { n: orders | count() } }' });
    html('<div data-anfra-query="1/other revenue"></div>');
    const { nodes, usage } = buildStructure(document, sdk.apps);
    expect(outline(nodes)).toEqual(['query other', 'query revenue']);
    expect(nodes[0]).toMatchObject({ app: 1, name: 'other', problems: [] });
    expect(Object.keys(usage)).toEqual([usageKey('query', 1, 'other'), usageKey('query', 0, 'revenue')]);
  });
});

describe('markersFor and locate', () => {
  it('finds the elements marked with an entity, by bare name or app/name', () => {
    html('<i data-anfra-query="revenue"></i><b data-anfra-query="0/revenue other"></b><u data-anfra-query="1/revenue"></u>');
    expect(markersFor(document, 'query', 0, 'revenue').map((el) => el.tagName)).toEqual(['I', 'B']);
    expect(markersFor(document, 'query', 1, 'revenue').map((el) => el.tagName)).toEqual(['U']);
    expect(markersFor(document, 'control', 0, 'revenue')).toEqual([]);
  });

  it('is how a query or control locates itself: false with nothing marked, true and shown otherwise', () => {
    const sdk = build();
    const { revenue, region } = revenueApp(sdk.createApp());
    expect(revenue.locate()).toBe(false);
    html('<div data-anfra-query="revenue"></div><div data-anfra-control="region"></div>');
    expect(revenue.locate()).toBe(true);
    expect(region.locate()).toBe(true);
    expect(document.getElementById('anfra-overlay')?.style.display).toBe('block');
    expect(document.querySelector('[data-testid="anfra-overlay-label"]')?.textContent).toBe('control region');
  });

  it('describes a node for the overlay by kind, id, label and size', () => {
    const sdk = build();
    revenueApp(sdk.createApp());
    html('<div data-anfra-block="kpis" data-anfra-label="KPIs" data-anfra-query="revenue"></div><div data-anfra-query="revenue"></div>');
    const structure = buildStructure(document, sdk.apps);
    const tile = structure.elements.get('0')!;
    expect(describeNode(structure.nodes[0], tile, structure)).toBe('block kpis · KPIs · 0 × 0');
    expect(describeNode(structure.nodes[0].children[0], tile, structure)).toBe('query revenue · 1 of 2 · 0 × 0');
  });
});
