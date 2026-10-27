/**
 * What feeds a query, and what a control feeds: the mappings and cross-filters of the declaration,
 * with each source's current value from the snapshot.
 */
import type { InspectedApp, SemanticType } from '../../../common/types';
import { controlMappings, crossFilters } from './checks';
import { formatCondition } from './format';

/** One thing that filters a query: its own `filters` block, a control mapped to it, or a query cross-filtering it. */
export interface QueryInput {
  kind: 'declared' | 'control' | 'crossFilter';
  /** The control or query it comes from; the query itself for a self cross-filter. */
  source: string;
  self: boolean;
  /** The field it filters on, with its aggregation; for a cross-filter, the selected fields. */
  field: string;
  /** What kind of values the field holds, when it can be told; the query's own columns may know better. */
  type?: SemanticType;
  /** What the query was last run with, in words: the applied value, not the one being edited. */
  value: string;
  /** The changed value not yet applied, in words, when it differs from `value`. */
  pending?: string;
}

const MANUAL_TYPES: Record<string, SemanticType> = {
  string: 'text',
  number: 'number',
  date: 'date',
  boolean: 'truefalse',
};

/** The kind of a control's values: the type its manual filter declares, else that of the value set. */
function controlType (
  declared: string | undefined,
  condition: InspectedApp['controls'][string]['condition'] | undefined,
): SemanticType | undefined {
  if (declared && MANUAL_TYPES[declared]) return MANUAL_TYPES[declared];
  const value: unknown = condition?.values?.[0];
  if (typeof value === 'number') return 'number';
  if (typeof value === 'boolean') return 'truefalse';
  if (value instanceof Date) return 'datetime';
  return typeof value === 'string' ? 'text' : undefined;
}

type Selection = NonNullable<InspectedApp['appliedSelection']>;

/** The field a mapping filters on, with its aggregation. */
const mappingField = (m: ReturnType<typeof controlMappings>[number]) =>
  m.aggregation ? `${m.aggregation}(${m.field})` : m.field;

const conditionText = (condition: InspectedApp['controls'][string]['condition'] | undefined) =>
  condition?.values?.length ? formatCondition(condition) : 'not set';

const selectionText = (selection: Selection | undefined) =>
  selection
    ? selection.conditions
        .map((condition) => `${condition.field} ${formatCondition(condition)}`)
        .join('; ') ||
      selection.expression ||
      `${selection.rowCount} rows`
    : 'no selection';

export function queryInputs (app: InspectedApp, query: string): QueryInput[] {
  const controls: QueryInput[] = controlMappings(app)
    .filter((m) => m.to.name === query)
    .map((m) => {
      const control = app.controls[m.from.name];
      const value = conditionText(control?.appliedCondition);
      const current = conditionText(control?.condition);
      return {
        kind: 'control',
        source: m.from.name,
        self: false,
        field: mappingField(m),
        type: controlType(
          app.declaration.filters[m.from.name]?.type,
          control?.appliedCondition ?? control?.condition,
        ),
        value,
        pending: control?.isDirty && current !== value ? current : undefined,
      };
    });
  const crosses: QueryInput[] = crossFilters(app)
    .filter((c) => c.to.name === query)
    .map((c) => {
      const from = (selection: Selection | undefined) =>
        selection?.source === c.from.name ? selection : undefined;
      const applied = from(app.appliedSelection);
      const current = from(app.selection);
      const value = selectionText(applied);
      const next = selectionText(current);
      return {
        kind: 'crossFilter',
        source: c.from.name,
        self: c.from.name === query,
        field: (applied ?? current)?.fields.join(', ') || '—',
        value,
        pending: next !== value ? next : undefined,
      };
    });
  return [...controls, ...crosses];
}

/** The queries a control is mapped to, with the field each mapping filters on. */
export function controlTargets (app: InspectedApp, control: string) {
  return controlMappings(app)
    .filter((m) => m.from.name === control)
    .map((m) => ({ query: m.to.name, field: mappingField(m) }));
}

/** The queries a query cross-filters: selecting its rows filters each of them. */
export function crossFilterTargets (app: InspectedApp, query: string) {
  return crossFilters(app)
    .filter((c) => c.from.name === query)
    .map((c) => ({ query: c.to.name, self: c.to.name === query }));
}

