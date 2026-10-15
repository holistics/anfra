import { describe, expect, it, vi } from 'vitest';
import { PermissionError, QueryError, TransportError } from '../common/errors';
import type { BackendQueryRequest, DatasetDescriptor, SemanticType } from '../common/types';
import { coreApiBackend, SUGGESTION_LIMIT } from './backend';
import { coreClient } from './client';
import type { components } from './schema';

type Answer = { status: number, body: unknown } | { events: string } | Error;

/** A core API answering each call with the next answer, recording what it was sent. */
function api (...answers: Answer[]) {
  const sent: { url: string, body: unknown, accept?: string }[] = [];
  const fetchImpl = vi.fn(async (input: Request) => {
    sent.push({ url: input.url, body: await input.clone().json(), accept: input.headers.get('Accept') ?? undefined });
    const answer = answers.shift();
    if (answer instanceof Error) throw answer;
    if (answer && 'events' in answer) {
      return new Response(answer.events, { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } });
    }
    return new Response(JSON.stringify(answer?.body), { status: answer?.status ?? 500, headers: { 'Content-Type': 'application/json' } });
  });
  return { client: coreClient('http://host/api', fetchImpl as unknown as typeof fetch), sent };
}

const ok = (body: unknown): Answer => ({ status: 200, body });
/** A streamed answer: each event a line, as the server writes them. */
const streamed = (...events: unknown[]): Answer => ({ events: events.map((e) => JSON.stringify(e) + '\n').join('') });

/** A query's page, streamed as the server streams it: its SQL, its header, its rows, its summary. */
function asStream (page: { sql: string, aql?: string, columns: unknown[], result: { fields: string[], records: unknown[][] } }): Answer {
  return streamed(
    { type: 'start' },
    { type: 'compiled', sql: page.sql, ...(page.aql ? { aql: page.aql } : {}) },
    { type: 'header', columns: page.columns, fields: page.result.fields },
    ...(page.result.records.length ? [{ type: 'rows', records: page.result.records }] : []),
    { type: 'summary', row_count: page.result.records.length, has_more: false, timings: { steps: [{ step: 'compiling_query', start_ms: 0 }, { step: 'executing_query', start_ms: 40 }], end_ms: 95 } },
    { type: 'end', status: 'succeeded' },
  );
}
const fail = (status: number, error: Record<string, unknown>): Answer => ({ status, body: { error } });

const request: BackendQueryRequest = {
  dataset: 'sales',
  aql: 'explore { dimensions { status: orders.status } }',
  input: { filters: [{ field: 'orders.status', operator: 'is', values: ['paid'] }], conditions: [], sorts: [], dateDrills: [{ field: 'orders.created_at', grain: 'month' }] },
  page: 2,
  pageSize: 10,
  timezone: 'Asia/Ho_Chi_Minh',
};
const signal = new AbortController().signal;

const answered = {
  sql: 'select 1',
  aql: 'explore { … }',
  columns: [{
    name: 'status', fieldName: 'status', modelId: 'orders', label: 'Status', adhoc: false, isMeasure: false,
    semantic_type: 'text', arrow_type: { type: 'utf8', label: 'utf8' }, db_type: 'text',
  }],
  result: { fields: ['status'], records: [['paid'], ['open']] },
};

/** True when A and B are the same set of types; false when either has one the other lacks. */
type Exactly<A, B> = [A] extends [B] ? ([B] extends [A] ? true : false) : false;

// Compiles only while the SDK's semantic types are exactly the ones the API answers: one the API
// adds, or one it no longer answers, fails the typecheck here.
const semanticTypesAreTheApis: Exactly<SemanticType, NonNullable<components['schemas']['Column']['semantic_type']>> = true;

it("names exactly the semantic types the API answers", () => {
  expect(semanticTypesAreTheApis).toBe(true);
});

