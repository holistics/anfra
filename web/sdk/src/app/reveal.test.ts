import {
  afterEach, describe, expect, it,
} from 'vitest';
import { flush, testFrame } from './testFrame';
import { forbidden } from '../common/errors';
import { createSdk } from './sdk';
import { salesDataset, stubBackend, testUser } from './testSupport';

/** The frame's user activation: whether the reader just clicked. */
function activation (isActive: boolean) {
  Object.defineProperty(window.navigator, 'userActivation', { value: { isActive }, configurable: true });
}

afterEach(() => {
  delete (window.navigator as { userActivation?: unknown }).userActivation;
});

function frame (answer: (request: unknown) => unknown = () => true) {
  const f = testFrame({ answers: { 'anfra.reveal': answer } });
  const app = f.anfra.createApp();
  const query = app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
  const control = app.createFilter('region', { dataset: 'sales', field: 'users.region' });
  return { f, query, control };
}

const reveals = (f: ReturnType<typeof testFrame>) => f.calls.filter((call) => call.name === 'anfra.reveal').map((call) => call.request);
const errors = (f: ReturnType<typeof testFrame>) => f.events.filter((event) => event.name === 'anfra.error').map((event) => event.payload);

describe('inspect()', () => {
  it('asks the host to reveal a query or a control, with the tab if named, and says whether the panel opened', async () => {
    activation(true);
    const { f, query, control } = frame((request) => (request as { entity: string }).entity === 'query');
    expect(await query.inspect()).toBe(true);
    expect(await control.inspect({ tab: 'inspect.queries' })).toBe(false);
    expect(reveals(f)).toEqual([
      { app: 0, entity: 'query', name: 'revenue' },
      { app: 0, entity: 'control', name: 'region', tab: 'inspect.queries' },
    ]);
  });

  it("needs a reader's click: without one it opens nothing, and reports it", async () => {
    activation(false);
    const { f, query } = frame();
    expect(await query.inspect()).toBe(false);
    await flush();
    expect(reveals(f)).toEqual([]);
    expect(errors(f)).toEqual([expect.objectContaining({ kind: 'call.inspect', entity: 'revenue', message: expect.stringContaining("reader's click") })]);
  });

  it('resolves false, and reports why, when the host refuses', async () => {
    activation(true);
    const { f, query } = frame(() => { throw forbidden('Not here.'); });
    expect(await query.inspect()).toBe(false);
    await flush();
    expect(errors(f)).toEqual([expect.objectContaining({ kind: 'call.inspect', message: expect.stringContaining('Not here.') })]);
  });

  it('resolves false outside a frame', async () => {
    activation(true);
    const sdk = createSdk({ backend: stubBackend({}).backend, datasets: { sales: salesDataset }, user: testUser });
    const query = sdk.createApp().createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
    expect(await query.inspect()).toBe(false);
  });
});
