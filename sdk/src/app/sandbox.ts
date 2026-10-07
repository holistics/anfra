import type { Sdk } from './sdk';

export const SANDBOX_GLOBAL = 'Anfra';

/**
 * Installs a provisioned SDK as the sandbox's global.
 *
 * Named `Anfra` rather than `SDK` because a Data App may run inside someone else's page, where a
 * global called `SDK` will collide with something. Returns an uninstall so a host rendering more
 * than one app in sequence — or a test — can put the previous value back.
 *
 * Hosts that have a module boundary should import the instance instead; the global exists for a
 * Data App page, where author code is a plain `<script>`.
 */
export function installSandbox (sdk: Sdk, target: Record<string, unknown> = globalThis as never): () => void {
  const previous = target[SANDBOX_GLOBAL];
  const existed = SANDBOX_GLOBAL in target;

  target[SANDBOX_GLOBAL] = sdk;

  return () => {
    if (existed) {
      target[SANDBOX_GLOBAL] = previous;
    } else {
      delete target[SANDBOX_GLOBAL];
    }
  };
}
