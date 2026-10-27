import { describe, expect, it } from 'vitest';
import {
  ORDERS_AQL, REVENUE_AQL, column, salesApp, salesSnapshot,
} from '../tabs/testSupport';
import { changesOf } from './changes';
import { dedent } from './format';
import { declaredFilters, isActive, queryInputs } from './inputs';
import { issuesOf, queryCounts } from './issues';
import { attributeRequests, type RequestEntry } from './requests';
import { appTrust, queryTrust } from './trust';

const entry = (id: number, aql: string, extra: Partial<RequestEntry> = {}): RequestEntry => ({
  id, call: 'core.submitQuery', caller: 'frame', at: '2026-10-27T00:00:00Z', status: 'ok', dataset: 'sales', aql, ...extra,
});

describe('trust', () => {
  it('rates a query by its dataset, its state and its ad hoc fields', () => {
    const app = salesApp();
    expect(queryTrust(undefined, app.queries.revenue)).toMatchObject({ level: 1 });
    expect(queryTrust(app.declaration.queries.orders, app.queries.orders)).toMatchObject({ level: 2, text: 'On the semantic layer, but failed' });
    expect(queryTrust(app.declaration.queries.revenue, app.queries.revenue)).toEqual({ level: 3, tone: 'ok', text: 'Semantic layer verified' });
    const adhoc = { ...app.queries.revenue, columns: [column('x', false, { adhoc: true })] };
    expect(queryTrust(app.declaration.queries.revenue, adhoc).text).toBe('Semantic layer verified · 1 ad hoc field');
  });

  it("rates an app by its lowest query, or 1 with none", () => {
    expect(appTrust(salesApp())).toMatchObject({ level: 2, text: '1 of 2 queries failed' });
    expect(appTrust(salesApp({ queries: {} }))).toMatchObject({ level: 1, text: 'No queries found' });
  });
});

describe('inputs', () => {
  it("lists a query's controls and cross-filters, with what it last ran with", () => {
    const inputs = queryInputs(salesApp(), 'revenue');
    expect(inputs).toEqual([
      expect.objectContaining({ kind: 'control', source: 'region', field: 'orders.region', value: 'is EU' }),
      expect.objectContaining({ kind: 'crossFilter', source: 'orders', value: 'no selection' }),
    ]);
    expect(inputs.filter(isActive).map((input) => input.source)).toEqual(['region']);
  });

  it("reads the filters a query's own AQL declares", () => {
    expect(declaredFilters(ORDERS_AQL)).toEqual([expect.objectContaining({ kind: 'declared', field: 'orders.status', value: "!= 'void'", type: 'text' })]);
    expect(declaredFilters(REVENUE_AQL)).toEqual([]);
  });
});

describe('changes', () => {
  it('lists the controls set away from what the declaration starts with', () => {
    expect(changesOf([salesApp()])).toEqual([{ app: 0, name: 'region', kind: 'filter', from: 'no value', to: 'is EU' }]);
  });
});

describe('issues', () => {
  it('lists failed queries, then wiring warnings and markup problems', () => {
    const issues = issuesOf(salesSnapshot());
    expect(issues.map((issue) => issue.level)).toEqual(['error', 'warn', 'warn']);
    expect(issues[0]).toMatchObject({ message: 'orders failed: Unknown field orders.status', target: { entity: 'query', app: 0, name: 'orders' } });
    expect(issues[2]).toMatchObject({ target: { node: '1' } });
  });

  it('counts queries by state, and what is dirty', () => {
    expect(queryCounts([salesApp()])).toMatchObject({ success: 1, error: 1, total: 2, dirty: 0, dirtyControls: 0 });
  });
});

describe('request attribution', () => {
  it("gives each query its requests, newest first, and keeps the rest apart", () => {
    const { byQuery, other } = attributeRequests([entry(1, REVENUE_AQL), entry(2, 'explore sales {}'), entry(3, REVENUE_AQL)], [salesApp()]);
    expect(byQuery.get('0/revenue')?.map((item) => [item.entry.id, item.shared])).toEqual([[3, false], [1, false]]);
    expect(other.map((item) => item.id)).toEqual([2]);
  });

  it('tags a request two queries declare alike as shared', () => {
    const app = salesApp();
    app.declaration.queries.copy = { dataset: 'sales', aql: REVENUE_AQL };
    const { byQuery } = attributeRequests([entry(1, REVENUE_AQL)], [app]);
    expect(byQuery.get('0/copy')?.[0]?.shared).toBe(true);
    expect(byQuery.get('0/revenue')?.[0]?.shared).toBe(true);
  });
});

describe('dedent', () => {
  it('strips the indent of the lines after the first', () => {
    expect(dedent('explore {\n    a\n      b\n  }')).toBe('explore {\n  a\n    b\n}');
  });
});
