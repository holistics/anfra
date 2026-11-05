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

  it("keep a call's bridge call and drop its stack, and the reverse for any other kind", () => {
    expect(checkErrorEvent({ kind: 'call.export', message: 'x', call: 'export.query', stack: 'sdk' }, mount))
      .toEqual(expect.objectContaining({ call: 'export.query' }));
    expect(checkErrorEvent({ kind: 'call.export', message: 'x', stack: 'sdk' }, mount)).not.toHaveProperty('stack');
    expect(checkErrorEvent({ kind: 'app.uncaught', message: 'x', call: 'made.up', stack: 'at f' }, mount))
      .toEqual(expect.not.objectContaining({ call: expect.anything() }));
  });

  it('name an API as a kind does', () => {
    expect(snakeCase('fetchMore')).toBe('fetch_more');
    expect(snakeCase('loadOptions')).toBe('load_options');
    expect(snakeCase('lineage')).toBe('lineage');
  });
});

describe('a failed call', () => {
  it('is named as the app called it (the outermost) and by the bridge call it sent (the innermost)', () => {
    const err = new DataAppError({ code: 'forbidden', scope: 'user', message: 'No.' });
    tagCall(err, { call: 'export.query' });
    tagCall(err, { call: 'other.call', api: 'inner' });
    tagCall(err, { api: 'export' });
    expect(callOf(err)).toEqual({ api: 'export', call: 'export.query' });
  });

  it('keeps its name when said of an entity, or hidden behind call_failed', () => {
    const err = new DataAppError({ code: 'forbidden', scope: 'user', message: 'No.' });
    tagCall(err, { api: 'fetch_more', call: 'core.submitQuery' });
    expect(callOf(err.about('orders'))).toEqual({ api: 'fetch_more', call: 'core.submitQuery' });
    expect(callOf(new DataAppError({ code: 'call_failed', scope: 'server', message: 'The call failed.', cause: err }))).toEqual({ api: 'fetch_more', call: 'core.submitQuery' });
  });

  it('is named by a named call when it rejects, and nothing else changes', async () => {
    const failing = namedCall(async () => { throw new DataAppError({ code: 'forbidden', scope: 'user', message: 'No.' }); }, 'lineage');
    expect(callOf(await failing().catch((err: unknown) => err))).toEqual({ api: 'lineage', call: undefined });
    expect(namedCall((n: number) => n + 1, 'sync')(1)).toBe(2);
  });
});
