/**
 * What the reader has changed since the app loaded: controls whose condition is no longer the one
 * the definition declared. Unlike "dirty" (a control differing from what queries last ran with),
 * this compares against the original.
 */
import type { Condition, InspectedApp, InspectedControl } from '../../../common/types';
import { formatCondition } from './format';

export interface ControlChange {
  app: number;
  name: string;
  kind: InspectedControl['kind'];
  from: string;
  to: string;
}

/** The condition a control starts with, as the SDK builds it from the declaration. */
function originalCondition (app: InspectedApp, name: string, kind: InspectedControl['kind']): Condition {
  if (kind === 'dateDrill') {
    const grain = app.declaration.dateDrills[name]?.default;
    return grain
      ? { operator: 'transform_date_drill', values: [`datetrunc ${grain}`] }
      : { operator: 'none' };
  }
  return app.declaration.filters[name]?.default ?? { operator: 'is', values: [] };
}

const same = (a: Condition, b: Condition) =>
  a.operator === b.operator &&
  (a.modifier ?? '') === (b.modifier ?? '') &&
  JSON.stringify((a.values ?? []).map(String)) === JSON.stringify((b.values ?? []).map(String));

/** A condition in words; a date drill by its grain. */
function describe (condition: Condition): string {
  if (condition.operator === 'none') return 'none';
  if (condition.operator === 'transform_date_drill') {
    return String(condition.values?.[0] ?? '').replace('datetrunc ', '') || 'none';
  }
  return (condition.values ?? []).length === 0 && condition.operator === 'is'
    ? 'no value'
    : formatCondition(condition);
}

/** Whether a control's condition differs from the one its declaration starts with. */
export const isChanged = (app: InspectedApp, name: string, control: InspectedControl) =>
  !same(originalCondition(app, name, control.kind), control.condition);

export function changesOf (apps: InspectedApp[]): ControlChange[] {
  return apps.flatMap((app, index) =>
    Object.entries(app.controls).flatMap(([name, control]) => {
      const original = originalCondition(app, name, control.kind);
      return same(original, control.condition)
        ? []
        : [
            {
              app: index,
              name,
              kind: control.kind,
              from: describe(original),
              to: describe(control.condition),
            },
          ];
    }),
  );
}
