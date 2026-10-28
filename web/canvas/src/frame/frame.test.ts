// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest';
import { installFrame } from './index.ts';
import { frameName, type CanvasHost } from './protocol.ts';

/** A window that looks like a canvas frame: named by the canvas, with a parent that hosts it. */
function frameWindow (name: string) {
  const host: CanvasHost = { attach: vi.fn(), detach: vi.fn(), event: vi.fn() };
  const win = window;
  win.name = name;
  Object.defineProperty(win, 'parent', { value: { __anfraCanvasHost: host }, configurable: true });
  win.fetch = vi.fn(async () => new Response('real'));
  return { win, host };
}

describe('installFrame', () => {
  it('does nothing outside a canvas frame', () => {
    window.name = '';
    expect(installFrame({ scenarios: {} })).toBeUndefined();
  });

  it('drops Vue\'s warnings about the source tags only', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    window.name = '';
    installFrame({ scenarios: {} });
    console.warn('[Vue warn]: Extraneous non-props attributes (data-canvas-src, data-canvas-cmp) were passed to component');
    console.warn('[Vue warn]: Extraneous non-props attributes (class, data-canvas-src) were passed to component');
    expect(warn).toHaveBeenCalledTimes(1);
    expect(warn.mock.calls[0][0]).toContain('(class, data-canvas-src)');
    warn.mockRestore();
  });

  it('answers the API from the scenario, attaches to the canvas, and picks elements', async () => {
    const { win, host } = frameWindow(frameName('fr_1', 'empty'));
    const agent = installFrame({
      scenarios: { default: { ops: { 'sessions.get': { user: { id: 'u1' } } } }, empty: { ops: { 'apps.list': { apps: [] } } } },
    }, win)!;
    expect(host.attach).toHaveBeenCalledWith('fr_1', agent);

    const apps = await win.fetch('/api/o/acme/apps.list', { method: 'POST', body: '{}' });
    expect(await apps.json()).toEqual({ apps: [] });
    expect(await (await win.fetch('/api/sessions.get', { method: 'POST' })).json()).toEqual({ user: { id: 'u1' } });
    expect((await win.fetch('/api/jobs.list', { method: 'POST' })).status).toBe(501);
    expect(await (await win.fetch('/assets/logo.svg')).text()).toBe('real');

    document.body.innerHTML = `
      <main data-canvas-src="src/pages/P.vue:2:3">
        <button data-canvas-src="src/pages/P.vue:4:5" data-canvas-cmp="Button"><span data-canvas-src="src/components/ui/button/Button.vue:9:3">Invite</span></button>
      </main>`;
    agent.setMode('inspect');
    const span = document.querySelector('span')!;
    span.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
    const pick = (host.event as ReturnType<typeof vi.fn>).mock.calls.find(([, e]) => e.type === 'pick')![1];
    expect(pick.info.ref).toMatchObject({ frame: 'fr_1', tag: 'button', component: 'Button', source: 'src/pages/P.vue:4:5', text: 'Invite' });
    expect(agent.locate(pick.info.ref)?.ref.source).toBe('src/pages/P.vue:4:5');

    // A component that gives its attributes to an inner element (HTextInput, to its <input>) is
    // picked as its root, on the inner element or anywhere else in it.
    document.body.innerHTML = `
      <main data-canvas-src="src/pages/P.vue:2:3">
        <div class="field"><svg></svg><input data-canvas-src="src/pages/P.vue:6:5" data-canvas-cmp="HTextInput"></div>
      </main>`;
    const field = document.querySelector('.field')!;
    const input = { vnode: { el: field, props: { 'data-canvas-src': 'src/pages/P.vue:6:5', 'data-canvas-cmp': 'HTextInput' } }, parent: null };
    for (const el of [field, ...field.children]) Object.assign(el, { __vueParentComponent: input });
    const picks = () => (host.event as ReturnType<typeof vi.fn>).mock.calls.filter(([, e]) => e.type === 'pick').map(([, e]) => e.info.ref);
    for (const el of [document.querySelector('input')!, document.querySelector('svg')!]) {
      el.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }));
      expect(picks().at(-1)).toMatchObject({ tag: 'div', component: 'HTextInput', source: 'src/pages/P.vue:6:5' });
    }
    expect(agent.locate(picks().at(-1))?.ref).toMatchObject({ tag: 'div', component: 'HTextInput' });
    expect(agent.tree()[0].children[0]).toMatchObject({ label: 'HTextInput', ref: { tag: 'div', source: 'src/pages/P.vue:6:5' } });
  });
});
