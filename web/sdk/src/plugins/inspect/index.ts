/**
 * `inspect`: what the host's inspect panel needs of a frame. While the host watches, the frame posts
 * a snapshot of its apps and its structure (`inspect.snapshot`) on each change; the host can
 * highlight a node, locate a node or an entity, and turn pick mode on, where a click reports a node
 * (`inspect.picked`). Calls the host page makes, never the frame.
 */
import type { LocateTarget } from '../../common/types';
import { definePlugin, type MountContext, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import setup from 'anfra-sdk:setup/inspect';

export function inspectPlugin (): Plugin {
  const watching = new WeakMap<MountContext, boolean>();
  const frameOf = (mount: MountContext | undefined): MountContext => {
    if (!mount) throw new Error('inspect: call it on a mounted Data App.');
    return mount;
  };
  return definePlugin({
    name: 'inspect',
    version: PLUGIN_VERSION,
    host: {
      mount (mount) {
        // The frame forgets on each load of its document; tell it again.
        return mount.onReady(() => {
          if (watching.get(mount)) mount.emit('inspect.watch', true);
        });
      },
      calls: {
        /** Start or stop the snapshots; survives the frame reloading. */
        watch (open: boolean, { mount }) {
          const frame = frameOf(mount);
          watching.set(frame, open === true);
          frame.emit('inspect.watch', open === true);
          return null;
        },
        /** Show a node of the last snapshot's structure on the page, or nothing (null). */
        highlight (node: string | null, { mount }) {
          frameOf(mount).emit('inspect.highlight', node);
          return null;
        },
        /** Scroll a node, or an entity wherever it is drawn, into view, and hold the highlight on it. */
        locate (target: LocateTarget, { mount }) {
          frameOf(mount).emit('inspect.locate', target);
          return null;
        },
        /** Start or stop pick mode: pointing outlines marked elements, a click reports one. */
        pick (on: boolean, { mount }) {
          frameOf(mount).emit('inspect.pick', on === true);
          return null;
        },
      },
    },
    frame: { setup },
  });
}
