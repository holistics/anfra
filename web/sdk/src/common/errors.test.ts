import { describe, expect, it } from 'vitest';
import {
  DataAppError, GENERIC_CODES, ValidationError, fromHttpStatus, hasErrorCode, type ErrorDetails,
} from './errors';
import type { components } from '../api/schema';

describe('fromHttpStatus, for a backend without the API error document', () => {
  it.each([
    [401, undefined, 'unauthenticated', 'user'],
    [403, undefined, 'forbidden', 'user'],
    [422, undefined, 'invalid_request', 'client'],
    [400, 'query_invalid', 'query_invalid', 'user'],
    [500, undefined, 'internal_server_error', 'server'],
    [503, undefined, 'internal_server_error', 'server'],
    [undefined, undefined, 'connection_failed', 'server'],
  ] as const)('answers %s as %s', (status, refused, code, scope) => {
    const err = fromHttpStatus(status, 'Nope.', refused ? { refused } : {});
    expect(err).toBeInstanceOf(DataAppError);
    expect(err).toMatchObject({ code, scope, message: 'Nope.' });
  });

  it("gives a backend's own refusal code a scope by its status", () => {
    expect(fromHttpStatus(409, 'Taken.', { refused: 'name_taken' })).toMatchObject({ code: 'name_taken', scope: 'user', status: 409 });
  });
});

describe('the shape', () => {
  it('gives a declaration error its code and scope', () => {
    expect(new ValidationError('No dataset sales.', 'revenue')).toMatchObject({
      name: 'ValidationError', code: 'declaration_invalid', scope: 'client', entity: 'revenue',
    });
  });

  it('names the entity an error is about without losing the rest', () => {
    const err = new DataAppError({
      code: 'result_too_large', scope: 'user', message: 'Too many rows.', status: 422, requestId: 'req_1', details: { max_rows: 10 },
    });
    expect(err.about('revenue')).toMatchObject({
      code: 'result_too_large', scope: 'user', status: 422, requestId: 'req_1', details: { max_rows: 10 }, entity: 'revenue',
    });
  });

  it('types the details of a code the API documents', () => {
    const err: unknown = new DataAppError({ code: 'result_too_large', scope: 'user', message: 'Too many rows.', details: { max_rows: 10 } });
    if (!hasErrorCode(err, 'result_too_large')) throw new Error('expected result_too_large');
    // Compiles only while the details are the API's ResultLimit.
    const limit: components['schemas']['ResultLimit'] | undefined = err.details;
    expect(limit?.max_rows).toBe(10);
    const typed: ErrorDetails<'query_invalid'> = { valid: false, diagnostics: [] } as ErrorDetails<'query_invalid'>;
    expect(typed).toBeDefined();
  });

  it("reuses the API's codes for its generic codes where the API has one", () => {
    expect(GENERIC_CODES.query_invalid).toBe('user');
    expect(GENERIC_CODES.internal_server_error).toBe('server');
  });
});
