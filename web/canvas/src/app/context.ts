import { inject, provide, type InjectionKey } from 'vue';
import type { Store } from './store.ts';

const KEY: InjectionKey<Store> = Symbol('anfra-canvas');

export const provideStore = (store: Store) => provide(KEY, store);

export function useStore (): Store {
  const store = inject(KEY);
  if (!store) throw new Error('useStore() outside the canvas');
  return store;
}

/** A comment's number on the canvas: its place among the board's comments, from 1. */
export const pinNumber = (store: Store, id: string) => (store.board.value?.comments.findIndex((c) => c.id === id) ?? -1) + 1;

/** Whether the app has scenarios. An app without them (no mocks) shows its screens as they are,
 *  and the canvas says nothing about scenarios. */
export const hasScenarios = (store: Store) => Object.keys(store.state.settings.scenarios).length > 0;

/** The scenarios a frame can be answered by, as choices: the app's, then the real backend. */
export const scenarioOptions = (store: Store) => [
  ...Object.entries(store.state.settings.scenarios).map(([value, sc]) => ({ value, label: sc.label ?? value, description: sc.description })),
  { value: 'live', label: 'Live backend', description: 'No mocks: the API answers from the server the dev proxy points at' },
];
