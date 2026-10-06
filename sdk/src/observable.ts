import type { Listener, Unsubscribe } from './types';

interface NotificationTarget {
  notifyChange (): void;
}

/**
 * The smallest thing that adapts to every framework: `subscribe(listener) => unsubscribe` is the
 * signature React's `useSyncExternalStore` wants, and a Vue adapter is a `shallowRef` plus a
 * `triggerRef`. Deliberately not named events (a taxonomy to version) nor a signal primitive
 * (owning a reactivity library). See docs/adr/0002.
 */
export class Observable implements NotificationTarget {
  private readonly listeners = new Set<Listener>();

  private readonly parent?: NotificationTarget;

  constructor (parent?: NotificationTarget) {
    this.parent = parent;
  }

  subscribe (listener: Listener): Unsubscribe {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  }

  /**
   * Public only so a child observable can bubble into its parent — an app subscriber sees every
   * entity's changes through one subscription. Subclasses call the protected `notify` instead.
   */
  notifyChange (): void {
    this.listeners.forEach((listener) => listener());
    this.parent?.notifyChange();
  }

  protected notify (): void {
    this.notifyChange();
  }
}
