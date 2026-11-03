import { describe, expect, it } from 'vitest';
import { countText, view } from './view';
import type { Plan } from './model';

describe('countText', () => {
  it('says whether the table holds every row, or the first ones so far', () => {
    expect(countText(100, true)).toBe(`Showing the first ${(100).toLocaleString()} rows`);
    expect(countText(11, false)).toBe('All 11 rows');
    expect(countText(1, false)).toBe('The only row');
    expect(countText(0, false)).toBe('');
  });
});

describe('view', () => {
  it('keeps what it hides hidden, even a button the class layer gives its own display', () => {
    const host = document.createElement('div');
    const root = host.attachShadow({ mode: 'open' });
    const plan: Plan = {
      title: 't', chips: [], columns: [], request: { dataset: 'd', aql: 'explore {}', input: { filters: [], conditions: [], sorts: [], dateDrills: [] } },
    };
    const stop = view(root, { plan, fetchPage: () => new Promise(() => {}) });
    expect(root.querySelector('style')!.textContent).toMatch(/\[hidden\]\s*\{\s*display:\s*none\s*!important/);
    stop();
  });
});