describe('coreApiBackend', () => {
  it('runs a query as core.query, streamed, the Query Input passed through, and answers it the way the SDK takes it', async () => {
    const { client, sent } = api(asStream(answered));
    const result = await coreApiBackend(client).submitQuery(request, signal);
    expect(sent).toEqual([{
      url: 'http://host/api/core.query',
      body: { query: request.aql, dataset: 'sales', input: request.input, page: 2, page_size: 10, timezone: 'Asia/Ho_Chi_Minh' },
      accept: 'application/x-ndjson',
    }]);
    expect(result).toMatchObject({
      columns: answered.columns,
      values: [['paid'], ['open']],
      meta: { page: 2, pageSize: 10, numRows: 2 },
      debug: {
        executedAql: 'explore { … }', executedSql: 'select 1', fromCache: false,
        timings: { steps: [{ step: 'compiling_query', startMs: 0 }, { step: 'executing_query', startMs: 40 }], endMs: 95 },
      },
    });
  });

  it("fails a query whose columns come without their types, which a Data App decides by", async () => {
    const { semantic_type: _, ...untyped } = answered.columns[0];
    const { client } = api(asStream({ ...answered, columns: [untyped] }));
    await expect(coreApiBackend(client).submitQuery(request, signal)).rejects.toThrowError(
      new QueryError("The server answered column 'status' without its types (semantic_type, arrow_type, db_type), "
        + 'which a Data App decides by: the server is older than this SDK.'),
    );
  });

  describe('an unpaged query', () => {
    const { page, pageSize, ...unpaged } = request;
    const events = [
      { type: 'start' },
      { type: 'status', phase: 'compiling' },
      { type: 'compiled', sql: answered.sql, aql: answered.aql },
      { type: 'status', phase: 'executing' },
      { type: 'header', columns: answered.columns, fields: ['status'] },
      { type: 'ping' },
      { type: 'rows', records: [['paid']] },
      { type: 'rows', records: [['open']] },
      { type: 'summary', row_count: 2, has_more: false, timings: { steps: [{ step: 'compiling_query', start_ms: 0 }], end_ms: 12 } },
    ];

    it('is sent unpaged, and gathered into every row', async () => {
      const { client, sent } = api(streamed(...events, { type: 'end', status: 'succeeded' }));
      const result = await coreApiBackend(client).submitQuery(unpaged, signal);
      expect(sent[0].body).not.toHaveProperty('page_size');
      expect(sent[0].accept).toBe('application/x-ndjson');
      expect(result).toMatchObject({
        columns: answered.columns,
        values: [['paid'], ['open']],
        meta: { numRows: 2 },
        debug: { executedAql: 'explore { … }', executedSql: 'select 1' },
      });
    });

    it('fails as its end says, as an error response would', async () => {
      const failed = { type: 'end', status: 'failed', error: { code: 'result_too_large', scope: 'user', status: 422, message: 'Too many rows.', request_id: 'req_1' } };
      const { client } = api(streamed(...events.slice(0, 7), failed));
      const err = await coreApiBackend(client).submitQuery(unpaged, signal).catch((e) => e);
      expect(err).toBeInstanceOf(QueryError);
      expect(err.message).toBe('Too many rows.');
    });

    it('fails when the stream is cut before its end', async () => {
      const { client } = api(streamed(...events.slice(0, 7)));
      const err = await coreApiBackend(client).submitQuery(unpaged, signal).catch((e) => e);
      expect(err).toBeInstanceOf(TransportError);
    });

    it('fails as an error response when refused before it starts', async () => {
      const { client } = api(fail(422, { code: 'validation_failed', scope: 'user', message: 'Invalid.', details: { violations: [{ field: 'dataset', message: 'Unknown dataset.' }] } }));
      const err = await coreApiBackend(client).submitQuery(unpaged, signal).catch((e) => e);
      expect(err).toBeInstanceOf(QueryError);
      expect(err.message).toBe('dataset: Unknown dataset.');
    });
  });

  it.each([
    ['an invalid query, as its diagnostics', fail(422, { code: 'query_invalid', scope: 'user', message: 'The query is invalid.', details: { valid: false, diagnostics: [{ message: 'No field x.', line: 1, column: 9 }] } }), QueryError, 'line 1:9: No field x.'],
    ['an input entry refused, as its violation', fail(422, { code: 'validation_failed', scope: 'user', message: 'Some of the input is not valid.', details: { violations: [{ field: 'input.filters[0].field', message: 'Unknown field.' }] } }), QueryError, 'input.filters[0].field: Unknown field.'],
    ['a data source failing', fail(502, { code: 'query_failed', scope: 'server', message: 'connection refused' }), QueryError, 'connection refused'],
    ['a caller who may not see it', fail(403, { code: 'forbidden', scope: 'user', message: 'Not yours.' }), PermissionError, 'Not yours.'],
    ['a sidecar down', fail(503, { code: 'sidecar_unavailable', scope: 'server', message: 'anfra-node is not responding.' }), TransportError, 'anfra-node is not responding.'],
    ['a code this SDK does not know, by its scope', fail(500, { code: 'something_new', scope: 'server', message: 'Broke.' }), TransportError, 'Broke.'],
    ['a server that cannot be reached', new TypeError('fetch failed'), TransportError, 'The server could not be reached: fetch failed'],
  ])('fails %s', async (_name, answer, Kind, message) => {
    const { client } = api(answer);
    const err = await coreApiBackend(client).submitQuery(request, signal).catch((e) => e);
    expect(err).toBeInstanceOf(Kind);
    expect(err.message).toBe(message);
  });

  it('lets an abort through as it is', async () => {
    const { client } = api(Object.assign(new Error('aborted'), { name: 'AbortError' }));
    await expect(coreApiBackend(client).submitQuery(request, signal)).rejects.toMatchObject({ name: 'AbortError' });
  });

  describe('lineage', () => {
    const traced = {
      results: [{
        target: { parent: 'orders', child: 'amount' },
        query: 'needed_by:(fqn:orders.amount)',
        diagram: {
          version: 1, query: 'needed_by:(fqn:orders.amount)', total: 0, matched: [], schema: { edges: {}, types: {} }, layers: [], cards: [],
        },
      }],
    };

    it('traces as core.lineage, sending the request and answering the answer as they are', async () => {
      const { client, sent } = api(ok(traced));
      const request = { targets: [{ parent: 'orders', child: 'amount' }], limit: 5, offset: 10 };
      await expect(coreApiBackend(client).lineage!(request, signal)).resolves.toEqual(traced);
      expect(sent).toEqual([{ url: 'http://host/api/core.lineage', body: request }]);
    });

    it("maps the API's error to the SDK's", async () => {
      const { client } = api(fail(503, { code: 'anfra.sidecar_unavailable', scope: 'Server', message: 'this command requires the anfra-node sidecar' }));
      await expect(coreApiBackend(client).lineage!({ targets: [{ parent: 'orders', child: 'amount' }] }, signal)).rejects.toThrowError(TransportError);
    });
  });

  describe('field suggestions', () => {
    const datasets: Record<string, DatasetDescriptor> = {
      sales: {
        id: 'sales',
        name: 'sales',
        data_models: [{
          id: 'orders',
          name: 'orders',
          fields: [
            { name: 'status', type: 'text', is_custom_measure: false },
            { name: 'amount', type: 'number', is_custom_measure: false },
          ],
        }],
        metrics: [],
      },
    };

    it("are one distinct-values query, narrowed for a text field by what the reader typed", async () => {
      const { client, sent } = api(asStream({ ...answered, result: { fields: ['value'], records: [['paid'], [null], [{ x: 1 }], [3], [true]] } }));
      const values = await coreApiBackend(client, { datasets }).fieldSuggestions({ dataset: 'sales', model: 'orders', field: 'status', q: ' pa ' }, signal);
      expect(sent[0].body).toEqual({
        query: 'explore { dimensions { value: orders.status } }',
        dataset: 'sales',
        input: { filters: [{ field: 'orders.status', operator: 'contains', values: ['pa'] }], sorts: [{ field: 'value', direction: 'asc' }] },
        page: 1,
        page_size: SUGGESTION_LIMIT,
      });
      expect(values).toEqual(['paid', 3, true]);
    });

    it('do not narrow a field that is not text', async () => {
      const { client, sent } = api(asStream({ ...answered, result: { fields: ['value'], records: [] } }));
      await coreApiBackend(client, { datasets }).fieldSuggestions({ dataset: 'sales', model: 'orders', field: 'amount', q: '12' }, signal);
      expect((sent[0].body as { input: { filters: unknown[] } }).input.filters).toEqual([]);
    });

    it('run only for a field the dataset defines, since it is spliced into AQL', async () => {
      const { client, sent } = api();
      await expect(coreApiBackend(client, { datasets }).fieldSuggestions({ dataset: 'sales', model: 'orders', field: 'x } } drop', q: '' }, signal))
        .rejects.toThrow(QueryError);
      expect(sent).toEqual([]);
    });
  });

  describe('exportQuery', () => {
    const exportRequest = {
      dataset: 'sales',
      aql: request.aql,
      input: request.input,
      timezone: 'Asia/Ho_Chi_Minh',
      format: 'csv',
      formatOptions: { bom: true, header: 'names' as const },
      filename: 'Sales - revenue',
    };

    it('exports as core.query.export, and answers the link the way the SDK takes it', async () => {
      const { client, sent } = api(ok({
        url: 'http://host/exports/download/t0k3n/Sales%20-%20revenue.csv',
        filename: 'Sales - revenue.csv',
        format: 'csv',
        row_count: 2,
        expires_at: '2026-10-13T10:00:00Z',
      }));
      const link = await coreApiBackend(client).exportQuery!(exportRequest, signal);
      expect(sent).toEqual([{
        url: 'http://host/api/core.query.export',
        body: {
          query: request.aql,
          dataset: 'sales',
          input: request.input,
          timezone: 'Asia/Ho_Chi_Minh',
          format: 'csv',
          format_options: { bom: true, header: 'names' },
          filename: 'Sales - revenue',
        },
      }]);
      expect(link).toEqual({
        url: 'http://host/exports/download/t0k3n/Sales%20-%20revenue.csv',
        filename: 'Sales - revenue.csv',
        format: 'csv',
        rowCount: 2,
        expiresAt: new Date('2026-10-13T10:00:00Z'),
      });
    });

    it('fails as the SDK error the API error is', async () => {
      const { client } = api(
        fail(500, { code: 'exports_unavailable', scope: 'server', message: 'This server cannot make exports.' }),
        fail(422, {
          code: 'validation_failed', scope: 'client', message: 'invalid', details: { violations: [{ field: 'filename', message: 'a file name cannot hold control characters' }] },
        }),
      );
      const backend = coreApiBackend(client);
      await expect(backend.exportQuery!(exportRequest, signal)).rejects.toThrow(TransportError);
      await expect(backend.exportQuery!(exportRequest, signal)).rejects.toThrow(/filename: a file name cannot hold control characters/);
    });
  });
});
