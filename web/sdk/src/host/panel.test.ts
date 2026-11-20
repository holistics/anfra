import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { BridgeMessage } from '../common/bridge';
import { salesDataset, testUser } from '../app/testSupport';
import { corePlugin } from '../plugins/core';
import { createHost, type MountedDataApp } from './runtime';

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

function mount () {
  const backend = { submitQuery: vi.fn(), fieldSuggestions: vi.fn() };
  const host = createHost({ plugins: [corePlugin(backend)], frameScript: 'x' });
  mounted = host.mount({
    container: document.body, definition: '', datasets: { sales: salesDataset }, user: testUser,
  });
  // The panel's messages only: a ready frame is sent the theme too.
  vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => {
    const reply = message as BridgeMessage;
    if (reply.type !== 'anfra:event' || reply.name !== 'anfra.theme') replies.push(reply);
  });
  return mounted.panel;
}

function fromFrame (message: BridgeMessage) {
  window.dispatchEvent(new MessageEvent('message', { data: message, source: mounted!.frame.contentWindow }));
}

const event = (name: string, payload: unknown): BridgeMessage => ({ type: 'anfra:event', name, payload });

describe('the panel', () => {
  it('watches the frame while open, and forgets its snapshot and selection when closed', () => {
    const panel = mount();
    const changes = vi.fn();
    panel.subscribe(changes);
    panel.show(undefined, '0.1');
    expect(panel.open).toBe(true);
    expect(panel.selection).toBe('0.1');
    fromFrame(event('anfra.snapshot', { apps: [] }));
    expect(panel.snapshot).toEqual({ apps: [], structure: { nodes: [], usage: {} } });
    panel.hide();
    expect(panel).toMatchObject({ open: false, selection: null, snapshot: undefined });
    expect(replies).toEqual([
      event('anfra.watch', true),
      event('anfra.locate', { node: '0.1' }),
      event('anfra.watch', false),
      event('anfra.highlight', null),
    ]);
    expect(changes).toHaveBeenCalledTimes(3);
  });

  it('tells a reloaded frame to keep watching and picking', () => {
    const panel = mount();
    fromFrame({ type: 'anfra:ready' });
    expect(replies).toEqual([]);
    panel.pick(true);
    replies.length = 0;
    fromFrame({ type: 'anfra:ready' });
    expect(replies).toEqual([event('anfra.watch', true), event('anfra.pick', true)]);
  });

  it('selects what the reader picks, and ends pick mode either way', () => {
    const panel = mount();
    panel.pick(true);
    fromFrame(event('anfra.picked', { node: '0.2' }));
    expect(panel).toMatchObject({ selection: '0.2', picking: false });
    panel.pick(true);
    fromFrame(event('anfra.picked', { node: null }));
    expect(panel).toMatchObject({ selection: '0.2', picking: false });
    expect(replies.filter((reply) => reply.type === 'anfra:event' && reply.name === 'anfra.pick')).toEqual([
      event('anfra.pick', true), event('anfra.pick', false), event('anfra.pick', true), event('anfra.pick', false),
    ]);
  });

  it('takes only snapshots and picks of their shape, and only while open', () => {
    const panel = mount();
    fromFrame(event('anfra.snapshot', { apps: [] }));
    fromFrame(event('anfra.picked', { node: '0' }));
    expect(panel).toMatchObject({ snapshot: undefined, selection: null });
    panel.show();
    const node = (children: unknown[] = []) => ({
      key: '0', kind: 'block', problems: [], children,
    });
    for (const payload of [
      null, 'x', { apps: 'x' }, { apps: [1] }, { apps: [], structure: { nodes: {} } },
      { apps: [], structure: { nodes: [1] } }, { apps: [], structure: { nodes: [{ key: 'a' }] } },
      { apps: [], structure: { nodes: [node([node(), { ...node(), problems: null }])] } },
    ]) {
      fromFrame(event('anfra.snapshot', payload));
    }
    for (const payload of [null, { node: 3 }, {}]) fromFrame(event('anfra.picked', payload));
    expect(panel).toMatchObject({ snapshot: undefined, selection: null });
  });

  it('toggles: open, then closed when asked again', () => {
    const panel = mount();
    panel.toggle();
    expect(panel.open).toBe(true);
    panel.toggle();
    expect(panel.open).toBe(false);
  });
  describe('reveal', () => {
    const marked = {
      apps: [],
      structure: {
        nodes: [{
          key: '0', kind: 'block', problems: [], children: [
            { key: '0.0', kind: 'query', name: 'revenue', app: 0, problems: [], children: [] },
          ],
        }],
        usage: {},
      },
    };

    it('keeps a one-shot reveal, with a new nonce for each, and never closes the panel', () => {
      const panel = mount();
      panel.show(undefined, { entity: 'query', app: 0, name: 'revenue' });
      expect(panel.open).toBe(true);
      const first = panel.revealed!;
      expect(first).toMatchObject({ entity: 'query', app: 0, name: 'revenue' });
      panel.show(undefined, { entity: 'query', app: 0, name: 'revenue' });
      expect(panel.open).toBe(true);
      expect(panel.revealed!.nonce).toBeGreaterThan(first.nonce);
      panel.hide();
      expect(panel.revealed).toBeUndefined();
    });

    it('selects a marked entity it reveals, once a snapshot has it', () => {
      const panel = mount();
      panel.show(undefined, { entity: 'query', app: 0, name: 'revenue' });
      expect(panel.selection).toBeNull();
      fromFrame(event('anfra.snapshot', { apps: [] }));
      expect(panel.selection).toBeNull();
      fromFrame(event('anfra.snapshot', marked));
      expect(panel.selection).toBe('0.0');
      // Selected once: the reader may select something else.
      panel.select('0');
      fromFrame(event('anfra.snapshot', marked));
      expect(panel.selection).toBe('0');
      // Already in the snapshot: selected at once.
      panel.show(undefined, { entity: 'query', app: 0, name: 'revenue' });
      expect(panel.selection).toBe('0.0');
    });

    it('lets a selection made after it win over the reveal still waiting', () => {
      const panel = mount();
      panel.show(undefined, { entity: 'query', app: 0, name: 'revenue' });
      panel.select('0');
      fromFrame(event('anfra.snapshot', marked));
      expect(panel.selection).toBe('0');
    });
  });
});
