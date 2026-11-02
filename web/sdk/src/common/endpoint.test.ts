import { describe, expect, it, vi } from 'vitest';
import { createEndpoint, fromBridgeError, toBridgeError } from './endpoint';
import type { BridgeMessage } from './bridge';
import { QueryError, TransportError } from './errors';

describe('an error crossing the bridge', () => {
  it("keeps an SDK error's request id and status", () => {
    const sent = new TransportError('anfra-node is not responding.', 503);
    sent.requestId = 'req_1';

    const got = fromBridgeError(toBridgeError(sent));

    expect(got).toBeInstanceOf(TransportError);
    expect(got).toMatchObject({ message: 'anfra-node is not responding.', status: 503, requestId: 'req_1' });
  });

  it('carries nothing of any other error', () => {
    const crossing = toBridgeError(Object.assign(new SyntaxError('Unexpected token < at https://internal/x'), { requestId: 'req_2' }));

    expect(crossing).toEqual({ name: 'QueryError', message: 'The call failed.' });
    expect(fromBridgeError(crossing)).toBeInstanceOf(QueryError);
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

    expect(got).toMatchObject({ name: 'QueryError', message: 'The call failed.' });
    expect(error).toHaveBeenCalledWith(expect.stringContaining("'core.query' failed"), bug);
    error.mockRestore();
  });

  it("does not log an SDK error, which crosses as it is", async () => {
    const error = vi.spyOn(console, 'error').mockImplementation(() => {});

    const got = await pair(() => Promise.reject(new TransportError('Down.', 503))).call('core.query', {}).catch((e) => e);

    expect(got).toMatchObject({ name: 'TransportError', message: 'Down.' });
    expect(error).not.toHaveBeenCalled();
    error.mockRestore();
  });
});
