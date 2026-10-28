import { describe, expect, it } from 'vitest';
import type { BoardSummary } from '../vite/boards.ts';
import { boardProject, listed, parseView, people, projects, screenProject, screens, viewHash } from './home.ts';
import type { Settings } from './store.ts';

const board = (slug: string, more: Partial<BoardSummary> = {}): BoardSummary => ({
  slug, name: slug, frames: 1, open: 0, updated: 0, routes: [], screens: [], map: { sections: [], frames: [] }, ...more,
});
const settings = {
  prototypes: ['/p/datasets/List', '/p/datasets/Detail', '/p/reports/Editor', '/p/Loose'],
  prototypeRoute: '/p',
  people: { 'thong.do': { name: 'Thong Do' } },
  me: 'Hung',
} as unknown as Settings;

describe('the home page', () => {
  it('finds a board\'s project: its own, else its screens\' folder', () => {
    expect(screenProject('/p/datasets/List?open=delete', '/p')).toBe('datasets');
    expect(screenProject('/p/Loose', '/p')).toBeUndefined();
    expect(boardProject(board('a', { routes: ['/p/reports/Editor', '/p/datasets/List', '/p/datasets/Detail'] }), '/p')).toBe('datasets');
    expect(boardProject(board('b', { project: 'mine', routes: ['/p/datasets/List'] }), '/p')).toBe('mine');
  });

  it('lists projects with their boards and screens, and everyone', () => {
    const boards = [board('a', { routes: ['/p/datasets/List'], owners: ['thong.do', 'khai.to'] })];
    expect(projects(boards, settings)).toEqual([
      { name: 'datasets', boards: 1, screens: 2 },
      { name: 'reports', boards: 0, screens: 1 },
    ]);
    expect(people(boards, settings).map((p) => p.name)).toEqual(['Thong Do', 'khai.to', 'Hung']);
  });

  it('lists a view\'s boards, and what a search finds', () => {
    const boards = [
      board('old', { updated: 1, owners: ['thong.do'], project: 'datasets' }),
      board('new', { updated: 2, screens: ['Checkout'], routes: ['/p/reports/Editor'] }),
    ];
    expect(listed(boards, settings, { kind: 'all' }, '', 'updated', []).map((b) => b.slug)).toEqual(['new', 'old']);
    expect(listed(boards, settings, { kind: 'person', id: 'thong.do' }, '', 'updated', []).map((b) => b.slug)).toEqual(['old']);
    expect(listed(boards, settings, { kind: 'project', id: 'reports' }, '', 'updated', []).map((b) => b.slug)).toEqual(['new']);
    expect(listed(boards, settings, { kind: 'all' }, 'thong', 'updated', []).map((b) => b.slug)).toEqual(['old']);
    expect(listed(boards, settings, { kind: 'all' }, 'checkout', 'updated', []).map((b) => b.slug)).toEqual(['new']);
    expect(listed(boards, settings, { kind: 'recent' }, '', 'updated', ['old', 'gone']).map((b) => b.slug)).toEqual(['old']);
    expect(screens(settings, { kind: 'project', id: 'datasets' }, '')).toEqual(['/p/datasets/List', '/p/datasets/Detail']);
  });

  it('keeps its view in the address', () => {
    for (const v of [{ kind: 'all' }, { kind: 'recent' }, { kind: 'project', id: 'data sets' }, { kind: 'person', id: 'thong.do' }] as const) {
      expect(parseView(viewHash(v))).toEqual(v);
    }
    expect(parseView('getting-started')).toBeUndefined();
  });
});
