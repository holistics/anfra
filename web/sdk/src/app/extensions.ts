/**
 * What plugins add to an SDK's queries and controls (`query.export`): a member named after the
 * plugin, made the first time it is read. Kept per SDK instance and put on each entity as its app
 * creates it, never on a shared prototype.
 */
import { ValidationError } from '../common/errors';

export type Factory<T> = (entity: T) => unknown;

export interface EntityExtensions<Q, C> {
  query: Map<string, Factory<Q>>;
  control: Map<string, Factory<C>>;
}

/** Add a plugin's member to `extensions` for `kind`, refusing a name the entity already has. */
export function addExtension<T extends object> (
  extensions: Map<string, Factory<T>>,
  prototype: T,
  kind: string,
  name: string,
  factory: Factory<T>,
): void {
  if (name in prototype || extensions.has(name)) {
    throw new ValidationError(`Plugin '${name}' can't add '${name}' to a ${kind}: it already has one.`);
  }
  extensions.set(name, factory);
}

/** Put each extension on `entity`, as a getter that makes its member once. */
export function applyExtensions<T extends object> (entity: T, extensions: Map<string, Factory<T>> | undefined): T {
  extensions?.forEach((factory, name) => {
    let made = false;
    let value: unknown;
    Object.defineProperty(entity, name, {
      configurable: true,
      enumerable: false,
      get () {
        if (!made) {
          value = factory(entity);
          made = true;
        }
        return value;
      },
    });
  });
  return entity;
}
