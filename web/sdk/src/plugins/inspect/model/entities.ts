/**
 * A query's or a control's key, as the panels keep what they show of one (`query:<app>/<name>`,
 * `control:<app>/<name>`), and whether the app still has it.
 */
import type { InspectedApp } from '../../../common/types';

type Kind = 'query' | 'control';

/** Whether the app at index `app` has a query, or a control, by this name. */
export function hasEntity (apps: readonly InspectedApp[], kind: Kind, app: number, name: string): boolean {
  return Object.hasOwn((kind === 'query' ? apps[app]?.queries : apps[app]?.controls) ?? {}, name);
}

const ENTITY_KEY = /^(query|control):(\d+)\/(.+)$/;

/**
 * Whether `key` names a query or a control its app no longer has. False for a key that names
 * neither, which isn't an entity's to drop.
 */
export function entityGone (key: string | undefined, apps: readonly InspectedApp[]): boolean {
  const match = key ? ENTITY_KEY.exec(key) : null;
  if (!match) return false;
  return !hasEntity(apps, match[1] as Kind, Number(match[2]), match[3]!);
}
