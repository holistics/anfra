/**
 * `base` with `changes` merged in, as an entity's `update` takes them: a key given `undefined` is
 * deleted, and a key left out keeps its value. `keys` limits the merge to those keys.
 */
export function withChanges<T extends object> (
  base: T,
  changes: Partial<T>,
  keys: readonly (keyof T)[] = Object.keys(changes) as (keyof T)[],
): T {
  const merged = { ...base };
  keys.forEach((key) => {
    if (!(key in changes)) return;
    const value = changes[key];
    if (value === undefined) delete (merged as Record<PropertyKey, unknown>)[key];
    else merged[key] = value as T[keyof T];
  });
  return merged;
}
