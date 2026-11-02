import type { Listener, Unsubscribe } from '../common/types';
import { reportError } from './reporting';

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
    this.listeners.forEach((listener) => {
      // A listener is the app's own code, drawing what changed. Its bug must break only its own
      // drawing: thrown on, it would stop the other listeners and fail the query that changed.
      try {
        listener();
      } catch (err) {
        // eslint-disable-next-line no-console -- the author's bug, shown where they debug
        console.error('anfra: a subscribe() listener threw; the queries carry on.', err);
        reportError('listener', err);
      }
    });
    this.parent?.notifyChange();
  }

  protected notify (): void {
    this.notifyChange();
  }
}
