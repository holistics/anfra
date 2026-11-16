import { afterEach, describe, expect, it } from 'vitest';
import { nextTick } from 'vue';
import { FOLDERS_KEY, useFolders } from './folders';

afterEach(() => window.localStorage.clear());

const kept = () => JSON.parse(window.localStorage.getItem(FOLDERS_KEY) ?? '[]') as string[];

describe('the folders', () => {
  it('start open, and keep what the reader collapses', async () => {
    const folders = useFolders();
    expect([...folders.collapsed.value]).toEqual([]);
    folders.toggle('finance');
    await nextTick();
    expect([...folders.collapsed.value]).toEqual(['finance']);
    expect(kept()).toEqual(['finance']);
    folders.toggle('finance');
    await nextTick();
    expect(kept()).toEqual([]);
  });

  it('read what an earlier visit kept', () => {
    window.localStorage.setItem(FOLDERS_KEY, JSON.stringify(['finance', 'sales']));
    expect([...useFolders().collapsed.value]).toEqual(['finance', 'sales']);
  });

  it('show the folders of the open Data App for this visit, without changing what is kept', async () => {
    window.localStorage.setItem(FOLDERS_KEY, JSON.stringify(['finance', 'finance/q3', 'sales']));
    const folders = useFolders();
    folders.reveal(['finance', 'finance/q3']);
    await nextTick();
    expect([...folders.collapsed.value]).toEqual(['sales']);
    expect(kept()).toEqual(['finance', 'finance/q3', 'sales']);
    // The next visit starts as the reader left it.
    expect([...useFolders().collapsed.value]).toEqual(['finance', 'finance/q3', 'sales']);
  });

  it('collapse a shown folder on the reader\'s toggle, and keep it collapsed', async () => {
    window.localStorage.setItem(FOLDERS_KEY, JSON.stringify(['finance']));
    const folders = useFolders();
    folders.reveal(['finance']);
    folders.toggle('finance');
    await nextTick();
    expect([...folders.collapsed.value]).toEqual(['finance']);
    expect(kept()).toEqual(['finance']);
    folders.toggle('finance');
    await nextTick();
    expect([...folders.collapsed.value]).toEqual([]);
    expect(kept()).toEqual([]);
  });
});
