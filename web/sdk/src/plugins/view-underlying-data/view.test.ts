import { describe, expect, it, vi } from 'vitest';
import { countText, view } from './view';
import type { Plan } from './model';
import type { BackendQueryResult } from '../../common/types';

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

  it('offers to try again when the first page fails, and the download only once there are rows', async () => {
    const host = document.createElement('div');
    const root = host.attachShadow({ mode: 'open' });
    const plan: Plan = {
      title: 't',
      chips: [],
      columns: [{ name: 'n', label: 'N', isMeasure: true }],
      request: { dataset: 'd', aql: 'explore {}', input: { filters: [], conditions: [], sorts: [], dateDrills: [] } },
    };
    const pages: number[] = [];
    let fail = true;
    const fetchPage = async (page: number): Promise<BackendQueryResult> => {
      pages.push(page);
      if (fail) throw new Error('Timed out.');
      return { columns: [{ name: 'n' }], values: [['1']], meta: { hasMore: false } } as unknown as BackendQueryResult;
    };
    const stop = view(root, { plan, fetchPage, download: async () => undefined });
    const $ = (id: string) => root.querySelector<HTMLElement>(`[data-testid="${id}"]`)!;
    await vi.waitFor(() => expect($('vud-state').textContent).toBe('Timed out.'));
    expect($('vud-more').hidden).toBe(false);
    expect($('vud-more').textContent).toBe('Try again');
    expect($('vud-download').hidden).toBe(true);

    fail = false;
    $('vud-more').click();
    await vi.waitFor(() => expect($('vud-state').hidden).toBe(true));
    expect(pages).toEqual([1, 1]);
    expect($('vud-more').hidden).toBe(true);
    expect($('vud-download').hidden).toBe(false);
    expect(root.querySelectorAll('tbody tr')).toHaveLength(1);
    stop();
  });
});
