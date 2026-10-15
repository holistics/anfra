import { describe, expect, it } from 'vitest';
import { flush, testFrame } from '../../app/testFrame';
import { OVERLAY_ID } from '../../app/overlay';
import { layerRoot } from '../../app/layer';
import setupInspect from './frame';

function frame () {
  document.body.innerHTML = '<section data-anfra-block="trend">chart</section>';
  return testFrame({ setups: [['inspect', setupInspect]] });
}

const wait = () => new Promise((resolve) => { setTimeout(resolve, 80); });

describe("inspect's frame half", () => {
  it('posts nothing until the host watches, then a snapshot of the apps and the structure', async () => {
    const f = frame();
    f.anfra.createApp({ title: 'Sales' });
    await wait();
    expect(f.events).toEqual([]);

    f.emit('inspect.watch', true);
    await flush();
    expect(f.events[0]).toMatchObject({
      name: 'inspect.snapshot',
      payload: { apps: [{ title: 'Sales' }], structure: { nodes: [{ kind: 'block', id: 'trend' }] } },
    });
  });

  it('posts again when an app changes, and stops when the host stops watching', async () => {
    const f = frame();
    f.emit('inspect.watch', true);
    await flush();
    const app = f.anfra.createApp();
    await wait();
    expect(f.events).toHaveLength(2);

    f.emit('inspect.watch', false);
    await flush();
    app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
    await wait();
    expect(f.events).toHaveLength(2);
  });

  it('highlights a node in the plugin layer, never in the page', async () => {
    const f = frame();
    f.emit('inspect.watch', true);
    await flush();
    const key = (f.events[0].payload as { structure: { nodes: { key: string }[] } }).structure.nodes[0].key;

    f.emit('inspect.highlight', key);
    await flush();

    expect(document.getElementById(OVERLAY_ID)).toBeNull();
    const overlay = layerRoot(document, 'anfra').getElementById(OVERLAY_ID) as HTMLElement;
    expect(overlay.style.display).toBe('block');
    f.emit('inspect.highlight', null);
    await flush();
    expect(overlay.style.display).toBe('none');
  });

  it('reports a picked node to the host, and Escape as none', async () => {
    const f = frame();
    f.emit('inspect.watch', true);
    f.emit('inspect.pick', true);
    await flush();

    document.querySelector('[data-anfra-block]')!.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await flush();

    expect(f.events.filter((e) => e.name === 'inspect.picked').map((e) => e.payload)).toEqual([{ node: expect.any(String) }, { node: null }]);
  });
});
