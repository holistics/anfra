import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { BridgeMessage } from '../common/bridge';
import { PermissionError, TransportError, ValidationError } from '../common/errors';
import type { Backend } from '../common/types';
import { salesDataset, testUser } from '../app/testSupport';
import { corePlugin } from '../plugins/core';
import { createHost, type MountedDataApp } from './runtime';
import { definePlugin, type CallContext, type Middleware, type Plugin } from './plugin';

let replies: BridgeMessage[];
let mounted: MountedDataApp | undefined;

beforeEach(() => {
  replies = [];
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.restoreAllMocks();
});

const settle = () => new Promise((resolve) => { setTimeout(resolve, 0); });

function backend (overrides: Partial<Backend> = {}): Backend {
  return {
    submitQuery: vi.fn(async () => ({ columns: [], values: [[1]] })),
    fieldSuggestions: vi.fn(async () => ['a', 'b']),
    ...overrides,
  };
}

const query = { dataset: 'sales', aql: 'explore {}', input: { filters: [], conditions: [], sorts: [], dateDrills: [] } };

/** Mount a frame on a host with these plugins, and catch what the host posts to it. */
function mount (plugins: Plugin[], middleware: Middleware[] = []) {
  const host = createHost({ plugins, middleware, frameScript: 'window.FRAME = 1;' });
  mounted = host.mount({
    container: document.body,
    definition: '<head></head><body>hi</body>',
    baseHref: '/appserve/files/',
    datasets: { sales: salesDataset },
    user: testUser,
  });
  vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { replies.push(message as BridgeMessage); });
  return { host, mounted };
}

function fromFrame (message: BridgeMessage, source: Window | null = mounted!.frame.contentWindow) {
  window.dispatchEvent(new MessageEvent('message', { data: message, source }));
}

const call = (id: number, name: string, request: unknown = null) => fromFrame({
  type: 'anfra:call', id, name, request,
});

describe('createHost', () => {
  it('needs core, refuses a plugin twice, and one whose plugins are missing', () => {
    const core = corePlugin(backend());
    const needy = definePlugin({ name: 'needy', version: '1', requires: ['export', 'lineage'] });
    expect(() => createHost({ plugins: [] })).toThrow(/needs the 'core' plugin/);
    expect(() => createHost({ plugins: [core, core] })).toThrow(/installed twice/);
    expect(() => createHost({ plugins: [core, needy] })).toThrow("Plugin 'needy' needs 'export', 'lineage', which are not installed.");
  });

  it('says what it installed, in order', () => {
    const host = createHost({ plugins: [corePlugin(backend()), definePlugin({ name: 'extra', version: '2' })] });
    expect(host.plugins).toEqual([{ name: 'core', version: expect.any(String) }, { name: 'extra', version: '2' }]);
  });
});

describe('definePlugin', () => {
  it('refuses a name that could not name calls and API, or that is the SDK\'s', () => {
    expect(() => definePlugin({ name: 'My Plugin', version: '1' })).toThrow(/lowercase/);
    expect(() => definePlugin({ name: 'plugins', version: '1' })).toThrow(/reserved/);
    expect(() => definePlugin({ name: 'ok', version: '1', host: { calls: { 'a.b': () => 1 } } })).toThrow(/not a call name/);
  });
});

describe('mounting', () => {
  it('mounts the definition in a frame that may run scripts and nothing more, provisioned with the plugins', () => {
    const extra = definePlugin({ name: 'extra', version: '2', frame: { setup: 'var __anfraPlugin = { default: function () {} };' } });
    mount([corePlugin(backend()), extra]);
    const { frame } = mounted!;
    expect(frame.getAttribute('sandbox')).toBe('allow-scripts');
    expect(frame.parentElement).toBe(document.body);
    expect(frame.srcdoc).toContain('window.FRAME = 1;');
    expect(frame.srcdoc).toContain('"hostOrigin":"http://localhost:3000"');
    expect(frame.srcdoc).toContain('"plugins":[{"name":"core"');
    expect(frame.srcdoc).toContain('__setups.push(["extra", __anfraPlugin && __anfraPlugin.default]);');
    expect(frame.srcdoc).toContain('<body>hi</body>');
    mounted!.unmount();
    expect(frame.isConnected).toBe(false);
    mounted = undefined;
  });

  it('refuses to mount without its frame script', () => {
    const host = createHost({ plugins: [corePlugin(backend())] });
    expect(() => host.mount({
      container: document.body, definition: '', baseHref: '/', datasets: {}, user: testUser,
    })).toThrow(/built without its frame script/);
  });
});

