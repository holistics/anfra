/**
 * Annotations, the kernel's side: what a plugin's frame half says about a query's data, checked as
 * `kernel.annotate` sets it and resolved against the query's current result as `query.annotations`
 * reads it. The definition draws the kinds it declares; the runtime badges the rest
 * (./annotationBadges.ts). See docs/designs/sdk-plugins.md, Annotations.
 */
import { ValidationError } from '../common/errors';
import { withSuggestion } from './suggest';
import type {
  Annotation, AnnotationInput, AnnotationKind, AnnotationSeverity, ColumnMeta, QueryResult, Row, SeriesPoint,
} from '../common/types';

export const ANNOTATION_KINDS: readonly AnnotationKind[] = ['mark', 'focus', 'line', 'band', 'series'];

export const SEVERITIES: readonly AnnotationSeverity[] = ['info', 'warning', 'critical'];

const isObject = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);

/** Checks one plugin's annotations against a query's result, and copies them: throws on the first that is malformed. */
export function checkAnnotations (plugin: string, queryName: string, result: QueryResult, annotations: unknown): AnnotationInput[] {
  if (!Array.isArray(annotations)) {
    throw new ValidationError(`Plugin '${plugin}' annotated query '${queryName}' with something other than a list of annotations.`, queryName);
  }
  const ids = new Set<string>();
  return annotations.map((annotation: unknown, position) => {
    const at = `Annotation ${position} of plugin '${plugin}' on query '${queryName}'`;
    const fail = (message: string): never => { throw new ValidationError(`${at} ${message}`, queryName); };
    if (!isObject(annotation)) return fail('is not an object.');
    const { kind } = annotation;
    if (!ANNOTATION_KINDS.includes(kind as AnnotationKind)) {
      return fail(`has kind '${String(kind)}', but a kind is one of ${ANNOTATION_KINDS.map((k) => `'${k}'`).join(', ')}.`);
    }
    const text: Record<string, string> = {};
    for (const field of ['id', 'label', 'detail'] as const) {
      const value = annotation[field];
      if (value === undefined) continue;
      if (typeof value !== 'string') fail(`has a ${field} that isn't a string.`);
      text[field] = value as string;
    }
    if (text.id !== undefined) {
      if (ids.has(text.id)) fail(`has the id '${text.id}', which another annotation in the set has.`);
      ids.add(text.id);
    }
    if (annotation.severity !== undefined) {
      if (!SEVERITIES.includes(annotation.severity as AnnotationSeverity)) {
        fail(`has severity '${String(annotation.severity)}', but a severity is 'info', 'warning' or 'critical'.`);
      }
      text.severity = annotation.severity as string;
    }
    const check = anchorChecker(result.columns, fail);
    const anchor = isObject(annotation.anchor) ? annotation.anchor : fail('has no anchor.');
    return { kind, ...text, anchor: check(kind as AnnotationKind, anchor) } as AnnotationInput;
  });
}

function anchorChecker (columns: readonly ColumnMeta[], fail: (message: string) => never) {
  const column = (name: unknown, role: string): ColumnMeta => {
    const found = typeof name === 'string' ? columns.find((candidate) => candidate.name === name) : undefined;
    if (found) return found;
    return fail(typeof name === 'string'
      ? withSuggestion(`names ${role} '${name}', but the result has no column '${name}'.`, name, columns.map((c) => c.name))
      : `has no ${role}.`);
  };
  const measure = (name: unknown): string => {
    const found = column(name, 'the measure');
    if (!found.isMeasure) fail(`names '${found.name}' as its measure, but '${found.name}' is not a measure column.`);
    return found.name;
  };
  const string = (value: unknown, what: string): string => (typeof value === 'string' ? value : fail(`has a ${what} that isn't a string. Values are strings, as rows carry them.`));
  /** A row named by its dimension values. */
  const dimensions = (row: unknown, what: string): Row => {
    if (!isObject(row) || !Object.keys(row).length) return fail(`has a ${what} that doesn't name a row by its dimension values.`);
    for (const name of Object.keys(row)) {
      if (column(name, 'the column').isMeasure) fail(`names a ${what} by '${name}', a measure; name rows by their dimension values.`);
    }
    return { ...row };
  };

  return (kind: AnnotationKind, anchor: Record<string, unknown>): AnnotationInput['anchor'] => {
    if (kind === 'mark' || kind === 'focus') {
      if (!Array.isArray(anchor.rows) || !anchor.rows.length) fail('has no rows in its anchor.');
      const rows = (anchor.rows as unknown[]).map((row) => dimensions(row, 'row'));
      return { rows, ...(anchor.measure !== undefined ? { measure: measure(anchor.measure) } : {}) };
    }
    if (kind === 'line') {
      return { column: column(anchor.column, 'the column').name, value: string(anchor.value, 'value') };
    }
    if (kind === 'band') {
      if (anchor.from === undefined || anchor.to === undefined) fail('needs a from and a to in its anchor.');
      return { column: column(anchor.column, 'the column').name, from: string(anchor.from, 'from'), to: string(anchor.to, 'to') };
    }
    const name = measure(anchor.measure);
    if (!Array.isArray(anchor.points) || !anchor.points.length) fail('has no points in its anchor.');
    const points = (anchor.points as unknown[]).map((point): SeriesPoint => {
      if (!isObject(point)) return fail('has a point that is not an object.');
      const hasBounds = point.lower !== undefined && point.upper !== undefined;
      if (point.value === undefined && !hasBounds) fail('has a point with neither a value nor both bounds (lower and upper).');
      if ((point.lower === undefined) !== (point.upper === undefined)) fail('has a point with one bound: give its value or bounds, both lower and upper.');
      return {
        at: dimensions(point.at, 'point'),
        ...(point.value !== undefined ? { value: string(point.value, 'value') } : {}),
        ...(hasBounds ? { lower: string(point.lower, 'lower bound'), upper: string(point.upper, 'upper bound') } : {}),
      };
    });
    return { measure: name, points };
  };
}

const sameValue = (a: unknown, b: unknown): boolean => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);

/** Whether a result row is the one an anchor's row names: every dimension it gives matches. */
export const rowMatches = (named: Row, row: Row): boolean => Object.keys(named).every((name) => sameValue(named[name], row[name]));

/**
 * Every plugin's annotations, merged in the order the plugins set them, as the definition reads
 * them: a rows anchor resolved to its rows' positions in `rows`, keeping only the rows found, and
 * left out when none is.
 */
export function resolveAnnotations (sets: ReadonlyMap<string, readonly AnnotationInput[]>, rows: readonly Row[]): Annotation[] {
  const merged: Annotation[] = [];
  for (const [plugin, set] of sets) {
    set.forEach((annotation, position) => {
      const key = `${plugin}:${annotation.id ?? position}`;
      if (annotation.kind !== 'mark' && annotation.kind !== 'focus') {
        merged.push({ ...annotation, plugin, key } as Annotation);
        return;
      }
      const found: Row[] = [];
      const index: number[] = [];
      for (const named of annotation.anchor.rows) {
        let any = false;
        rows.forEach((row, at) => {
          if (!rowMatches(named, row)) return;
          any = true;
          if (!index.includes(at)) index.push(at);
        });
        if (any) found.push(named);
      }
      if (!index.length) return;
      merged.push({
        ...annotation, anchor: { ...annotation.anchor, rows: found, index: index.sort((a, b) => a - b) }, plugin, key,
      });
    });
  }
  return merged;
}
