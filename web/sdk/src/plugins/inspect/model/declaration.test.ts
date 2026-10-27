import { describe, expect, it } from 'vitest';
import type { InspectedApp } from '../../../common/types';
import { declarationCode, dedent } from './declaration';

function app (declaration: Partial<InspectedApp['declaration']>): InspectedApp {
  return { declaration: { title: 'T', queries: {}, filters: {}, dateDrills: {}, mappings: [], ...declaration } } as InspectedApp;
}

describe('declarationCode', () => {
  it('is empty with nothing declared', () => {
    expect(declarationCode(app({}))).toBe('');
  });

  it('writes queries, controls, mappings and cross-filters, in that order', () => {
    expect(declarationCode(app({
      queries: {
        'top-products': { dataset: 'sales', aql: 'explore sales {\n      measures { revenue }\n    }', pageSize: 10 },
        trend: { dataset: 'sales', aql: 'explore sales {}' },
      },
      filters: { region: { field: 'orders.region' } as never },
      dateDrills: { grain: { default: 'month' } },
      mappings: [
        { kind: 'crossFilter', id: 'x', from: { kind: 'query', name: 'top-products' }, to: { kind: 'query', name: 'trend' } },
        { kind: 'control', id: 'm', from: { kind: 'filter', name: 'region' }, to: { kind: 'query', name: 'trend' }, field: 'orders.region', aggregation: 'sum' } as never,
      ],
    }))).toBe([
      'const topProducts = app.createQuery("top-products", {',
      '  dataset: "sales",',
      '  pageSize: 10,',
      '  aql: `',
      '    explore sales {',
      '      measures { revenue }',
      '    }',
      '  `,',
      '});',
      'const trend = app.createQuery("trend", {',
      '  dataset: "sales",',
      '  aql: `',
      '    explore sales {}',
      '  `,',
      '});',
      '',
      'const region = app.createFilter("region", {"field":"orders.region"});',
      'const grain = app.createDateDrill("grain", {"default":"month"});',
      '',
      'app.mapControl(region, trend, {"field":"orders.region","aggregation":"sum"});',
      '',
      'app.mapCrossFilter(topProducts, trend);',
    ].join('\n'));
  });

  it('makes variable names unique and legal', () => {
    const code = declarationCode(app({
      queries: { 'a-b': { dataset: 's', aql: 'x' }, aB: { dataset: 's', aql: 'y' }, class: { dataset: 's', aql: 'z' } },
    }));
    expect(code).toContain('const aB = app.createQuery("a-b"');
    expect(code).toContain('const aB2 = app.createQuery("aB"');
    expect(code).toContain('const _class = app.createQuery("class"');
  });

  it('escapes what would end or break the template literal', () => {
    expect(declarationCode(app({ queries: { q: { dataset: 's', aql: 'a `b` ${c} \\d' } } }))).toContain('    a \\`b\\` \\${c} \\\\d');
  });
});

describe('dedent', () => {
  it('strips the indent of the lines after the first', () => {
    expect(dedent('explore {\n    a\n      b\n  }')).toBe('explore {\n  a\n    b\n}');
  });
});
