import { describe, expect, it } from 'vitest';
import type { InspectedApp } from '../../../common/types';
import { mentionsField, wiringChecks } from './checks';

function app (declaration: Partial<InspectedApp['declaration']>): InspectedApp {
  return { declaration: { title: 'T', queries: {}, filters: {}, dateDrills: {}, mappings: [], ...declaration } } as InspectedApp;
}

describe('wiringChecks', () => {
  it('finds nothing wrong with a wired app', () => {
    expect(wiringChecks(app({
      queries: { a: { dataset: 'sales', aql: 'explore sales { dimensions { orders.created_at } }' } },
      dateDrills: { grain: { default: 'month' } },
      mappings: [{ kind: 'control', id: 'm', from: { kind: 'dateDrill', name: 'grain' }, to: { kind: 'query', name: 'a' }, field: 'orders.created_at' }],
    }))).toEqual([]);
  });

  it('puts errors first, then warnings, then info', () => {
    const checks = wiringChecks(app({
      queries: {
        a: { dataset: 'sales', aql: 'explore sales { dimensions { orders.created_at_utc } }' },
        b: { dataset: 'ops', aql: 'explore ops {}' },
        c: { dataset: 'sales', aql: 'explore sales {}' },
      },
      filters: { region: { field: 'orders.region' } as never },
      dateDrills: { grain: { default: 'month' } },
      mappings: [
        { kind: 'control', id: 'm', from: { kind: 'dateDrill', name: 'grain' }, to: { kind: 'query', name: 'a' }, field: 'orders.created_at' },
        { kind: 'crossFilter', id: 'x', from: { kind: 'query', name: 'a' }, to: { kind: 'query', name: 'b' } },
      ],
    }));
    expect(checks).toEqual([
      { level: 'error', message: 'Cross-filter a → b spans datasets (sales → ops).', about: { entity: 'query', name: 'a' } },
      { level: 'warn', message: 'region is mapped to no query, so it has no effect.', about: { entity: 'control', name: 'region' } },
      { level: 'warn', message: 'grain → a: orders.created_at isn\'t in the query\'s AQL, so the grain never changes.', about: { entity: 'control', name: 'grain' } },
      { level: 'info', message: 'c has no controls or cross-filters, so it never changes.', about: { entity: 'query', name: 'c' } },
    ]);
  });
});

describe('mentionsField', () => {
  it('matches a field only as a whole name', () => {
    expect(mentionsField('dimensions { orders.created_at }', 'orders.created_at')).toBe(true);
    expect(mentionsField('dimensions { orders.created_at_utc }', 'orders.created_at')).toBe(false);
    expect(mentionsField('dimensions { x.orders.created_at }', 'orders.created_at')).toBe(false);
  });
});
