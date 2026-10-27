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
