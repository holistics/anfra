import { describe, expect, it } from 'vitest';
import { ICONS } from '@holistics/anfra-design/icons';
import { THEME_TOKENS } from '../../common/theme';
import { COMPONENTS, COMPONENT_STYLES, FALLBACKS, tokensRead } from './components';
import { columnIcon, icon, STYLES } from './view';

describe('the class layer in plugin tabs', () => {
  const theme = new Set(THEME_TOKENS.map((name) => `--anfra-${name}`));

  it('has a fallback for each token the class layer reads that the theme lacks, and none it doesn\'t', () => {
    const read = tokensRead(COMPONENTS);
    expect(read.length).toBeGreaterThan(0);
    expect(read.filter((name) => !FALLBACKS[name] && !theme.has(name))).toEqual([]);
    expect(Object.keys(FALLBACKS).filter((name) => !read.includes(name))).toEqual([]);
  });

  it('falls back only on the theme\'s tokens', () => {
    expect(Object.values(FALLBACKS).flatMap(tokensRead).filter((name) => !theme.has(name))).toEqual([]);
  });

  it('reads each token with its fallback, and is in every tab\'s stylesheet', () => {
    expect(COMPONENT_STYLES).toContain('var(--anfra-color-text-default, var(--anfra-color-fg))');
    expect(tokensRead(COMPONENT_STYLES).filter((name) => FALLBACKS[name])).toEqual([]);
    expect(STYLES.startsWith(COMPONENT_STYLES)).toBe(true);
  });
});

describe('icons', () => {
  it('are lucide\'s, as anfra-design has them', () => {
    expect(icon(document, 'copy').querySelector('path')?.getAttribute('d')).toBe(ICONS.copy);
    expect(icon(document, 'copy').getAttribute('stroke-width')).toBe('2');
  });

  it('give a column its type\'s, Σ for a metric, and a placeholder for a type anfra-design lacks', () => {
    const d = (svg: SVGElement) => svg.querySelector('path')?.getAttribute('d');
    expect(d(columnIcon(document, { isMeasure: false, semantic_type: 'date' }))).toBe(ICONS['type-date']);
    expect(d(columnIcon(document, { isMeasure: true, semantic_type: 'number' }))).toBe(ICONS.metric);
    expect(d(columnIcon(document, { isMeasure: false, semantic_type: 'geo' as 'text' }))).toBe(ICONS['type-unknown']);
  });
});
