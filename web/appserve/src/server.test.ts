import {
  beforeEach, describe, expect, it, vi,
} from 'vitest';

// The core API as a test answers it: each op's body, or a status to fail with.
const answers = new Map<string, { status: number, body: unknown }>();
const answer = (op: string, body: unknown, status = 200) => answers.set(op, { status, body });

vi.mock('anfra-sdk/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('anfra-sdk/api')>();
  const fetchImpl = async (request: Request) => {
    const op = new URL(request.url).pathname.replace('/api/', '');
    const body = await request.clone().json() as { fqn?: string };
    const found = answers.get(body.fqn ? `${op} ${body.fqn}` : op);
    if (!found) return new Response('{}', { status: 404 });
    return new Response(JSON.stringify(found.body), { status: found.status, headers: { 'Content-Type': 'application/json' } });
  };
  return { ...actual, coreClient: (base: string) => actual.coreClient(`http://appserve.test${base}`, fetchImpl as typeof fetch) };
});

const server = await import('./server');

beforeEach(() => answers.clear());

describe('problems', () => {
  it("are the files that don't compile, and the validators' errors", async () => {
    answer('core.validate', {
      valid: false,
      diagnostics: [{ filePath: 'a.aml', row: 3, col: 7, message: 'Unexpected }' }],
      reports: [
        { severity: 'error', filePath: 'b.aml', message: 'Unknown model' },
        { severity: 'warning', filePath: 'c.aml', message: 'Unused' },
      ],
    });
    expect(await server.problems()).toEqual([
      { file: 'a.aml', line: 3, column: 7, message: 'Unexpected }' },
      { file: 'b.aml', message: 'Unknown model' },
    ]);
  });
});

describe('datasets', () => {
  it("are the ones that load, with the problems of the ones that don't", async () => {
    answer('core.show', {
      object: {
        kind: 'repo',
        datasets: [
          { kind: 'dataset', fqn: 'shop.sales', name: 'sales', models: [], metrics: [] },
          { kind: 'dataset', fqn: 'shop.broken', name: 'broken' },
        ],
      },
      diagnostics: [
        { filePath: 'a.aml', message: 'Unexpected }' },
        { message: 'Data source "pg" is not configured.' },
        { message: 'Dataset "shop.broken" can\'t be shown in full: boom' },
      ],
    });

    const loaded = await server.datasets();
    expect(Object.keys(loaded.datasets)).toEqual(['shop.sales']);
    // The files that don't compile are left to problems().
    expect(loaded.problems).toEqual([
      { message: 'Data source "pg" is not configured.' },
      { message: 'Dataset "shop.broken" can\'t be shown in full: boom' },
    ]);
  });
});

describe('health', () => {
  it('is up, down, or unreachable', async () => {
    answer('core.status', { state: 'healthy' });
    expect(await server.health()).toBe('up');
    answer('core.status', { state: 'degraded' });
    expect(await server.health()).toBe('down');
    answer('core.status', {}, 503);
    expect(await server.health()).toBe('unreachable');
  });
});

describe('version', () => {
  it('is the version serving, with the newer release the server knows of', async () => {
    answer('core.version', { version: '0.4.2', update: { latest: '0.5.0', available: true } });
    expect(await server.version()).toEqual({ current: '0.4.2', latest: '0.5.0' });
  });

  it("has no latest when the server is current or doesn't check", async () => {
    answer('core.version', { version: '0.5.0', update: { latest: '0.5.0', available: false } });
    expect(await server.version()).toEqual({ current: '0.5.0' });
    answer('core.version', { version: '0.5.0' });
    expect(await server.version()).toEqual({ current: '0.5.0' });
  });

  it("is none when the server can't be asked", async () => {
    answer('core.version', {}, 500);
    expect(await server.version()).toBeUndefined();
  });
});
