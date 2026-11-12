import { describe, expect, it } from 'vitest';
import { flush, testFrame } from './testFrame';
import { OVERLAY_ID } from './overlay';
import { layerRoot } from './layer';

function frame () {
  document.body.innerHTML = '<section data-anfra-block="trend">chart</section>';
  return testFrame();
}

const wait = () => new Promise((resolve) => { setTimeout(resolve, 80); });

describe('inspection, the frame\'s side of the panel', () => {
  it('posts nothing until the host watches, then a snapshot of the apps and the structure', async () => {
    const f = frame();
    f.anfra.createApp({ title: 'Sales' });
    await wait();
    expect(f.events).toEqual([]);

    f.emit('anfra.watch', true);
    await flush();
    expect(f.events[0]).toMatchObject({
      name: 'anfra.snapshot',
      payload: { apps: [{ title: 'Sales' }], structure: { nodes: [{ kind: 'block', id: 'trend' }] } },
    });
  });

  it('posts again when an app changes, and stops when the host stops watching', async () => {
    const f = frame();
    f.emit('anfra.watch', true);
    await flush();
    const app = f.anfra.createApp();
    await wait();
    expect(f.events).toHaveLength(2);

    f.emit('anfra.watch', false);
    await flush();
    app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
    await wait();
    expect(f.events).toHaveLength(2);
  });

  it('highlights a node in the plugin layer, never in the page', async () => {
    const f = frame();
    f.emit('anfra.watch', true);
    await flush();
    const key = (f.events[0].payload as { structure: { nodes: { key: string }[] } }).structure.nodes[0].key;

    f.emit('anfra.highlight', key);
    await flush();

    expect(document.getElementById(OVERLAY_ID)).toBeNull();
    const overlay = layerRoot(document, 'anfra').getElementById(OVERLAY_ID) as HTMLElement;
    expect(overlay.style.display).toBe('block');
    f.emit('anfra.highlight', null);
    await flush();
    expect(overlay.style.display).toBe('none');
  });

  it('reports a picked node to the host, and Escape as none', async () => {
    const f = frame();
    f.emit('anfra.watch', true);
    f.emit('anfra.pick', true);
    await flush();

    document.querySelector('[data-anfra-block]')!.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await flush();

    expect(f.events.filter((e) => e.name === 'anfra.picked').map((e) => e.payload)).toEqual([{ node: expect.any(String) }, { node: null }]);
  });

  it('gives the plugin layer and Anfra.theme the host\'s theme', async () => {
    const f = frame();
    const heard: string[] = [];
    f.anfra.theme.subscribe((theme: { scheme: string }) => heard.push(theme.scheme));
    expect(f.anfra.theme.scheme).toBe('light');
    f.emit('anfra.theme', { scheme: 'dark', tokens: { 'color-accent': '#ff0000' } });
    await flush();
    expect(f.anfra.theme.tokens['color-accent']).toBe('#ff0000');
    expect(heard).toEqual(['dark']);
    layerRoot(document, 'notes');
    const layer = document.querySelector<HTMLElement>('[data-anfra-layer]')!;
    expect(layer.style.getPropertyValue('--anfra-color-accent')).toBe('#ff0000');
  });

  it('forwards the host\'s shortcuts while focus is in the frame, unless the page handled the key', async () => {
    const f = frame();
    f.emit('anfra.shortcuts', ['alt+shift+i']);
    await flush();
    const press = (handled: boolean) => {
      const event = new KeyboardEvent('keydown', { code: 'KeyI', key: 'ˆ', altKey: true, shiftKey: true, cancelable: true });
      if (handled) event.preventDefault();
      window.dispatchEvent(event);
      return event;
    };
    expect(press(false).defaultPrevented).toBe(true);
    press(true);
    window.dispatchEvent(new KeyboardEvent('keydown', { code: 'KeyX', key: 'x' }));
    await flush();
    expect(f.events.filter((e) => e.name === 'anfra.shortcut')).toEqual([{ name: 'anfra.shortcut', payload: 'alt+shift+i' }]);
  });
});
