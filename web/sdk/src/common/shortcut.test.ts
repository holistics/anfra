import { describe, expect, it } from 'vitest';
import { eventShortcut, normalizeShortcut, shortcutEvent } from './shortcut';

describe('shortcuts', () => {
  it('puts a combination in one form, modifiers in a fixed order', () => {
    expect(normalizeShortcut('Shift+Alt+I')).toBe('alt+shift+i');
    expect(normalizeShortcut('Meta+K')).toBe('meta+k');
    expect(normalizeShortcut('Escape')).toBe('escape');
    for (const bad of ['', 'Hyper+K', 'Alt+', 3, undefined]) expect(normalizeShortcut(bad)).toBeUndefined();
  });

  it('reads an event by its physical letter or digit, else by its key', () => {
    expect(eventShortcut(new KeyboardEvent('keydown', { code: 'KeyI', key: 'ˆ', altKey: true, shiftKey: true }))).toBe('alt+shift+i');
    expect(eventShortcut(new KeyboardEvent('keydown', { code: 'Digit1', key: '!', ctrlKey: true }))).toBe('ctrl+1');
    expect(eventShortcut(new KeyboardEvent('keydown', { code: 'Escape', key: 'Escape' }))).toBe('escape');
  });

  it('makes the event a combination names, with the key and code a browser would set', () => {
    expect(shortcutEvent('meta+k')).toEqual({ key: 'k', code: 'KeyK', ctrlKey: false, altKey: false, shiftKey: false, metaKey: true });
    expect(shortcutEvent('ctrl+shift+k')).toMatchObject({ key: 'K', code: 'KeyK', ctrlKey: true, shiftKey: true });
    expect(shortcutEvent('ctrl+1')).toMatchObject({ key: '1', code: 'Digit1' });
    expect(shortcutEvent('shift+escape')).toMatchObject({ key: 'Escape', code: 'Escape', shiftKey: true });
    expect(shortcutEvent('alt+arrowup')).toMatchObject({ key: 'ArrowUp', code: 'ArrowUp' });
    expect(shortcutEvent('f5')).toMatchObject({ key: 'F5', code: 'F5' });
    for (const shortcut of ['alt+shift+i', 'ctrl+1', 'shift+escape', 'meta+arrowup', 'f5']) {
      expect(eventShortcut(new KeyboardEvent('keydown', shortcutEvent(shortcut)))).toBe(shortcut);
    }
  });
});
