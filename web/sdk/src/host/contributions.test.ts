import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { salesDataset, testUser } from '../app/testSupport';
import { corePlugin } from '../plugins/core';
import { definePlugin, type CallDefinition, type CallHandler, type Plugin } from './plugin';
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

  it('closes the panel when pressed, whichever of its plugin\'s tabs shows', async () => {
    const { mounted: app } = host([notes()]);
    app.panel.show('notes.first');
    expect(app.commandState('notes.open')).toEqual({ pressed: true });
    await app.run('notes.open');
    expect(app.panel.open).toBe(false);
  });

  it('opens the panel at its first tab when pick mode starts it', () => {
    const { mounted: app } = host([notes()]);
    app.panel.pick(true);
    expect(app.panel).toMatchObject({ open: true, tab: 'notes.first', picking: true });
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

describe('shortcuts', () => {
  const plugin = (save: CallHandler | CallDefinition = () => 'saved') => definePlugin({
    name: 'notes',
    version: '1',
    host: {
      calls: { save },
      commands: [{ id: 'save', title: 'Save', shortcut: 'Alt+Shift+S', run: { call: 'notes.save' } }],
    },
  });

  it('tells a ready frame which to forward, and runs the command it forwards only with a call the frame may make', async () => {
    const save = vi.fn(() => 'saved');
    const runtime = createHost({ plugins: [core(), plugin({ frame: true, handler: save })], frameScript: 'x' });
    mounted = runtime.mount({
      container: document.body, definition: '', datasets: { sales: salesDataset }, user: testUser,
    });
    const posted: { name?: string, payload?: unknown }[] = [];
    vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { posted.push(message as never); });
    const fromFrame = (data: unknown) => window.dispatchEvent(new MessageEvent('message', { data, source: mounted!.frame.contentWindow }));
    fromFrame({ type: 'anfra:ready' });
    expect(posted.find((message) => message.name === 'anfra.shortcuts')?.payload).toEqual(['alt+shift+s']);
    fromFrame({ type: 'anfra:event', name: 'anfra.shortcut', payload: 'alt+shift+s' });
    fromFrame({ type: 'anfra:event', name: 'anfra.shortcut', payload: 'alt+shift+x' });
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    expect(save).toHaveBeenCalledTimes(1);
  });

  it('forwards the host\'s own keys too, and presses each it gets back on the frame element', () => {
    const { mounted: app } = host([plugin()], { keys: ['Meta+K'] });
    const posted: { name?: string, payload?: unknown }[] = [];
    vi.spyOn(app.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { posted.push(message as never); });
    const fromFrame = (data: unknown) => window.dispatchEvent(new MessageEvent('message', { data, source: app.frame.contentWindow }));
    fromFrame({ type: 'anfra:ready' });
    expect(posted.find((message) => message.name === 'anfra.shortcuts')?.payload).toEqual(['alt+shift+s', 'meta+k']);
    const pressed: KeyboardEvent[] = [];
    const onKeydown = (event: KeyboardEvent) => pressed.push(event);
    window.addEventListener('keydown', onKeydown);
    fromFrame({ type: 'anfra:event', name: 'anfra.shortcut', payload: 'meta+k' });
    fromFrame({ type: 'anfra:event', name: 'anfra.shortcut', payload: 'meta+j' });
    window.removeEventListener('keydown', onKeydown);
    expect(pressed).toHaveLength(1);
    expect(pressed[0]).toMatchObject({ key: 'k', code: 'KeyK', metaKey: true, ctrlKey: false, target: app.frame });
  });

  it('refuses a key that isn\'t one, and one a command has', () => {
    expect(() => host([plugin()], { keys: ['Meta'] })).toThrow("Key 'Meta' isn't a key combination.");
    expect(() => host([plugin()], { keys: ['Shift+Alt+S'] })).toThrow("Key 'Shift+Alt+S' is the shortcut of command 'notes.save'.");
  });

  it('refuses a forwarded key whose command\'s call is the host\'s alone, which a key on the host\'s page runs', async () => {
    const save = vi.fn(() => 'saved');
    const { mounted: app } = host([plugin(save)]);
    window.dispatchEvent(new MessageEvent('message', {
      data: { type: 'anfra:event', name: 'anfra.shortcut', payload: 'alt+shift+s' }, source: app.frame.contentWindow,
    }));
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    expect(save).not.toHaveBeenCalled();
    app.handleKeydown(new KeyboardEvent('keydown', { code: 'KeyS', altKey: true, shiftKey: true }));
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    expect(save).toHaveBeenCalledTimes(1);
  });

  it('leaves a key alone while the reader types', () => {
    const { mounted: app } = host([plugin()]);
    const input = document.createElement('input');
    document.body.append(input);
    const event = new KeyboardEvent('keydown', { code: 'KeyS', altKey: true, shiftKey: true, cancelable: true });
    Object.defineProperty(event, 'target', { value: input });
    expect(app.handleKeydown(event)).toBe(false);
    expect(event.defaultPrevented).toBe(false);
    input.remove();
  });

  it('refuses a shortcut that isn\'t one, two commands with one, and one on a menu command', () => {
    const command = (id: string, shortcut: string, target?: 'node') => ({ id, title: id, shortcut, run: { call: 'notes.save' }, ...(target ? { target } : {}) });
    expect(() => host([plugin()], { commands: [command('x', 'Alt+Shift')] })).toThrow("Command 'x' has a shortcut that isn't one: 'Alt+Shift'.");
    expect(() => host([plugin()], { commands: [command('x', 'shift+alt+s')] })).toThrow("Commands 'notes.save' and 'x' have the same shortcut, 'shift+alt+s'.");
    expect(() => host([plugin()], { commands: [command('x', 'Alt+X', 'node')] })).toThrow("Command 'x' is for the menu on the page, which has no shortcuts.");
  });

  it('matches a key on the host\'s page by its physical key, and leaves a handled one alone', () => {
    const { mounted: app } = host([plugin()]);
    const key = (init: KeyboardEventInit) => new KeyboardEvent('keydown', { cancelable: true, ...init });
    const event = key({ code: 'KeyS', key: 'Í', altKey: true, shiftKey: true });
    expect(app.handleKeydown(event)).toBe(true);
    expect(event.defaultPrevented).toBe(true);
    expect(app.handleKeydown(key({ code: 'KeyS', key: 's', altKey: true }))).toBe(false);
    const handled = key({ code: 'KeyS', key: 's', altKey: true, shiftKey: true });
    handled.preventDefault();
    expect(app.handleKeydown(handled)).toBe(false);
  });
});

describe('menu commands', () => {
  const node = {
    kind: 'node', node: { key: '0', kind: 'block', id: 'trend', problems: [], children: [] }, ancestors: [], rect: { x: 0, y: 0, width: 1, height: 1 },
  };
  const ask = (handler: CallHandler = vi.fn(() => 'asked')) => definePlugin({
    name: 'ask',
    version: '1',
    host: {
      calls: { ask: handler },
      commands: [
        { id: 'ask', title: 'Ask AI', target: 'node', run: { call: 'ask.ask', request: { tone: 'short' } } },
        {
          id: 'share', title: 'Share', target: 'node', run: { call: 'ask.ask' }, state: () => ({ disabled: 'Disabled by admin' }),
        },
        {
          id: 'trends', title: 'Only trends', target: 'node', run: { call: 'ask.ask' }, when: (target) => target.kind === 'node' && target.node.id === 'other',
        },
      ],
    },
  });

  function mountWith (plugin: Plugin) {
    const runtime = createHost({ plugins: [core(), plugin], frameScript: 'x', uiBody: 'ui' });
    mounted = runtime.mount({
      container: document.body, definition: '', datasets: { sales: salesDataset }, user: testUser,
    });
    const posted: { type: string, name?: string, id?: number, payload?: unknown, ok?: boolean, result?: unknown, error?: unknown }[] = [];
    vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { posted.push(message as never); });
    const fromFrame = (data: unknown) => window.dispatchEvent(new MessageEvent('message', { data, source: mounted!.frame.contentWindow }));
    return { posted, fromFrame };
  }

  it('tells a ready frame which targets it has commands for', () => {
    const { posted, fromFrame } = mountWith(ask());
    fromFrame({ type: 'anfra:ready' });
    expect(posted).toContainEqual({ type: 'anfra:event', name: 'anfra.menu', payload: { kinds: ['node'] } });
  });

  it('answers the frame\'s menu for a target: what applies, and why one can\'t run', async () => {
    const { posted, fromFrame } = mountWith(ask());
    fromFrame({ type: 'anfra:call', id: 1, name: 'anfra.menu', request: node });
    fromFrame({ type: 'anfra:call', id: 2, name: 'anfra.menu', request: { kind: 'node', node: 'x' } });
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    expect(posted.find((m) => m.id === 1)?.result).toEqual([{ id: 'ask.ask', label: 'Ask AI' }, { id: 'ask.share', label: 'Share', disabled: 'Disabled by admin' }]);
    expect(posted.find((m) => m.id === 2)).toMatchObject({ ok: false, error: { name: 'ValidationError' } });
  });

  it('runs the one the reader picked with its target, started by the frame, and refuses a disabled one', async () => {
    const handler = vi.fn((_request: unknown, context: { chain: unknown }) => context.chain);
    const { posted, fromFrame } = mountWith(ask(handler));
    fromFrame({ type: 'anfra:call', id: 1, name: 'anfra.run', request: { id: 'ask.ask', target: node } });
    fromFrame({ type: 'anfra:call', id: 2, name: 'anfra.run', request: { id: 'ask.share', target: node } });
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    expect(handler).toHaveBeenCalledWith({ tone: 'short', target: node }, expect.anything());
    expect(posted.find((m) => m.id === 1)?.result).toEqual([{ kind: 'frame' }, { kind: 'host' }, { kind: 'plugin', plugin: 'ask' }]);
    expect(posted.find((m) => m.id === 2)).toMatchObject({ ok: false, error: { name: 'PermissionError', message: 'Disabled by admin' } });
  });

  it('runs only menu commands from the frame, and a menu command only with its target', async () => {
    const { posted, fromFrame } = mountWith(notes());
    fromFrame({ type: 'anfra:call', id: 1, name: 'anfra.run', request: { id: 'notes.save', target: node } });
    await new Promise((resolve) => { setTimeout(resolve, 0); });
    expect(posted.find((m) => m.id === 1)).toMatchObject({ ok: false, error: { name: 'PermissionError' } });
    mounted!.unmount();
    mountWith(ask());
    await expect(mounted!.run('ask.ask')).rejects.toThrow('runs from the menu on the page');
  });
});
