import { describe, expect, it } from 'vitest';
import { DataAppError } from './errors';
import {
  callOf, checkErrorEvent, isErrorKind, namedCall, snakeCase, tagCall,
} from './runtimeErrors';

const mount = { id: 1 };

describe('runtime error kinds', () => {
  it('are <source>.<what> under a known source, in snake_case', () => {
    expect(['call.query', 'call.fetch_more', 'app.uncaught', 'plugin.uncaught', 'host.internal', 'call.foo_bar'].every(isErrorKind)).toBe(true);
    expect(['query', 'uncaught', 'other.thing', 'call.', 'call.fetchMore', 'app.uncaught.more', 42].some(isErrorKind)).toBe(false);
  });

  it("take a kind the host does not know under a known source, but never the host's own from a frame", () => {
    expect(checkErrorEvent({ kind: 'call.foo_bar', message: 'x' }, mount)).toMatchObject({ kind: 'call.foo_bar' });
    expect(checkErrorEvent({ kind: 'host.internal', message: 'made up' }, mount)).toBeUndefined();
    expect(checkErrorEvent({ kind: 'uncaught', message: 'old' }, mount)).toBeUndefined();
  });

  it("drop a call's stack, which is the SDK's, and any call a frame names: that is the host's to say", () => {
    const call = checkErrorEvent({ kind: 'call.export', message: 'x', call: 'export.query', stack: 'sdk' }, mount);
    expect(call).not.toHaveProperty('call');
    expect(call).not.toHaveProperty('stack');
    expect(checkErrorEvent({ kind: 'app.uncaught', message: 'x', call: 'made.up', stack: 'at f' }, mount))
      .toEqual(expect.objectContaining({ stack: 'at f' }));
    expect(checkErrorEvent({ kind: 'app.uncaught', message: 'x', call: 'made.up' }, mount)).not.toHaveProperty('call');
  });

  it('name an API as a kind does', () => {
    expect(snakeCase('fetchMore')).toBe('fetch_more');
    expect(snakeCase('loadOptions')).toBe('load_options');
    expect(snakeCase('lineage')).toBe('lineage');
  });
});

describe('a failed call', () => {
  it('is named as the app called it: the outermost name', () => {
    const err = new DataAppError({ code: 'forbidden', scope: 'user', message: 'No.' });
    tagCall(err, 'inner');
    tagCall(err, 'export');
    expect(callOf(err)).toBe('export');
  });

  it('keeps its name when said of an entity, or hidden behind call_failed', () => {
    const err = new DataAppError({ code: 'forbidden', scope: 'user', message: 'No.' });
    tagCall(err, 'fetch_more');
    expect(callOf(err.about('orders'))).toBe('fetch_more');
    expect(callOf(new DataAppError({ code: 'call_failed', scope: 'server', message: 'The call failed.', cause: err }))).toBe('fetch_more');
  });

  it('is named by a named call when it rejects, and nothing else changes', async () => {
    const failing = namedCall(async () => { throw new DataAppError({ code: 'forbidden', scope: 'user', message: 'No.' }); }, 'lineage');
    expect(callOf(await failing().catch((err: unknown) => err))).toBe('lineage');
    expect(namedCall((n: number) => n + 1, 'sync')(1)).toBe(2);
  });
});
