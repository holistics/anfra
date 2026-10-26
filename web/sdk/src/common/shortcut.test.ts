import { describe, expect, it } from 'vitest';
import { eventShortcut, normalizeShortcut } from './shortcut';

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
});
