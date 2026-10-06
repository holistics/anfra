// @vitest-environment node
/// <reference types="node" />
// Node, not jsdom: esbuild can't run under jsdom. The bundle is loaded into a fresh `vm` context
// instead, which stands in for a page's global scope.
import {
  afterAll, beforeAll, describe, expect, it,
} from 'vitest';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import * as vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { build } from 'tsup';
import { salesDataset, stubBackend, testUser } from './testSupport';

// The IIFE bundle is what a provisioner drops into a Data App page with a script tag. Build it the
// way `pnpm build` does and load it as a page would, so this checks the artifact, not the source.
const here = path.dirname(fileURLToPath(import.meta.url));
let outDir: string;
let code: string;

beforeAll(async () => {
  outDir = fs.mkdtempSync(path.join(os.tmpdir(), 'anfra-sdk-bundle-'));
  await build({
    entry: { 'anfra-sdk': path.join(here, '..', 'index.ts') },
    format: ['iife'],
    globalName: 'AnfraSdk',
    target: 'es2020',
    outDir,
    silent: true,
    config: false,
  });
  code = fs.readFileSync(path.join(outDir, 'anfra-sdk.global.js'), 'utf-8');
}, 60_000);

afterAll(() => {
  fs.rmSync(outDir, { recursive: true, force: true });
});

describe('the IIFE bundle', () => {
  it('exposes what a provisioner needs on `AnfraSdk`, and lets it install `Anfra`', () => {
    // Run as a classic script: its top-level `var AnfraSdk` lands on the page's global object.
    const page: Record<string, unknown> = {};
    vm.runInNewContext(code, page);
    const lib = page.AnfraSdk as typeof import('../index');

    expect(typeof lib.createSdk).toBe('function');
    expect(typeof lib.installSandbox).toBe('function');
    expect(lib.SANDBOX_GLOBAL).toBe('Anfra');
    expect(typeof lib.QueryError).toBe('function');

    const sdk = lib.createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stubBackend().backend });
    const uninstall = lib.installSandbox(sdk, page);

    expect(page.Anfra).toBe(sdk);
    uninstall();
    expect('Anfra' in page).toBe(false);
  });
});
