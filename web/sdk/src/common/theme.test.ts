import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import {
  DEFAULT_TOKENS, DESIGN_TOKENS, THEME_TOKENS, designTheme, resolveTheme,
} from './theme';

/** Each custom property `@holistics/anfra-design` sets, resolved through its var()s, per scheme. */
function designValues (): Record<'light' | 'dark', Map<string, string>> {
  const files = ['tokens.css', 'fonts.css', 'type.css']
    .map((file) => readFileSync(resolve(__dirname, '../../../design', file), 'utf8'))
    .join('\n')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/@import[^;]+;/g, '');
  const light = new Map<string, string>();
  const dark = new Map<string, string>();
  for (const [, selector, body] of files.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
    const into = selector!.includes('dark') ? dark : light;
    for (const [, name, value] of body!.matchAll(/(--[\w-]+)\s*:\s*([^;]+);/g)) into.set(name!, value!.trim().replace(/\s+/g, ' '));
  }
  const resolved = (scheme: Map<string, string>) => {
    const read = (name: string): string => (scheme.get(name) ?? light.get(name) ?? '')
      .replace(/var\(\s*(--[\w-]+)\s*\)/g, (_, inner: string) => read(inner))
      .replace(/\(\s+/g, '(').replace(/\s+\)/g, ')');
    return new Map([...light.keys()].map((name) => [name, read(name)]));
  };
  return { light: resolved(light), dark: resolved(new Map([...light, ...dark])) };
}

describe('DEFAULT_TOKENS', () => {
  it('are anfra-design\'s tokens, resolved, in each scheme', () => {
    const design = designValues();
    for (const scheme of ['light', 'dark'] as const) {
      for (const name of THEME_TOKENS) {
        expect(DEFAULT_TOKENS[scheme][name], `${scheme} ${name}`).toBe(design[scheme].get(DESIGN_TOKENS[name]));
      }
    }
  });
});

describe('designTheme', () => {
  it('reads each token from its anfra-design property, leaving out what the page lacks', () => {
    const page: Record<string, string> = { '--anfra-color-background-default': ' oklch(1 0 0) ', '--anfra-color-text-highlight-default': 'violet' };
    const theme = designTheme('dark', { getPropertyValue: (name: string) => page[name] ?? '' });
    expect(theme).toEqual({ scheme: 'dark', tokens: { 'color-bg': 'oklch(1 0 0)', 'color-accent': 'violet' } });
  });
});

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
