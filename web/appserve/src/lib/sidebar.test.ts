import { describe, expect, it } from 'vitest';
import {
  clampWidth, DEFAULT_WIDTH, isSidebarKey, MAX_WIDTH, MIN_WIDTH, rememberOpen, rememberWidth, storedOpen, storedWidth,
} from './sidebar';

function memory (): Storage {
  const items = new Map<string, string>();
  return {
    get length () { return items.size; },
    clear: () => items.clear(),
    getItem: (key) => items.get(key) ?? null,
    key: (i) => [...items.keys()][i] ?? null,
    removeItem: (key) => { items.delete(key); },
    setItem: (key, value) => { items.set(key, value); },
  };
}

const blocked = { getItem: () => { throw new Error('denied'); }, setItem: () => { throw new Error('denied'); } } as unknown as Storage;

describe('the sidebar width', () => {
  it('keeps the sidebar and the viewer usable', () => {
    expect(clampWidth(100)).toBe(MIN_WIDTH);
    expect(clampWidth(900)).toBe(MAX_WIDTH);
    expect(clampWidth(300.4)).toBe(300);
  });

  it('is the one last dragged to, or the default', () => {
    const storage = memory();
    expect(storedWidth(storage)).toBe(DEFAULT_WIDTH);
    rememberWidth(320, storage);
    expect(storedWidth(storage)).toBe(320);
    storage.setItem('anfra-appserve:sidebar-width', '9000');
    expect(storedWidth(storage)).toBe(MAX_WIDTH);
    storage.setItem('anfra-appserve:sidebar-width', 'wide');
    expect(storedWidth(storage)).toBe(DEFAULT_WIDTH);
  });

  it('falls back to the default without storage', () => {
    expect(storedWidth(blocked)).toBe(DEFAULT_WIDTH);
    expect(() => rememberWidth(300, blocked)).not.toThrow();
  });

  it('starts even when reaching localStorage throws, as with site data blocked', () => {
    const own = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
    Object.defineProperty(globalThis, 'localStorage', { configurable: true, get: () => { throw new DOMException('denied', 'SecurityError'); } });
    try {
      expect(storedWidth()).toBe(DEFAULT_WIDTH);
      expect(storedOpen()).toBe(true);
      expect(() => { rememberWidth(300); rememberOpen(false); }).not.toThrow();
    } finally {
      if (own) Object.defineProperty(globalThis, 'localStorage', own);
    }
  });
});

describe('whether the sidebar is open', () => {
  it('stays as the reader left it, open at first', () => {
    const storage = memory();
    expect(storedOpen(storage)).toBe(true);
    rememberOpen(false, storage);
    expect(storedOpen(storage)).toBe(false);
    rememberOpen(true, storage);
    expect(storedOpen(storage)).toBe(true);
    expect(storedOpen(blocked)).toBe(true);
  });
});

describe('the sidebar shortcut', () => {
  const key = (init: KeyboardEventInit) => new KeyboardEvent('keydown', { key: '\\', ...init });

  it('is Cmd+\\ or Ctrl+\\, nothing else held', () => {
    expect(isSidebarKey(key({ metaKey: true }))).toBe(true);
    expect(isSidebarKey(key({ ctrlKey: true }))).toBe(true);
    expect(isSidebarKey(key({}))).toBe(false);
    expect(isSidebarKey(key({ ctrlKey: true, shiftKey: true }))).toBe(false);
    expect(isSidebarKey(key({ metaKey: true, altKey: true }))).toBe(false);
    expect(isSidebarKey(new KeyboardEvent('keydown', { key: 'b', ctrlKey: true }))).toBe(false);
  });
});
