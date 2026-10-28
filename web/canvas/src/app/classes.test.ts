import { describe, expect, it } from 'vitest';
import { groups, overrides } from './classes.ts';

const G = groups({ type: [{ name: 'body-sm', value: '0.875rem' }], textColor: [{ name: 'weak', value: '#666' }], iconColor: [{ name: 'weak', value: '#888' }], color: [], background: [{ name: 'hover', value: '#eee' }], borderColor: [], radius: [] } as never);

describe('overrides', () => {
  it('finds the classes that restyle a component, and leaves those that place it', () => {
    expect(overrides('w-48 flex-1 self-center mt-4 bg-red-500 px-6 text-body-sm text-weak rounded-lg shadow md:bg-hover', G))
      .toEqual(['bg-red-500', 'px-6', 'text-body-sm', 'text-weak', 'rounded-lg', 'shadow']);
    expect(overrides('w-full shrink-0', G)).toEqual([]);
  });

  it('takes an icon token as an icon\'s color, not a restyle', () => {
    expect(G.iconColor.test('icon-weak')).toBe(true);
    expect(G.iconColor.test('icon-left')).toBe(false);
    expect(overrides('icon-weak shrink-0', G)).toEqual([]);
  });
});