/** The AQL without its `//` and block comments, quotes respected. */
function withoutComments (text: string): string {
  let out = '';
  let quote: string | undefined;
  for (let i = 0; i < text.length; i++) {
    const ch = text[i] ?? '';
    if (quote) {
      out += ch;
      if (ch === '\\') out += text[++i] ?? '';
      else if (ch === quote) quote = undefined;
    } else if (ch === '"' || ch === "'" || ch === '`') {
      quote = ch;
      out += ch;
    } else if (ch === '/' && text[i + 1] === '/') {
      while (i < text.length && text[i] !== '\n') i++;
      i--;
    } else if (ch === '/' && text[i + 1] === '*') {
      const end = text.indexOf('*/', i + 2);
      i = end < 0 ? text.length : end + 1;
      out += ' ';
    } else out += ch;
  }
  return out;
}

/** The text of the `{ … }` block opening at `open`, braces and quotes balanced; undefined if it never closes. */
function blockAt (text: string, open: number): string | undefined {
  let depth = 0;
  let quote: string | undefined;
  for (let i = open; i < text.length; i++) {
    const ch = text[i] ?? '';
    if (quote) {
      if (ch === '\\') i++;
      else if (ch === quote) quote = undefined;
    } else if (ch === '"' || ch === "'" || ch === '`') quote = ch;
    else if (ch === '{') depth++;
    else if (ch === '}' && --depth === 0) return text.slice(open + 1, i);
  }
  return undefined;
}

/** Splits at newlines and `;` outside brackets and quotes, so a condition may span lines. */
function splitEntries (block: string): string[] {
  const entries: string[] = [];
  let depth = 0;
  let quote: string | undefined;
  let current = '';
  for (let i = 0; i < block.length; i++) {
    const ch = block[i] ?? '';
    if (quote) {
      if (ch === '\\') {
        current += ch + (block[++i] ?? '');
        continue;
      }
      if (ch === quote) quote = undefined;
    } else if (ch === '"' || ch === "'" || ch === '`') quote = ch;
    else if ('({['.includes(ch)) depth++;
    else if (')}]'.includes(ch)) depth--;
    else if ((ch === ';' || ch === '\n') && depth <= 0) {
      entries.push(current);
      current = '';
      continue;
    }
    current += ch;
  }
  entries.push(current);
  return entries.map((entry) => entry.trim()).filter(Boolean);
}

/** The kind of an AQL literal, as far as its spelling says: `'a'`, `3`, `@2024-01-01`, `true`. */
function literalType (operand: string): SemanticType | undefined {
  if (/^["'`]/.test(operand)) return 'text';
  if (/^-?\d/.test(operand)) return 'number';
  if (operand.startsWith('@')) return 'date';
  return /^(true|false)\b/.test(operand) ? 'truefalse' : undefined;
}

const CONDITION = /^(.*?)\s+(==|!=|>=|<=|>|<|not in|in|is not|is|like|not like|matches|not matches)\s+(.+)$/s;

/**
 * The filters a query's own AQL declares: each entry of its `filters { … }` block, as the field
 * it filters and the condition on it. An entry that isn't `field operator value` keeps its whole
 * text as the field.
 */
export function declaredFilters (source: string | undefined): QueryInput[] {
  const aql = source && withoutComments(source);
  const at = aql?.search(/\bfilters\s*\{/);
  if (!aql || at === undefined || at < 0) return [];
  const block = blockAt(aql, aql.indexOf('{', at));
  if (block === undefined) return [];
  return splitEntries(block).map((entry) => {
    const text = entry.replace(/\s+/g, ' ');
    const match = CONDITION.exec(text);
    const operand = match?.[3]?.replace(/^[[(]\s*/, '');
    return {
      type: operand ? literalType(operand) : undefined,
      kind: 'declared',
      source: 'query',
      self: true,
      field: match?.[1] ?? text,
      value: match ? `${match[2]} ${match[3]}` : '—',
    };
  });
}

/** Whether a filter narrows the query now: a declared one always does, an input only once it has a value. */
export const isActive = (input: QueryInput) =>
  input.kind === 'declared' || (input.value !== 'not set' && input.value !== 'no selection');
