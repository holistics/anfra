import { describe, expect, it } from 'vitest';
import { DEFAULT_TOKENS, resolveTheme } from './theme';

describe('resolveTheme', () => {
  it('fills the scheme\'s defaults under the tokens set', () => {
    const theme = resolveTheme({ scheme: 'dark', tokens: { 'color-bg': '#000' } });
    expect(theme.scheme).toBe('dark');
    expect(theme.tokens['color-bg']).toBe('#000');
    expect(theme.tokens['color-fg']).toBe(DEFAULT_TOKENS.dark['color-fg']);
  });

  it('is light, with defaults, for anything that isn\'t a theme', () => {
    for (const input of [undefined, null, 'dark', { scheme: 'blue' }]) {
      expect(resolveTheme(input)).toEqual({ scheme: 'light', tokens: DEFAULT_TOKENS.light });
    }
  });

  it('keeps the default for a value that could leave its declaration or fetch', () => {
    const bad = ['red; background: url(x)', 'url(https://x.test/a.png)', '}', 'expression(alert(1))', '\\61', '', 'x'.repeat(201), 3];
    for (const value of bad) {
      expect(resolveTheme({ scheme: 'light', tokens: { 'color-bg': value } }).tokens['color-bg'], String(value)).toBe(DEFAULT_TOKENS.light['color-bg']);
    }
    expect(resolveTheme({ scheme: 'light', tokens: { font: '"Inter", sans-serif' } }).tokens.font).toBe('"Inter", sans-serif');
  });
});
