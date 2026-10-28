import { describe, expect, it } from 'vitest';
import { kitCatalog, palette } from './catalog.ts';

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
});
