import { afterEach, describe, expect, it, vi } from 'vitest';
import { createSdk } from './sdk';
import { salesDataset, stubBackend, testUser } from './testSupport';

const sdkWith = (plugins: string[], features?: { dateDrill?: boolean }) => createSdk({
  datasets: { sales: salesDataset },
  user: testUser,
  backend: stubBackend().backend,
  plugins: plugins.map((name) => ({ name, version: '0.0.0' })),
  ...(features ? { features } : {}),
});

describe('Anfra.features (deprecated)', () => {
  afterEach(() => { vi.restoreAllMocks(); });

  it('says which of the optional plugins the host installed', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    expect(sdkWith(['core', 'export', 'lineage']).features).toEqual({ export: true, render: false, lineage: true });
    expect(sdkWith(['core']).features).toEqual({ export: false, render: false, lineage: false });
  });

  it('keeps the kernel gates the environment gave', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    expect(sdkWith(['core', 'render'], { dateDrill: false }).features).toEqual({
      dateDrill: false, export: false, render: true, lineage: false,
    });
  });

  it('warns once, on the first read, naming Anfra.plugins.has', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const sdk = sdkWith(['core', 'export']);
    expect(warn).not.toHaveBeenCalled();

    expect(sdk.features.export).toBe(true);
    expect(sdk.features.render).toBe(false);

    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0][0]).toContain("Anfra.plugins.has('export')");
  });

  it('is read-only', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const sdk = sdkWith(['core']);
    expect(Object.isFrozen(sdk.features)).toBe(true);
    expect(() => { (sdk as unknown as { features: unknown }).features = {}; }).toThrow();
  });
});
