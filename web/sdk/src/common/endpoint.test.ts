import { describe, expect, it, vi } from 'vitest';
import { createEndpoint, fromBridgeError, toBridgeError } from './endpoint';
import type { BridgeMessage } from './bridge';
import {
  DataAppError, ValidationError, connectionFailed, forbidden,
} from './errors';

describe('an error crossing the bridge', () => {
  it("keeps an SDK error's shape: code, scope, status and the request's id", () => {
    const sent = new DataAppError({
      code: 'sidecar_unavailable', scope: 'server', message: 'anfra-node is not responding.', status: 503, requestId: 'req_1',
    });

    const got = fromBridgeError(toBridgeError(sent));

    expect(got).toBeInstanceOf(DataAppError);
    expect(got).toMatchObject({
      code: 'sidecar_unavailable', scope: 'server', message: 'anfra-node is not responding.', status: 503, requestId: 'req_1',
    });
  });

  it('keeps a declaration error a ValidationError', () => {
    expect(fromBridgeError(toBridgeError(new ValidationError('No dataset sales.')))).toBeInstanceOf(ValidationError);
  });

  it('passes details only for a code the SDK knows, and steps only as text', () => {
    const limit = { max_rows: 100000 };
    const crossed = toBridgeError(new DataAppError({
      code: 'result_too_large', scope: 'user', message: 'Too many rows.', details: limit,
      context: [{ step: 'query', text: 'running the query', params: { path: '/srv/repo' } } as never],
    }));
    expect(crossed.details).toEqual(limit);
    expect(crossed.context).toEqual([{ step: 'query', text: 'running the query' }]);

    const unknown = toBridgeError(new DataAppError({
      code: 'cloud_quota', scope: 'user', message: 'Over quota.', details: { tenant: 't_1' },
    }));
    expect(unknown).not.toHaveProperty('details');
  });

  it('reads a scope it does not know as the server\'s', () => {
    const got = fromBridgeError({ name: 'DataAppError', code: 'odd', scope: 'nobody', message: 'Odd.' });
    expect(got.scope).toBe('server');
  });

  it('carries nothing of any other error', () => {
    const crossing = toBridgeError(Object.assign(new SyntaxError('Unexpected token < at https://internal/x'), { requestId: 'req_2' }));

    expect(crossing).toEqual({
      name: 'DataAppError', code: 'call_failed', scope: 'server', message: 'The call failed.',
    });
    expect(fromBridgeError(crossing)).toMatchObject({ code: 'call_failed' });
  });

  it('keeps the helpers\' errors', () => {
    expect(fromBridgeError(toBridgeError(forbidden('No.')))).toMatchObject({ code: 'forbidden', scope: 'user' });
    expect(fromBridgeError(toBridgeError(connectionFailed('Cut.', 502)))).toMatchObject({ code: 'connection_failed', status: 502 });
  });
});

describe('a call that fails on the answering side', () => {
  /** Two endpoints posting to each other, the host answering with `answer`. */
  function pair (answer: () => Promise<unknown>) {
    let frame: ReturnType<typeof createEndpoint> | undefined;
    const host = createEndpoint((m: BridgeMessage) => frame!.receive(m), { answer, event: () => {} });
    frame = createEndpoint((m: BridgeMessage) => host.receive(m), { answer: async () => undefined, event: () => {} });
    return frame;
  }

  it('logs an error it hides where it was raised', async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});
    const bug = new TypeError("Cannot read properties of undefined (reading 'rows')");

    const got = await pair(() => Promise.reject(bug)).call('core.query', {}).catch((e) => e);

    expect(got).toMatchObject({ code: 'call_failed', message: 'The call failed.' });
    expect(error).toHaveBeenCalledWith(expect.stringContaining("'core.query' failed"), bug);
    error.mockRestore();
  });

  it("does not log an SDK error, which crosses as it is", async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});

    const got = await pair(() => Promise.reject(connectionFailed('Down.', 503))).call('core.query', {}).catch((e) => e);

    expect(got).toMatchObject({ code: 'connection_failed', message: 'Down.' });
    expect(error).not.toHaveBeenCalled();
    error.mockRestore();
  });
});
