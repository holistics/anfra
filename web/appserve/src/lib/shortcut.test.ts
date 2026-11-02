import { describe, expect, it } from 'vitest';
import { keyLabels } from './shortcut';

describe('keyLabels', () => {
  it('draws a Mac\'s modifiers as its symbols', () => {
    expect(keyLabels('Alt+Shift+I', true)).toEqual(['⌥', '⇧', 'I']);
    expect(keyLabels('Meta+K', true)).toEqual(['⌘', 'K']);
  });

  it('names them elsewhere', () => {
    expect(keyLabels('Alt+Shift+I', false)).toEqual(['Alt', 'Shift', 'I']);
    expect(keyLabels('Ctrl+k', false)).toEqual(['Ctrl', 'K']);
  });

  it('takes a combination in any case, with spaces, as the SDK does', () => {
    expect(keyLabels('ctrl + shift + k', true)).toEqual(['⌃', '⇧', 'K']);
    expect(keyLabels('ALT+f11', false)).toEqual(['Alt', 'F11']);
  });

  it('draws named keys as their symbols', () => {
    expect(keyLabels('Escape', false)).toEqual(['Esc']);
    expect(keyLabels('Shift+ArrowUp', true)).toEqual(['⇧', '↑']);
  });
});
