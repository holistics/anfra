// @vitest-environment node
/// <reference types="node" />
// Node, not jsdom: esbuild can't run under jsdom.
import { afterAll, describe, expect, it } from 'vitest';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import * as vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { buildFrameHalf } from '.';

const here = path.dirname(fileURLToPath(import.meta.url));
const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'anfra-frame-half-'));
afterAll(() => fs.rmSync(dir, { recursive: true, force: true }));

describe('buildFrameHalf', () => {
  it('builds the setup, and the body, to scripts that leave their default export where the kernel looks', async () => {
    const half = await buildFrameHalf({
      setup: path.join(here, '../../examples/notes/frame.ts'),
      body: (fs.writeFileSync(path.join(dir, 'body.ts'), 'export default { draw: 1 };'), path.join(dir, 'body.ts')),
    });
    const setup = vm.runInNewContext(`${half.setup}; __anfraPlugin`) as { default: unknown };
    const body = vm.runInNewContext(`${half.body}; __anfraBody`) as { default: unknown };
    expect(typeof setup.default).toBe('function');
    expect(body.default).toEqual({ draw: 1 });
  });

  it('refuses a frame half that bundles the API client or the host\'s code', async () => {
    for (const specifier of ['@holistics/anfra-sdk/api', '@holistics/anfra-sdk/host', 'openapi-fetch']) {
      const entry = path.join(dir, `bad-${specifier.replace(/\W/g, '-')}.ts`);
      fs.writeFileSync(entry, `import * as x from '${specifier}';\nexport default () => x;`);
      await expect(buildFrameHalf({ setup: entry }), specifier).rejects.toThrow(/can't import/);
    }
  });
});
