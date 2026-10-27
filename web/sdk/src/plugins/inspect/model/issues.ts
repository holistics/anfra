/**
 * Everything wrong with the inspected frame, in one list: failed queries, wiring checks and
 * structure problems, most severe first, each with where it is.
 */
import type { InspectedApp, StructureNode } from '../../../common/types';
import type { Snapshot } from '../../../host/panel';
import { type Check, wiringChecks } from './checks';
import { handleFor } from './format';
import type { RevealTarget } from './types';

export interface Issue {
  level: 'error' | 'warn';
  message: string;
  target: RevealTarget;
}

/** Each structure node with problems, in tree order. */
export function problemNodes (nodes: StructureNode[]): StructureNode[] {
  return nodes.flatMap((node) => [...(node.problems.length ? [node] : []), ...problemNodes(node.children)]);
}

/** `checks` is the app's `wiringChecks`, when already worked out. */
export function appIssues (app: InspectedApp, index: number, checks = wiringChecks(app)): Issue[] {
  const issues: Issue[] = [];
  for (const [name, query] of Object.entries(app.queries)) {
    if (query.state !== 'error') continue;
    issues.push({ level: 'error', message: `${name} failed: ${query.error?.message ?? 'no message'}`, target: { entity: 'query', app: index, name } });
  }
  checks.forEach((check, i) => {
    if (check.level === 'info') return;
    issues.push({ level: check.level, message: check.message, target: { check: i, app: index } });
  });
  return issues;
}

/** `checks` is each app's `wiringChecks`, by index, when already worked out. */
export function issuesOf (snapshot: Snapshot, checks?: Check[][]): Issue[] {
  const issues = snapshot.apps.flatMap((app, index) => appIssues(app, index, checks?.[index]));
  for (const node of problemNodes(snapshot.structure.nodes)) {
    for (const problem of node.problems) issues.push({ level: 'warn', message: `${handleFor(node)}: ${problem}`, target: { node: node.key } });
  }
  // Stable: errors first, each level in the order found.
  return [...issues.filter((i) => i.level === 'error'), ...issues.filter((i) => i.level === 'warn')];
}

/**
 * How many queries are in each state, and how many are dirty, across every app; and how many
 * controls are dirty, so that `dirty + dirtyControls` is what Queries' dirty filter shows.
 */
export function queryCounts (apps: InspectedApp[]) {
  const counts = { success: 0, error: 0, executing: 0, idle: 0, dirty: 0, total: 0, dirtyControls: 0 };
  for (const app of apps) {
    for (const query of Object.values(app.queries)) {
      counts[query.state]++;
      counts.total++;
      if (query.isDirty) counts.dirty++;
    }
    for (const control of Object.values(app.controls)) if (control.isDirty) counts.dirtyControls++;
  }
  return counts;
}
