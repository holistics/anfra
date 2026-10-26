import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { salesDataset, testUser } from '../app/testSupport';
import { corePlugin } from '../plugins/core';
import { definePlugin, type CallHandler, type Plugin } from './plugin';
import { createHost, type HostOptions, type MountedDataApp } from './runtime';

let mounted: MountedDataApp | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.restoreAllMocks();
});

const core = () => corePlugin({ submitQuery: vi.fn(), fieldSuggestions: vi.fn() });

const notes = (handler: CallHandler = vi.fn(() => 'saved')) => definePlugin({
  name: 'notes',
  version: '1',
  host: {
    calls: { save: handler },
    commands: [
      { id: 'save', title: 'Save notes', run: { call: 'notes.save', request: { all: true } } },
      { id: 'open', title: 'Notes', icon: 'note', run: { tab: 'notes.list' } },
    ],
    tabs: [
      { id: 'list', title: 'Notes', order: 2, mount: (root) => { root.innerHTML = '<p>notes</p>'; } },
      { id: 'first', title: 'First', order: 1, mount: () => {} },
    ],
  },
});

function host (plugins: Plugin[], options: Partial<HostOptions> = {}) {
  const runtime = createHost({ plugins: [core(), ...plugins], frameScript: 'x', ...options });
  mounted = runtime.mount({
    container: document.body, definition: '', datasets: { sales: salesDataset }, user: testUser,
  });
  vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation(() => {});
  return { runtime, mounted };
}

describe('contributions', () => {
  it('lists the plugins\' commands and tabs under their names, then the host\'s own, tabs in order', () => {
    const { runtime } = host([notes()], {
      commands: [{ id: 'comment', title: 'Comment', run: { call: 'notes.save' } }],
      tabs: [{ id: 'host-log', title: 'Log', mount: () => {} }],
    });
    expect(runtime.commands.map((c) => [c.id, c.plugin])).toEqual([['notes.save', 'notes'], ['notes.open', 'notes'], ['comment', undefined]]);
    expect(runtime.tabs.map((t) => t.id)).toEqual(['host-log', 'notes.first', 'notes.list']);
  });

  it('refuses two contributions with one id', () => {
    expect(() => host([notes()], { commands: [{ id: 'notes.save', title: 'x', run: { call: 'notes.save' } }] })).toThrow("Two contributions are named 'notes.save'.");
  });

  it('leaves out what the filter refuses, everywhere, while the plugin\'s calls stay', async () => {
    const save = vi.fn(() => 'saved');
    const { runtime, mounted: app } = host([notes(save)], { contributions: (c) => c.kind === 'command' });
    expect(runtime.tabs).toEqual([]);
    app.panel.show('notes.list');
    expect(app.panel.tab).toBeUndefined();
    await expect(app.call('notes.save')).resolves.toBe('saved');
  });

  it('runs a command\'s call as its plugin, started by the host', async () => {
    const save = vi.fn((_request: unknown, context: { caller: unknown, origin: unknown }) => [context.caller, context.origin]);
    const { mounted: app } = host([notes(save)]);
    await expect(app.run('notes.save')).resolves.toEqual([{ kind: 'plugin', plugin: 'notes' }, { kind: 'host' }]);
    expect(save).toHaveBeenCalledWith({ all: true }, expect.anything());
  });

  it('toggles the panel at a command\'s tab, pressed while one of its plugin\'s tabs shows', async () => {
    const { mounted: app } = host([notes()]);
    const changes = vi.fn();
    app.onCommandsChange(changes);
    expect(app.commandState('notes.open')).toEqual({ pressed: false });
    await app.run('notes.open');
    expect(app.panel).toMatchObject({ open: true, tab: 'notes.list' });
    expect(app.commandState('notes.open')).toEqual({ pressed: true });
    expect(changes).toHaveBeenCalled();
    await app.run('notes.open');
    expect(app.panel.open).toBe(false);
  });

  it('reads a command\'s own state, again when its plugin says it changed, and refuses it disabled', async () => {
    let disabled: string | undefined = 'Disabled by admin';
    const plugin = definePlugin({
      name: 'ask',
      version: '1',
      host: {
        calls: { start: () => 'started' },
        commands: [{ id: 'start', title: 'Ask AI', run: { call: 'ask.start' }, state: () => ({ disabled }) }],
        mount: (mount) => { setTimeout(() => { disabled = undefined; mount.commandsChanged(); }, 0); },
      },
    });
    const { mounted: app } = host([plugin]);
    await expect(app.run('ask.start')).rejects.toThrow('Disabled by admin');
    const changed = new Promise<void>((resolve) => { app.onCommandsChange(resolve); });
    await changed;
    await expect(app.run('ask.start')).resolves.toBe('started');
  });

  it('draws a tab in the element\'s shadow root, with calls about the mount, and takes it down', async () => {
    const down = vi.fn();
    const save = vi.fn(() => 'saved');
    const plugin = definePlugin({
      name: 'notes',
      version: '1',
      host: {
        calls: { save },
        tabs: [{
          id: 'list',
          title: 'Notes',
          mount: async (root, context) => {
            root.innerHTML = `<p>${context.tab}</p>`;
            await context.call('notes.save');
            return down;
          },
        }],
      },
    });
    const { mounted: app } = host([plugin]);
    const element = document.createElement('div');
    const unmount = app.panel.mountTab('notes.list', element);
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    expect(element.shadowRoot?.innerHTML).toBe('<p>notes.list</p>');
    expect(save).toHaveBeenCalled();
    unmount();
    expect(down).toHaveBeenCalled();
    expect(element.shadowRoot?.innerHTML).toBe('');
  });
});

describe('the theme', () => {
  it('sets its tokens on a tab\'s element, and follows each change', () => {
    const plugin = definePlugin({
      name: 'notes',
      version: '1',
      host: { tabs: [{ id: 'list', title: 'Notes', mount: () => {} }] },
    });
    const { runtime, mounted: app } = host([plugin], { theme: { scheme: 'dark', tokens: { 'color-bg': '#101010' } } });
    const element = document.createElement('div');
    const unmount = app.panel.mountTab('notes.list', element);
    expect(element.style.getPropertyValue('--anfra-color-bg')).toBe('#101010');
    expect(element.style.getPropertyValue('color-scheme')).toBe('dark');
    runtime.setTheme({ scheme: 'light' });
    expect(runtime.theme.scheme).toBe('light');
    expect(element.style.getPropertyValue('--anfra-color-bg')).toBe('#ffffff');
    unmount();
    runtime.setTheme({ scheme: 'dark' });
    expect(element.style.getPropertyValue('--anfra-color-bg')).toBe('#ffffff');
  });

  it('sends the frame its theme each time it is ready, and on each change', () => {
    const runtime = createHost({ plugins: [core()], frameScript: 'x', theme: { scheme: 'dark' } });
    mounted = runtime.mount({
      container: document.body, definition: '', datasets: { sales: salesDataset }, user: testUser,
    });
    const posted: unknown[] = [];
    vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { posted.push(message); });
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'anfra:ready' }, source: mounted.frame.contentWindow }));
    runtime.setTheme({ scheme: 'light' });
    expect(posted).toEqual([
      { type: 'anfra:event', name: 'anfra.theme', payload: expect.objectContaining({ scheme: 'dark' }) },
      { type: 'anfra:event', name: 'anfra.theme', payload: expect.objectContaining({ scheme: 'light' }) },
    ]);
  });
});
