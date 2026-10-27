/** Static checks over an app's declaration, for the mistakes the SDK reports late or never. */
import type { ControlMappingJson, CrossFilterJson, InspectedApp } from '../../../common/types';

export type CheckLevel = 'error' | 'warn' | 'info';

export interface Check {
  level: CheckLevel;
  message: string;
  /** The entity the check is about, to link to. */
  about: { entity: 'query' | 'control', name: string };
}

/** Whether `aql` names `field` as a whole name: `orders.created_at`, not `orders.created_at_utc`. */
export function mentionsField (aql: string, field: string): boolean {
  const escaped = field.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp(`(^|[^\\w.])${escaped}(?![\\w])`).test(aql);
}

export const controlMappings = (app: InspectedApp) => app.declaration.mappings.filter((m): m is ControlMappingJson => m.kind === 'control');

export const crossFilters = (app: InspectedApp) => app.declaration.mappings.filter((m): m is CrossFilterJson => m.kind === 'crossFilter');

/** Every check that fails for `app`, errors first. */
export function wiringChecks (app: InspectedApp): Check[] {
  const { queries, filters, dateDrills } = app.declaration;
  const mappings = controlMappings(app);
  const crosses = crossFilters(app);
  const checks: Check[] = [];

  for (const cross of crosses) {
    const from = queries[cross.from.name];
    const to = queries[cross.to.name];
    if (from?.dataset && to?.dataset && from.dataset !== to.dataset) {
      checks.push({
        level: 'error',
        message: `Cross-filter ${cross.from.name} → ${cross.to.name} spans datasets (${from.dataset} → ${to.dataset}).`,
        about: { entity: 'query', name: cross.from.name },
      });
    }
  }
  for (const name of [...Object.keys(filters), ...Object.keys(dateDrills)]) {
    if (!mappings.some((m) => m.from.name === name)) {
      checks.push({ level: 'warn', message: `${name} is mapped to no query, so it has no effect.`, about: { entity: 'control', name } });
    }
  }
  for (const mapping of mappings) {
    if (mapping.from.kind !== 'dateDrill') continue;
    const aql = queries[mapping.to.name]?.aql;
    if (aql && !mentionsField(aql, mapping.field)) {
      checks.push({
        level: 'warn',
        message: `${mapping.from.name} → ${mapping.to.name}: ${mapping.field} isn't in the query's AQL, so the grain never changes.`,
        about: { entity: 'control', name: mapping.from.name },
      });
    }
  }
  for (const name of Object.keys(queries)) {
    const fed = mappings.some((m) => m.to.name === name) || crosses.some((c) => c.to.name === name);
    if (!fed) checks.push({ level: 'info', message: `${name} has no controls or cross-filters, so it never changes.`, about: { entity: 'query', name } });
  }
  return checks;
}
