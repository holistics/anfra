import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { DataAppError } from '../common/errors';
import type { Backend } from '../common/types';
import type { FrameKernel } from './kernel';
import { EVENTS_PER_MINUTE, installReporting } from './reporting';
import { createSdk } from './sdk';
import { numberTypes, salesDataset, testUser } from './testSupport';

let uninstall: (() => void) | undefined;

afterEach(() => {
  uninstall?.();
  uninstall = undefined;
  vi.restoreAllMocks();
});

/** Install reporting on this window, catching what the kernel would send the host. */
function install (captureConsole = false, now?: () => number) {
  const emit = vi.fn();
  uninstall = installReporting({ emit } as unknown as FrameKernel, window, captureConsole, now);
  return emit;
}

const failing = (err: unknown): Backend => ({
  submitQuery: async () => { throw err; },
  fieldSuggestions: async () => [],
});

const revenueOf = (backend: Backend) => {
  const app = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend }).createApp();
  const query = app.createQuery('revenue', { dataset: 'sales', aql: 'explore { measures { total: orders | sum(orders.amount) } }' });
  return { app, query };
};

describe('reporting', () => {
  it('reports a query that ends in error, with its shape and never its stack', async () => {
    const emit = install();
    const { app } = revenueOf(failing(new DataAppError({
      code: 'query_failed', scope: 'user', message: 'column x does not exist', requestId: 'req-1',
    })));
    await app.execute();
    expect(emit).toHaveBeenCalledWith('error', {
      kind: 'call.query', code: 'query_failed', scope: 'user', message: 'column x does not exist', requestId: 'req-1', entity: 'revenue', at: expect.any(String),
    });
  });

  it('reports a listener\'s exception, with its stack, and keeps it out of the console lines', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    const emit = install(true);
    const { app } = revenueOf({
      submitQuery: async () => ({ values: [['1']], columns: [{ name: 'total', fieldName: 'total', label: 'Total', adhoc: true, isMeasure: true, ...numberTypes }], meta: { page: 1, pageSize: 1000, numRows: 1 } }),
      fieldSuggestions: async () => [],
    });
    app.subscribe(() => { throw new TypeError('echarts is not defined'); });
    await app.execute();
    expect(emit).toHaveBeenCalledWith('error', expect.objectContaining({ kind: 'app.listener', message: 'echarts is not defined', stack: expect.any(String) }));
    expect(emit.mock.calls.filter(([name]) => name === 'console')).toEqual([]);
  });

  it('reports the app\'s uncaught errors and unhandled rejections', () => {
    const emit = install();
    window.dispatchEvent(new ErrorEvent('error', { error: new RangeError('bad index'), message: 'bad index' }));
    const rejection = new Event('unhandledrejection') as PromiseRejectionEvent;
    Object.defineProperty(rejection, 'reason', { value: new DataAppError({ code: 'forbidden', scope: 'user', message: 'No access.', entity: 'revenue' }) });
    window.dispatchEvent(rejection);
    expect(emit.mock.calls).toEqual([
      ['error', expect.objectContaining({ kind: 'app.uncaught', message: 'bad index', stack: expect.any(String) })],
      ['error', expect.objectContaining({ kind: 'app.uncaught', code: 'forbidden', scope: 'user', entity: 'revenue' })],
    ]);
  });

  it('reports a call the app left unhandled by its name, as the app called it', async () => {
    const emit = install();
    const unhandled = (reason: unknown) => {
      const rejection = new Event('unhandledrejection') as PromiseRejectionEvent;
      Object.defineProperty(rejection, 'reason', { value: reason });
      window.dispatchEvent(rejection);
    };
    let calls = 0;
    const backend: Backend = {
      submitQuery: async () => {
        calls += 1;
        if (calls === 1) return { values: [['1']], columns: [{ name: 'total', fieldName: 'total', label: 'Total', adhoc: true, isMeasure: true, ...numberTypes }], meta: { page: 1, pageSize: 10, numRows: 1, hasMore: true } };
        throw new DataAppError({ code: 'connection_failed', scope: 'server', message: 'Cut.' });
      },
      fieldSuggestions: async () => [],
    };
    const app = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend }).createApp();
    const query = app.createQuery('rows', { dataset: 'sales', aql: 'explore { measures { total: orders | sum(orders.amount) } }', pageSize: 10 });
    await app.execute();
    const failed = await query.fetchMore().catch((err: unknown) => err);
    emit.mockClear();
    unhandled(failed);
    expect(emit).toHaveBeenCalledWith('error', expect.objectContaining({ kind: 'call.fetch_more', code: 'connection_failed' }));
    expect(emit.mock.calls[0][1]).not.toHaveProperty('stack');
    expect(emit.mock.calls[0][1]).not.toHaveProperty('call');
  });

  it('sends console lines only when captured, still writing them to the console', () => {
    const written = vi.spyOn(console, 'warn').mockImplementation(() => {});
    let emit = install(false);
    console.warn('not sent');
    expect(emit).not.toHaveBeenCalled();
    uninstall!();

    emit = install(true);
    console.warn('rows', { n: 2 }, new Error('why'));
    console.error('anfra: the SDK\'s own note');
    expect(emit.mock.calls).toEqual([['console', { level: 'warn', message: 'rows {"n":2} Error: why', at: expect.any(String) }]]);
    expect(written).toHaveBeenCalledWith('rows', { n: 2 }, expect.any(Error));
  });

  it('sends at most so many events a minute, and says in the console how many were not', () => {
    let t = 0;
    const warned = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const emit = install(false, () => t);
    for (let i = 0; i < EVENTS_PER_MINUTE + 5; i += 1) window.dispatchEvent(new ErrorEvent('error', { message: 'again' }));
    expect(emit).toHaveBeenCalledTimes(EVENTS_PER_MINUTE);
    t = 60_000;
    window.dispatchEvent(new ErrorEvent('error', { message: 'later' }));
    expect(emit).toHaveBeenCalledTimes(EVENTS_PER_MINUTE + 1);
    expect(warned).toHaveBeenCalledWith(expect.stringMatching(/^anfra: 5 runtime errors over/));
  });

  it('says how many were not sent once the minute is over, even with nothing after', () => {
    vi.useFakeTimers();
    try {
      const warned = vi.spyOn(console, 'warn').mockImplementation(() => {});
      install(false, Date.now);
      for (let i = 0; i < EVENTS_PER_MINUTE + 2; i += 1) window.dispatchEvent(new ErrorEvent('error', { message: 'again' }));
      expect(warned).not.toHaveBeenCalled();
      vi.advanceTimersByTime(60_000);
      expect(warned).toHaveBeenCalledWith(expect.stringMatching(/^anfra: 2 runtime errors over/));
    } finally {
      vi.useRealTimers();
    }
  });

  it('keeps console lines from crowding out errors', () => {
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    const emit = install(true, () => 0);
    for (let i = 0; i < EVENTS_PER_MINUTE * 2; i += 1) console.warn('render');
    window.dispatchEvent(new ErrorEvent('error', { error: new TypeError('real'), message: 'real' }));
    expect(emit.mock.calls.filter(([name]) => name === 'console')).toHaveLength(EVENTS_PER_MINUTE);
    expect(emit).toHaveBeenLastCalledWith('error', expect.objectContaining({ kind: 'app.uncaught', message: 'real' }));
  });
});
