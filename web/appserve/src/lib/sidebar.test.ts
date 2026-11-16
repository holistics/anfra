import { afterEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import {
  clampWidth, DEFAULT_WIDTH, isSidebarKey, keptOpen, keptWidth, MAX_WIDTH, MIN_WIDTH, readWidth,
} from './sidebar';

afterEach(() => window.localStorage.clear());

describe('the sidebar width', () => {
  it('keeps the sidebar and the viewer usable', () => {
    expect(clampWidth(100)).toBe(MIN_WIDTH);
    expect(clampWidth(900)).toBe(MAX_WIDTH);
    expect(clampWidth(300.4)).toBe(300);
  });

  it('reads a kept width within bounds, and anything else as the default', () => {
    expect(readWidth('320')).toBe(320);
    expect(readWidth('9000')).toBe(MAX_WIDTH);
    expect(readWidth('wide')).toBe(DEFAULT_WIDTH);
    expect(readWidth(null)).toBe(DEFAULT_WIDTH);
  });

  it('is kept on this device, and nothing is written until it changes', async () => {
    expect(keptWidth.value).toBe(DEFAULT_WIDTH);
    expect(window.localStorage.getItem('anfra-appserve:sidebar-width')).toBeNull();
    keptWidth.value = 320;
    await nextTick();
    expect(window.localStorage.getItem('anfra-appserve:sidebar-width')).toBe('320');
    keptWidth.value = DEFAULT_WIDTH;
  });
});

describe('whether the sidebar is open', () => {
  it('stays as the reader left it, open at first', async () => {
    expect(keptOpen.value).toBe(true);
    keptOpen.value = false;
    await nextTick();
    expect(window.localStorage.getItem('anfra-appserve:sidebar-open')).toBe('false');
    keptOpen.value = true;
  });
});

describe('with the site storage blocked', () => {
  it('starts with the defaults, and holds changes for the page', async () => {
    const own = Object.getOwnPropertyDescriptor(window, 'localStorage')!;
    Object.defineProperty(window, 'localStorage', { configurable: true, get: () => { throw new DOMException('denied', 'SecurityError'); } });
    try {
      vi.resetModules();
      const fresh = await import('./sidebar');
      expect(fresh.keptWidth.value).toBe(DEFAULT_WIDTH);
      expect(fresh.keptOpen.value).toBe(true);
      fresh.keptWidth.value = 300;
      await nextTick();
      expect(fresh.keptWidth.value).toBe(300);
    } finally {
      Object.defineProperty(window, 'localStorage', own);
    }
  });
});

describe('the sidebar shortcut', () => {
  const key = (init: KeyboardEventInit) => new KeyboardEvent('keydown', { key: '\\', code: 'Backslash', ...init });

  it('is Cmd+\\ or Ctrl+\\, nothing else held', () => {
    expect(isSidebarKey(key({ metaKey: true }))).toBe(true);
    expect(isSidebarKey(key({ ctrlKey: true }))).toBe(true);
    expect(isSidebarKey(key({}))).toBe(false);
    expect(isSidebarKey(key({ ctrlKey: true, shiftKey: true }))).toBe(false);
    expect(isSidebarKey(new KeyboardEvent('keydown', { key: 'b', code: 'KeyB', ctrlKey: true }))).toBe(false);
  });

  it("doesn't toggle again and again while held", () => {
    expect(isSidebarKey(key({ ctrlKey: true, repeat: true }))).toBe(false);
  });

  it('is reached on a layout that types \\ elsewhere, by the key where US keyboards have it', () => {
    // German: the key left of Enter types # but is the Backslash key.
    expect(isSidebarKey(new KeyboardEvent('keydown', { key: '#', code: 'Backslash', ctrlKey: true }))).toBe(true);
    // AltGr+ß types \ there, which Windows reports as Ctrl+Alt: that is typing, not the shortcut.
    expect(isSidebarKey(new KeyboardEvent('keydown', { key: '\\', code: 'Minus', ctrlKey: true, altKey: true }))).toBe(false);
  });
});
