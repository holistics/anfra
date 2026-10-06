import { describe, expect, it } from 'vitest';
import { createSdk } from './sdk';
import { installSandbox, SANDBOX_GLOBAL } from './sandbox';
import { salesDataset, stubBackend, testUser } from './testSupport';

describe('installSandbox', () => {
  it('installs the instance as `Anfra`', () => {
    const target: Record<string, unknown> = {};
    const sdk = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend });

    installSandbox(sdk, target);

    expect(SANDBOX_GLOBAL).toBe('Anfra');
    expect(target.Anfra).toBe(sdk);
    expect(target).not.toHaveProperty('Holistics');
  });

  it('exposes the provisioned instance under the sandbox global', () => {
    const target: Record<string, unknown> = {};
    const sdk = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend });

    const uninstall = installSandbox(sdk, target);

    expect(target[SANDBOX_GLOBAL]).toBe(sdk);
    // Author code needs nothing but the global — no provisioning, no imports.
    expect((target[SANDBOX_GLOBAL] as typeof sdk).createApp({}).toJSON().queries).toEqual({});

    uninstall();
    expect(SANDBOX_GLOBAL in target).toBe(false);
  });

  it('restores a previous value rather than clobbering it', () => {
    const target: Record<string, unknown> = { [SANDBOX_GLOBAL]: 'existing' };
    const uninstall = installSandbox(createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend }), target);

    uninstall();

    expect(target[SANDBOX_GLOBAL]).toBe('existing');
  });
});
