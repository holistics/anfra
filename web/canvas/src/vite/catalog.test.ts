import { describe, expect, it } from 'vitest';
import { kitCatalog, palette, readTokens } from './catalog.ts';

describe('kitCatalog', () => {
  const kit = {
    components: [
      { name: 'HButton', from: '@holistics/design-system', props: [{ name: 'type', type: 'enum' as const, options: ['primary', 'secondary'] }], snippet: '<HButton label="Button" />' },
      { name: 'HModal', from: '@holistics/design-system', slot: true, snippet: false as const },
      { name: 'HDivider', from: '@holistics/design-system' },
    ],
  };

  it('holds each component with the props the kit gives it', () => {
    const { catalog } = kitCatalog(kit);
    expect(catalog.map((c) => [c.name, c.folder, c.from, c.main])).toEqual([
      ['HButton', 'hbutton', '@holistics/design-system', true],
      ['HModal', 'hmodal', '@holistics/design-system', true],
      ['HDivider', 'hdivider', '@holistics/design-system', true],
    ]);
    expect(catalog[0].props).toEqual([{ name: 'type', type: 'enum', options: ['primary', 'secondary'] }]);
    expect(catalog[2].props).toEqual([]);
  });

  it('places each with its snippet, imported from its package, and leaves out the ones set to false', () => {
    const { catalog, snippets } = kitCatalog(kit);
    expect(palette(catalog, snippets)).toEqual([
      { id: 'hbutton', name: 'HButton', markup: '<HButton label="Button" />', imports: [{ name: 'HButton', from: '@holistics/design-system' }] },
      { id: 'hdivider', name: 'HDivider', markup: '<HDivider />', imports: [{ name: 'HDivider', from: '@holistics/design-system' }] },
    ]);
  });
  it('marks a kit\'s components as instances, with the placement each takes', () => {
    const { catalog } = kitCatalog({ components: [
      { name: 'HButton', from: 'ds' },
      { name: 'HTextInput', from: 'ds', classTarget: 'inner' },
      { name: 'HModal', from: 'ds', classTarget: 'none', placement: ['width'] },
      { name: 'HTooltip', from: 'ds', placement: [] },
    ] });
    expect(catalog.map((c) => [c.kit, c.classTarget, c.placement])).toEqual([
      [true, 'root', ['width', 'grow', 'align', 'margin']],
      [true, 'inner', ['width', 'grow', 'align', 'margin']],
      [true, 'none', []],
      [true, 'root', []],
    ]);
  });

  it('keeps the semantic color a component takes', () => {
    const { catalog } = kitCatalog({ components: [{ name: 'HIcon', from: 'ds', color: 'icon' }, { name: 'HButton', from: 'ds' }] });
    expect(catalog.map((c) => c.color)).toEqual(['icon', undefined]);
  });
});

describe('readTokens', () => {
  it('reads an icon\'s colors apart from the text\'s', () => {
    const t = readTokens('@theme { --text-color-weak: #666; --icon-color-weak: #888; --icon-color-highlight-default: #1b7ce4; }');
    expect(t.textColor).toEqual([{ name: 'weak', value: '#666' }]);
    expect(t.iconColor).toEqual([{ name: 'weak', value: '#888' }, { name: 'highlight-default', value: '#1b7ce4' }]);
  });
});
