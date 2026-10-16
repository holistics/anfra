/**
 * `inspect`: what the host's inspect panel needs of a frame. While the host watches, the frame posts
 * a snapshot of its apps and its structure (`inspect.snapshot`) on each change; the host can
 * highlight a node, locate a node or an entity, and turn pick mode on, where a click reports a node
 * (`inspect.picked`). Calls the host page makes, never the frame.
 */
import type { InspectedApp, InspectedStructure, LocateTarget } from '../../common/types';
import { definePlugin, type MountContext, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import setup from 'anfra-sdk:setup/inspect';

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

/**
 * A snapshot as the frame posts it, or undefined when it isn't one: the frame is the definition's
 * realm, so a panel gets only something of the snapshot's shape, never what a script made up.
 */
export function checkSnapshot (payload: unknown): { apps: InspectedApp[], structure: InspectedStructure } | undefined {
  if (!isObject(payload) || !Array.isArray(payload.apps) || !payload.apps.every(isObject)) return undefined;
  const structure = payload.structure ?? { nodes: [], usage: {} };
  if (!isObject(structure) || !Array.isArray(structure.nodes) || !isObject(structure.usage ?? {})) return undefined;
  return {
    apps: payload.apps as unknown as InspectedApp[],
    structure: { nodes: structure.nodes, usage: structure.usage ?? {} } as InspectedStructure,
  };
}

/** A pick as the frame posts it: a node's key, or null for none; undefined when it isn't one. */
export function checkPicked (payload: unknown): { node: string | null } | undefined {
  if (!isObject(payload) || !(typeof payload.node === 'string' || payload.node === null)) return undefined;
  return { node: payload.node };
}

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
      events: {
        /** The frame's apps and structure, while watched. Only while watched, and only of its shape. */
        snapshot: (payload, mount) => (watching.get(mount) ? checkSnapshot(payload) : undefined),
        /** The node the reader picked, or null on leaving pick mode. */
        picked: (payload, mount) => (watching.get(mount) ? checkPicked(payload) : undefined),
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
