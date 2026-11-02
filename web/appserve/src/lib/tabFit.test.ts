import { describe, expect, it } from 'vitest';
import { fitTabs } from './tabFit';

const tabs = ['overview', 'data', 'queries', 'structure', 'declaration'].map((id) => ({ id, width: 80 }));

describe('fitTabs', () => {
  it('shows every tab when they all fit', () => {
    expect(fitTabs(tabs, 500, 2, 30)).toEqual({ shown: tabs.map((tab) => tab.id), more: [] });
  });

  it('keeps room for More tabs, and moves what doesn\'t fit behind it, in order', () => {
    expect(fitTabs(tabs, 300, 2, 30)).toEqual({ shown: ['overview', 'data', 'queries'], more: ['structure', 'declaration'] });
  });

  it('always shows the selected tab, in its place, pushing the last out', () => {
    expect(fitTabs(tabs, 300, 2, 30, 'declaration')).toEqual({ shown: ['overview', 'data', 'declaration'], more: ['queries', 'structure'] });
  });

  it('shows the selected tab alone when nothing else fits', () => {
    expect(fitTabs(tabs, 100, 2, 30, 'queries')).toEqual({ shown: ['queries'], more: ['overview', 'data', 'structure', 'declaration'] });
  });
});
