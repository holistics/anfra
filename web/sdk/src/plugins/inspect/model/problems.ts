/**
 * Everything wrong with the inspected frame, in one list: failed queries, the frame's other
 * runtime errors, wiring checks and structure problems, most severe first, each with where it is.
 */
import type { InspectedApp, StructureNode } from '../../../common/types';
import type { Snapshot } from '../../../host/panel';
import { type Check, wiringChecks } from './checks';
import { handleFor } from './format';
import type { RevealTarget } from './types';
import type { RuntimeEntry } from '../state';
import type { AppErrorKind } from '../../../common/runtimeErrors';

export interface Problem {
  level: 'error' | 'warn';
  message: string;
  /** Where it is; a runtime error with nowhere to go shows `detail` instead. */
  target?: RevealTarget;
  /** A runtime error's stack, the call that failed, its request id. */
  detail?: string;
  /** How many times a runtime error happened. */
  count?: number;
}

const RUNTIME_LABELS: Record<string, string> = {
  'call.query': 'Query failed',
  'app.uncaught': 'Uncaught error',
  'app.listener': 'A subscribe() listener threw',
  'plugin.uncaught': 'A plugin failed',
  'host.internal': 'The host failed a call',
};

/** A runtime error's kind, as the panel says it: a kind it does not know, by its source. */
function runtimeLabel (kind: AppErrorKind): string {
  if (Object.hasOwn(RUNTIME_LABELS, kind)) return RUNTIME_LABELS[kind];
  const [source, what] = kind.split('.', 2);
  return source === 'call' ? `Unhandled ${what} call failed` : `Error (${kind})`;
}

/** A runtime error as a problem: to its query or control when it names one, else with its stack. */
export function runtimeProblem (entry: RuntimeEntry, snapshot: Snapshot): Problem {
  const { event, count } = entry;
  const detail = [
    event.call && `Call: ${event.call}`,
    event.code && `Code: ${event.code}${event.scope ? ` (${event.scope})` : ''}`,
    event.requestId && `Request: ${event.requestId}`,
    event.stack,
  ].filter(Boolean).join('\n');
  const problem: Problem = { level: 'error', message: `${runtimeLabel(event.kind)}: ${event.message}` };
  if (detail) problem.detail = detail;
  if (count > 1) problem.count = count;
  const { entity } = event;
  if (entity) {
    // Own names only: an entity the frame calls 'constructor' is no query.
    const app = snapshot.apps.findIndex((a) => Object.hasOwn(a.queries, entity) || Object.hasOwn(a.controls, entity));
    if (app >= 0) problem.target = { entity: Object.hasOwn(snapshot.apps[app].queries, entity) ? 'query' : 'control', app, name: entity };
  }
  return problem;
}

/** Each structure node with problems, in tree order. */
export function problemNodes (nodes: StructureNode[]): StructureNode[] {
  return nodes.flatMap((node) => [...(node.problems.length ? [node] : []), ...problemNodes(node.children)]);
}

/** `checks` is the app's `wiringChecks`, when already worked out. */
export function appProblems (app: InspectedApp, index: number, checks = wiringChecks(app)): Problem[] {
  const problems: Problem[] = [];
  for (const [name, query] of Object.entries(app.queries)) {
    if (query.state !== 'error') continue;
    problems.push({ level: 'error', message: `${name} failed: ${query.error?.message ?? 'no message'}`, target: { entity: 'query', app: index, name } });
  }
  checks.forEach((check, i) => {
    if (check.level === 'info') return;
    problems.push({ level: check.level, message: check.message, target: { check: i, app: index } });
  });
  return problems;
}

/**
 * `checks` is each app's `wiringChecks`, by index, when already worked out; `runtime`, the frame's
 * runtime errors, newest first, listed after the apps' own.
 */
export function problemsOf (snapshot: Snapshot, checks?: Check[][], runtime: readonly RuntimeEntry[] = []): Problem[] {
  const problems = snapshot.apps.flatMap((app, index) => appProblems(app, index, checks?.[index]));
  problems.push(...runtime.map((entry) => runtimeProblem(entry, snapshot)));
  for (const node of problemNodes(snapshot.structure.nodes)) {
    for (const problem of node.problems) problems.push({ level: 'warn', message: `${handleFor(node)}: ${problem}`, target: { node: node.key } });
  }
  // Stable: errors first, each level in the order found.
  return [...problems.filter((p) => p.level === 'error'), ...problems.filter((p) => p.level === 'warn')];
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
