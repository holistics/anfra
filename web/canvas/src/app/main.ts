import { createApp, type Component } from 'vue';
import CanvasApp from './CanvasApp.vue';
import { createStore, type Settings } from './store.ts';
import './canvas.css';

/** Start the canvas in an element of its page. The Vite plugin's page calls it with the app's settings. */
export function mount (el: HTMLElement | null, settings: Settings, previews?: Component) {
  if (!el) throw new Error('the canvas page has no element to mount in');
  const store = createStore(settings, previews);
  createApp(CanvasApp, { store }).mount(el);
  return store;
}
