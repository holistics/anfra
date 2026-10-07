import { describe, expect, it } from 'vitest';
import {
  filterEntries, findApp, folderPaths, pathFromLocation, urlFor, type CatalogEntry,
} from './catalog';

const tree: CatalogEntry[] = [
  { kind: 'app', path: 'Home.HTML', label: 'Home' },
  {
    kind: 'folder',
    path: 'sales',
    name: 'sales',
    children: [
      { kind: 'app', path: 'sales/overview.html', label: 'Sales Overview' },
      { kind: 'folder', path: 'sales/deep', name: 'deep', children: [{ kind: 'app', path: 'sales/deep/q 3.html', label: 'Q3' }] },
    ],
  },
];

describe('the Data App URL', () => {
  it('names a Data App by its path, without .html', () => {
    expect(urlFor('sales/overview.html')).toBe('/apps/sales/overview');
    expect(urlFor('sales/deep/q 3.html')).toBe('/apps/sales/deep/q%203');
  });

  it('reads back the path it names, and nothing outside /apps/', () => {
    expect(pathFromLocation('/apps/sales/deep/q%203')).toBe('sales/deep/q 3');
    expect(pathFromLocation('/apps/sales/overview/')).toBe('sales/overview');
    expect(pathFromLocation('/apps/')).toBeUndefined();
    expect(pathFromLocation('/')).toBeUndefined();
  });

  it('finds the Data App it names, whatever the case of its .html', () => {
    expect(findApp(tree, 'Home')?.label).toBe('Home');
    expect(findApp(tree, 'sales/deep/q 3')?.label).toBe('Q3');
    expect(findApp(tree, 'sales')).toBeUndefined();
  });
});

describe('filterEntries', () => {
  it('keeps the apps matching by label or path, and the folders leading to them', () => {
    expect(filterEntries(tree, 'q3')).toEqual([
      { ...tree[1], children: [{ ...(tree[1] as { children: CatalogEntry[] }).children[1] }] },
    ]);
    expect(filterEntries(tree, 'OVERVIEW').map((e) => e.path)).toEqual(['sales']);
    expect(filterEntries(tree, '  ')).toBe(tree);
    expect(filterEntries(tree, 'nothing')).toEqual([]);
  });
});

it('lists the folders leading to a Data App', () => {
  expect(folderPaths('sales/deep/x.html')).toEqual(['sales', 'sales/deep']);
  expect(folderPaths('x.html')).toEqual([]);
});
