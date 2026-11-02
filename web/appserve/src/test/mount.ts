// Listeners go in props as `onX`.
import {
  createApp, h, nextTick, type Component,
} from 'vue';

export interface Mounted {
  host: HTMLElement;
  unmount: () => void;
}

const mounted: Mounted[] = [];

export async function mount (component: Component, props: Record<string, unknown> = {}, slot?: () => unknown): Promise<Mounted> {
  const host = document.createElement('div');
  document.body.append(host);
  const app = createApp({ render: () => h(component, props, slot ? { default: slot } : undefined) });
  app.mount(host);
  await nextTick();
  const handle = { host, unmount: () => { app.unmount(); host.remove(); } };
  mounted.push(handle);
  return handle;
}

export function unmountAll (): void {
  mounted.splice(0).forEach((each) => each.unmount());
}