describe('the frame\'s calls', () => {
  it('are answered by the plugin that has them', async () => {
    const b = backend();
    mount([corePlugin(b)]);
    call(1, 'core.submitQuery', query);
    call(2, 'core.fieldSuggestions', { dataset: 'sales', model: 'orders', field: 'status', q: '' });
    await settle();
    expect(b.submitQuery).toHaveBeenCalledWith(query, expect.any(AbortSignal));
    expect(replies).toEqual([
      { type: 'anfra:result', id: 1, ok: true, result: { columns: [], values: [[1]] } },
      { type: 'anfra:result', id: 2, ok: true, result: ['a', 'b'] },
    ]);
  });

  it('send a failure across as its error class by name', async () => {
    mount([corePlugin(backend({ submitQuery: async () => { throw new PermissionError('Not yours.'); } }))]);
    call(1, 'core.submitQuery', query);
    await settle();
    expect(replies).toEqual([{ type: 'anfra:result', id: 1, ok: false, error: { name: 'PermissionError', message: 'Not yours.' } }]);
  });

  it('reach only what a plugin let the frame call, and only what is installed', async () => {
    const handler = vi.fn(() => 'secret');
    const agent = definePlugin({ name: 'agent', version: '1', host: { calls: { setKey: handler } } });
    mount([corePlugin(backend()), agent]);
    call(1, 'agent.setKey', 'k');
    call(2, 'ingest.run');
    await settle();
    expect(handler).not.toHaveBeenCalled();
    expect(replies).toEqual([
      { type: 'anfra:result', id: 1, ok: false, error: { name: 'PermissionError', message: "'agent.setKey' can't be called from a Data App." } },
      { type: 'anfra:result', id: 2, ok: false, error: { name: 'TransportError', message: "Nothing on this host answers 'ingest.run'." } },
    ]);
  });

  it('are served from their own frame only', async () => {
    const b = backend();
    mount([corePlugin(b)]);
    fromFrame({ type: 'anfra:call', id: 1, name: 'core.submitQuery', request: query }, window);
    await settle();
    expect(b.submitQuery).not.toHaveBeenCalled();
    expect(replies).toEqual([]);
  });

  it('are aborted when the frame cancels, and answered with nothing', async () => {
    let signal: AbortSignal | undefined;
    mount([corePlugin(backend({
      submitQuery: (_r, s) => new Promise((_resolve, reject) => {
        signal = s;
        s.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));
      }),
    }))]);
    call(7, 'core.submitQuery', query);
    await settle();
    fromFrame({ type: 'anfra:cancel', id: 7 });
    await settle();
    expect(signal?.aborted).toBe(true);
    expect(replies).toEqual([]);
  });

  it('carry who made them: the frame, and the mount it is', async () => {
    let seen: CallContext | undefined;
    const spy = definePlugin({
      name: 'spy', version: '1', host: { calls: { look: { frame: true, handler: (_r, context) => { seen = context; return null; } } } },
    });
    mount([corePlugin(backend()), spy]);
    call(1, 'spy.look');
    await settle();
    expect(seen).toMatchObject({ name: 'spy.look', caller: { kind: 'frame' }, mount: { frame: mounted!.frame } });
  });

  it('answer a plugin\'s frame body, and nothing for a plugin without one', async () => {
    mount([corePlugin(backend()), definePlugin({ name: 'heavy', version: '1', frame: { setup: '', body: 'BODY' } })]);
    call(1, 'anfra.body', { plugin: 'heavy' });
    call(2, 'anfra.body', { plugin: 'core' });
    await settle();
    expect(replies).toEqual([
      { type: 'anfra:result', id: 1, ok: true, result: 'BODY' },
      { type: 'anfra:result', id: 2, ok: false, error: { name: 'ValidationError', message: "No plugin 'core' with a frame body is installed." } },
    ]);
  });
});

