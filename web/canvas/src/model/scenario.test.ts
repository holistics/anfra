import { describe, expect, it } from 'vitest';
import { answer, opOf, scenarioOps, type Scenarios } from './scenario.ts';

const scenarios: Scenarios = {
  default: { ops: { 'sessions.get': { user: { id: 'u1' } }, 'apps.list': { apps: [1, 2] } } },
  empty: { label: 'No apps', ops: { 'apps.list': { apps: [] } } },
  broken: { extends: 'empty', ops: { 'apps.list': { $status: 500, $body: { code: 'internal' } } } },
  alone: { extends: false, ops: {} },
};

describe('scenarios', () => {
  it('extend default unless they say otherwise', () => {
    expect(scenarioOps(scenarios, 'empty')).toEqual({ 'sessions.get': { user: { id: 'u1' } }, 'apps.list': { apps: [] } });
    expect(scenarioOps(scenarios, 'broken')['apps.list']).toMatchObject({ $status: 500 });
    expect(scenarioOps(scenarios, 'alone')).toEqual({});
  });

  it('read the op and org from an address', () => {
    expect(opOf('/api/o/acme/apps.list')).toEqual({ op: 'apps.list', org: 'acme' });
    expect(opOf('/api/sessions.get')).toEqual({ op: 'sessions.get' });
    expect(opOf('/assets/x.js')).toBeUndefined();
  });

  it('answer values, functions and errors, and say what is not mocked', async () => {
    const ops = { a: { ok: 1 }, b: ({ org }: { org?: string }) => ({ org }), c: { $status: 403 } };
    expect(await answer(ops, { op: 'a', body: {} })).toEqual({ $status: 200, $body: { ok: 1 } });
    expect(await answer(ops, { op: 'b', org: 'acme', body: {} })).toEqual({ $status: 200, $body: { org: 'acme' } });
    expect(await answer(ops, { op: 'c', body: {} })).toEqual({ $status: 403 });
    expect((await answer(ops, { op: 'd', body: {} })).$status).toBe(501);
  });
});
