// @vitest-environment node
/// <reference types="node" />
// Node, not jsdom: esbuild can't run under jsdom. The frame script is loaded into a fresh `vm`
// context instead, with a stand-in for the frame's window.
import {
  afterAll, beforeAll, describe, expect, it,
} from 'vitest';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import * as vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { build } from 'tsup';
import { PROVISION_ELEMENT_ID, type FrameMessage, type Provision } from '../common/bridge';
import { salesDataset, testUser } from './testSupport';

// The frame script is what a host injects into a Data App's frame. Build it the way `pnpm build`
// does and run it as a frame would, so this checks the artifact, not the source.
const here = path.dirname(fileURLToPath(import.meta.url));
let outDir: string;
let code: string;

beforeAll(async () => {
  outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'anfra-sdk-frame-'));
  await build({
    entry: { frame: path.join(here, 'frame.ts') },
    format: ['iife'],
    target: 'es2020',
    outDir,
    silent: true,
    config: false,
  });
  code = fs.readFileSync(path.join(outDir, 'frame.global.js'), 'utf-8');
}, 60_000);

afterAll(() => {
  fs.rmSync(outDir, { recursive: true, force: true });
});

describe('the frame script', () => {
  it('provisions the SDK from its host, installs `Anfra`, and says it is ready', () => {
    const provision: Provision = { datasets: { sales: salesDataset }, user: testUser, hostOrigin: 'http://127.0.0.1:7878' };
    const posted: [FrameMessage, string][] = [];
    const parent = { postMessage: (message: FrameMessage, origin: string) => posted.push([message, origin]) };
    const frame: Record<string, unknown> = {
      parent,
      setTimeout,
      addEventListener: () => {},
      document: {
        getElementById: (id: string) => (id === PROVISION_ELEMENT_ID ? { textContent: JSON.stringify(provision) } : null),
      },
    };
    frame.window = frame;

    vm.runInNewContext(code, frame);

    const anfra = frame.Anfra as { createApp: unknown, datasets: Record<string, unknown>, user: unknown };
    expect(typeof anfra?.createApp).toBe('function');
    expect(Object.keys(anfra.datasets)).toEqual(['sales']);
    expect(posted).toEqual([[{ type: 'anfra:ready' }, 'http://127.0.0.1:7878']]);
  });

  it('carries no API client: a frame reaches data only over the bridge', () => {
    for (const marker of ['openapi-fetch', '/core.query', '/core.show', 'coreApiBackend', 'fetch(']) {
      expect(code, marker).not.toContain(marker);
    }
  });
});
