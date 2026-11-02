import { describe, expect, it } from 'vitest';
import { fromBridgeError, toBridgeError } from './endpoint';
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
