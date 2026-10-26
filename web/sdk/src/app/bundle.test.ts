// @vitest-environment node
/// <reference types="node" />
// Node, not jsdom: esbuild can't run under jsdom. The frame script is loaded into a fresh `vm`
// context instead, with a stand-in for the frame's window.
import {
  afterEach, beforeAll, describe, expect, it, vi,
} from 'vitest';
import * as path from 'node:path';
import * as vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { build } from 'esbuild';
import { PROVISION_ELEMENT_ID, type FrameMessage, type Provision } from '../common/bridge';
import { composeFrameScript } from '../host/provision';
import { salesDataset, testUser } from './testSupport';

// The frame script is what a host injects into a Data App's frame: the kernel, then each plugin's
// setup. Build them the way `pnpm build` does (tsup.config.ts) and run them as a frame would, so
// this checks the artifacts, not the source.
const here = path.dirname(fileURLToPath(import.meta.url));
let kernel: string;
let lineage: string;
let code: string;

async function script (entry: string, globalName: string): Promise<string> {
  const result = await build({
    entryPoints: [path.join(here, entry)], bundle: true, format: 'iife', globalName, target: 'es2020', write: false, keepNames: true,
  });
  return result.outputFiles[0].text;
}

beforeAll(async () => {
  kernel = await script('frame.ts', '__anfraKernel');
  lineage = await script('../plugins/lineage/frame.ts', '__anfraPlugin');
  code = composeFrameScript(kernel, [{ name: 'lineage', setup: lineage }]);
}, 60_000);

afterEach(() => {
  vi.useRealTimers();
});

describe('the frame script', () => {
  it('provisions the SDK from its host, runs the plugins\' setups, installs `Anfra`, and says it is ready', () => {
    vi.useFakeTimers();
    const provision: Provision = {
      datasets: { sales: salesDataset }, user: testUser, plugins: [{ name: 'core', version: '1' }, { name: 'lineage', version: '1' }], hostOrigin: 'http://127.0.0.1:7878',
    };
    const posted: [FrameMessage, string][] = [];
    const parent = { postMessage: (message: FrameMessage, origin: string) => posted.push([message, origin]) };
    let onMessage: ((event: { source: unknown, data: unknown }) => void) | undefined;
    const frame: Record<string, unknown> = {
      parent,
      setTimeout,
      addEventListener: (_type: string, listener: typeof onMessage) => { onMessage = listener; },
      document: {
        getElementById: (id: string) => (id === PROVISION_ELEMENT_ID ? { textContent: JSON.stringify(provision) } : null),
        querySelectorAll: () => [],
        documentElement: {},
      },
    };
    frame.window = frame;
    frame.console = console;

    vm.runInNewContext(code, frame);

    const anfra = frame.Anfra as { createApp: unknown, datasets: Record<string, unknown>, plugins: { has: (name: string) => boolean } };
    expect(typeof anfra?.createApp).toBe('function');
    expect(Object.keys(anfra.datasets)).toEqual(['sales']);
    expect(anfra.plugins.has('lineage')).toBe(true);
    expect(posted).toEqual([[{ type: 'anfra:ready' }, 'http://127.0.0.1:7878']]);
    // Neither the kernel nor a setup is left on the frame's window.
    expect(Object.keys(frame).filter((key) => key.startsWith('__anfra'))).toEqual([]);
    // The panel's events, the kernel's own: watching posts a snapshot at once (apps and structure), and
    // a locate posts a fresh one after. No document is touched until then: nothing is drawn or
    // observed for a frame nobody inspects.
    onMessage!({ source: parent, data: { type: 'anfra:event', name: 'anfra.watch', payload: true } });
    expect(posted.at(-1)).toEqual([{ type: 'anfra:event', name: 'anfra.snapshot', payload: { apps: [], structure: { nodes: [], usage: {} } } }, 'http://127.0.0.1:7878']);
    onMessage!({ source: parent, data: { type: 'anfra:event', name: 'anfra.locate', payload: { app: 0, kind: 'query', name: 'nothing' } } });
    vi.advanceTimersByTime(100);
    expect(posted).toHaveLength(3);
    expect(posted.at(-1)?.[0]).toMatchObject({ type: 'anfra:event', name: 'anfra.snapshot' });
  });

  it('carries no API client, nor does a plugin\'s setup: a frame reaches data only over the bridge', () => {
    for (const marker of ['openapi-fetch', '/core.query', '/core.show', 'coreApiBackend', 'fetch(']) {
      expect(code, marker).not.toContain(marker);
    }
  });

  it('carries no renderer: the host sends it on a frame\'s first rendering', () => {
    for (const marker of ['foreignObject', 'html-to-image', 'DCTDecode']) {
      expect(code, marker).not.toContain(marker);
    }
  });
});