describe('middleware', () => {
  it('wraps the calls it matches, outermost by stage then name, never by registration', async () => {
    const order: string[] = [];
    const layer = (name: string, match: Middleware['match'], stage?: number): Middleware => ({
      name,
      match,
      stage,
      handle: async (request, context, next) => {
        order.push(`${name}:${context.name}`);
        return next(request);
      },
    });
    let trace: readonly string[] = [];
    const spy = definePlugin({
      name: 'spy', version: '1', host: { calls: { look: { frame: true, handler: (_r, context) => { trace = [...context.trace]; return null; } } } },
    });
    mount([corePlugin(backend()), spy], [layer('zeta', '*'), layer('alpha', 'spy.*'), layer('first', '*', -1), layer('other', 'core.submitQuery')]);
    call(1, 'spy.look');
    await settle();
    expect(order).toEqual(['first:spy.look', 'alpha:spy.look', 'zeta:spy.look']);
    expect(trace).toEqual(['first', 'alpha', 'zeta']);
  });

  it('may change the request, answer in the plugin\'s place, refuse, or change the answer', async () => {
    const b = backend();
    const cache: Middleware = {
      name: 'cache', match: 'core.submitQuery', handle: async (request, _context, next) => ((request as { aql: string }).aql === 'cached' ? 'from cache' : next(request)),
    };
    const quota: Middleware = {
      name: 'quota', match: 'core.*', handle: async (request, context, next) => {
        if (context.name === 'core.fieldSuggestions') throw new PermissionError('Over quota.');
        return next({ ...(request as object), tagged: true });
      },
    };
    const wrap: Middleware = { name: 'wrap', match: 'core.submitQuery', stage: -1, handle: async (request, _context, next) => ({ wrapped: await next(request) }) };
    mount([corePlugin(b)], [cache, quota, wrap]);
    call(1, 'core.submitQuery', { aql: 'cached' });
    call(2, 'core.submitQuery', query);
    call(3, 'core.fieldSuggestions', {});
    await settle();
    expect(b.submitQuery).toHaveBeenCalledWith({ ...query, tagged: true }, expect.any(AbortSignal));
    expect(b.fieldSuggestions).not.toHaveBeenCalled();
    const byId = (r: BridgeMessage) => (r.type === 'anfra:result' ? r.id : 0);
    expect([...replies].sort((a, b2) => byId(a) - byId(b2))).toEqual([
      { type: 'anfra:result', id: 1, ok: true, result: { wrapped: 'from cache' } },
      { type: 'anfra:result', id: 2, ok: true, result: { wrapped: { columns: [], values: [[1]] } } },
      { type: 'anfra:result', id: 3, ok: false, error: { name: 'PermissionError', message: 'Over quota.' } },
    ]);
  });

  it('runs around a plugin\'s calls to another plugin too, which carry that plugin as their caller', async () => {
    const callers: unknown[] = [];
    const audit: Middleware = { name: 'audit', match: 'core.submitQuery', handle: (request, context, next) => { callers.push(context.caller); return next(request); } };
    const agent = definePlugin({
      name: 'agent',
      version: '1',
      requires: ['core'],
      host: { calls: { ask: { frame: true, handler: (_r, context) => context.call('core.submitQuery', query) } } },
    });
    mount([corePlugin(backend()), agent], [audit]);
    call(1, 'agent.ask');
    call(2, 'core.submitQuery', query);
    await settle();
    expect(callers).toHaveLength(2);
    expect(callers).toContainEqual({ kind: 'plugin', plugin: 'agent' });
    expect(callers).toContainEqual({ kind: 'frame' });
    expect(replies.map((r) => r.type === 'anfra:result' && r.ok)).toEqual([true, true]);
  });

  it('may come with a plugin', async () => {
    const seen: string[] = [];
    const logger = definePlugin({
      name: 'logger', version: '1', host: { middleware: [{ name: 'log', match: '*', handle: (r, c, next) => { seen.push(c.name); return next(r); } }] },
    });
    mount([corePlugin(backend()), logger]);
    call(1, 'core.submitQuery', query);
    await settle();
    expect(seen).toEqual(['core.submitQuery']);
  });
});

describe('the host\'s side', () => {
  it('calls a host half, or the frame half when no host half answers', async () => {
    const watch = vi.fn(() => 'ok');
    const panel = definePlugin({ name: 'panel', version: '1', host: { calls: { watch } } });
    mount([corePlugin(backend()), panel]);
    await expect(mounted!.call('panel.watch', true)).resolves.toBe('ok');
    expect(watch).toHaveBeenCalledWith(true, expect.objectContaining({ caller: { kind: 'host' }, mount: expect.anything() }));

    const drawn = mounted!.call('panel.draw', { block: 'a' });
    expect(replies).toEqual([{ type: 'anfra:call', id: 1, name: 'panel.draw', request: { block: 'a' } }]);
    fromFrame({ type: 'anfra:result', id: 1, ok: false, error: { name: 'ValidationError', message: 'No block a.' } });
    await expect(drawn).rejects.toBeInstanceOf(ValidationError);
  });

  it('calls with no frame on the host itself', async () => {
    const host = createHost({ plugins: [corePlugin(backend())] });
    await expect(host.call('core.submitQuery', query)).resolves.toEqual({ columns: [], values: [[1]] });
    await expect(host.call('core.nothing')).rejects.toBeInstanceOf(TransportError);
  });

  it('sends the frame events, hears its events, and its readiness, until unmounted', () => {
    const ready = vi.fn();
    const plugin = definePlugin({ name: 'p', version: '1', host: { mount: (m) => m.onReady(ready) } });
    mount([corePlugin(backend()), plugin]);
    const heard = vi.fn();
    mounted!.on('p.changed', heard);
    mounted!.emit('p.watch', true);
    expect(replies).toEqual([{ type: 'anfra:event', name: 'p.watch', payload: true }]);
    fromFrame({ type: 'anfra:event', name: 'p.changed', payload: { n: 1 } });
    fromFrame({ type: 'anfra:ready' });
    expect(heard).toHaveBeenCalledWith({ n: 1 });
    expect(ready).toHaveBeenCalledTimes(1);
  });

  it('rejects a call into the frame still running when it unmounts, and disposes each plugin\'s mount', async () => {
    const dispose = vi.fn();
    mount([corePlugin(backend()), definePlugin({ name: 'p', version: '1', host: { mount: () => dispose } })]);
    const pending = mounted!.call('p.draw');
    mounted!.unmount();
    mounted = undefined;
    await expect(pending).rejects.toThrow(/unmounted/);
    expect(dispose).toHaveBeenCalledTimes(1);
  });
});
