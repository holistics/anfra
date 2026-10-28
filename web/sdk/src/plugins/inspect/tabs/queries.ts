/**
 * Queries: every app's queries, controls and selection, one line each until opened, and the
 * requests the frame made, under the query each belongs to or under Other requests. The search,
 * filter, open rows and closed sections are the mount's, so they outlive a switch of tab; what is
 * open inside a row (its sections, code tab, "Show all") is the tab's.
 */
import type {
  ColumnMeta, InspectedApp, InspectedControl, InspectedError, InspectedQuery, InspectedSelection, InspectedStructure, LocateKind,
} from '../../../common/types';
import type { TabContext } from '../../../host/contributions';
import { columnKind } from '../model/columns';
import { isChanged } from '../model/changes';
import {
  blockNodeKey, dedent, entityAttribute, entityKey, entityNodeKey, formatAgo, formatCondition, formatTime, plural, usageOf,
} from '../model/format';
import {
  controlTargets, crossFilterTargets, declaredFilters, isActive, queryInputs, type QueryInput,
} from '../model/inputs';
import { queryCounts } from '../model/issues';
import { attributeRequests, type RequestEntry } from '../model/requests';
import { queryTrust } from '../model/trust';
import type { QueriesFilter } from '../model/types';
import { stateOf, type InspectState } from '../state';
import {
  codeBlock, collapsible, copyIcon, drawTab, fields, filterIndicator, flash, h, icon, lineageButton, link, note, stateDot, table, trustIndicator,
  type Children, type IconName,
} from '../view';

type CodeTab = 'declared' | 'aql' | 'sql' | 'signature';
type Section = 'queries' | 'controls' | 'selection' | 'other';
type Attributed = { entry: RequestEntry, shared?: boolean };

/** How many requests show before "Show all". */
const FIRST = 5;
const FILTERS: readonly QueriesFilter[] = ['all', 'errors', 'dirty'];

/** The last reveal each mount's Queries handled, so a remount of the tab doesn't flash it again. */
const handled = new WeakMap<InspectState, number>();

const matches = (search: string, ...texts: (string | undefined)[]) => !search || texts.some((text) => text?.toLowerCase().includes(search));

function showQuery (view: InspectState['queries'], app: InspectedApp, name: string, query: InspectedQuery): boolean {
  if (view.filter === 'errors' && query.state !== 'error') return false;
  if (view.filter === 'dirty' && !query.isDirty) return false;
  return matches(view.search.trim().toLowerCase(), name, app.declaration.queries[name]?.dataset);
}

function showControl (view: InspectState['queries'], app: InspectedApp, name: string, control: InspectedControl): boolean {
  // Controls have no error state.
  if (view.filter === 'errors') return false;
  if (view.filter === 'dirty' && !control.isDirty) return false;
  const declaration = app.declaration.filters[name] ?? app.declaration.dateDrills[name];
  const field = declaration && 'field' in declaration ? declaration.field : undefined;
  return matches(view.search.trim().toLowerCase(), name, declaration?.label, field);
}

const errorText = (error: InspectedError) => `${error.name}${error.status !== undefined ? ` (${error.status})` : ''}: ${error.message}`;

const queriesText = (n: number) => (n === 1 ? '1 query' : `${n} queries`);

function prettyJson (text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

const CALL_LABELS: Record<string, string> = {
  'core.submitQuery': 'Query',
  'export.query': 'Export',
  'core.fieldSuggestions': 'Suggestions',
  'lineage.trace': 'Lineage',
  'render.save': 'Picture saved',
};

const labelOf = (entry: RequestEntry) => CALL_LABELS[entry.call] ?? entry.call;

/** A request's title: the field it suggests for, the dataset it queries, or what it is. */
const titleOf = (entry: RequestEntry) => (entry.call === 'core.fieldSuggestions' && entry.field ? entry.field : entry.dataset || labelOf(entry));

/** What a request's line says: where its rows came from, the page, and the rows or how it ended. */
function summaryOf (entry: RequestEntry): string {
  const ended = entry.status !== 'ok' && entry.status !== 'pending' ? entry.status : undefined;
  if (entry.call === 'core.fieldSuggestions') {
    return `suggestions for ${entry.field}${entry.q ? ` "${entry.q}"` : ''}${entry.numRows === undefined ? '' : ` · ${entry.numRows}`}`;
  }
  if (entry.call === 'export.query') {
    return [`export${entry.format ? ` ${entry.format}` : ''}`, entry.status === 'ok' ? plural(entry.numRows ?? 0, 'row') : ended].filter(Boolean).join(' · ');
  }
  if (entry.call !== 'core.submitQuery') {
    return [entry.targets?.join(', ') ?? entry.filename ?? (entry.caller === 'frame' ? undefined : entry.caller), ended].filter(Boolean).join(' · ');
  }
  return [
    entry.fromCache === undefined ? undefined : entry.fromCache ? 'cache' : 'database',
    entry.page && entry.page > 1 ? `page ${entry.page}` : undefined,
    entry.status === 'ok' ? plural(entry.numRows ?? 0, 'row') : ended,
  ].filter(Boolean).join(' · ');
}

/** What a request asked, as JSON: its input with the call's other fields merged in, the declared AQL left out. */
function inputOf (entry: RequestEntry): string | undefined {
  const { input, page, pageSize, timezone, bustCache, dataset, aql, field, q, format, formatOptions, filename, targets, request } = entry;
  const shown = entry.call === 'core.submitQuery' ? { dataset, page, pageSize, timezone, bustCache, ...input }
    : entry.call === 'export.query' ? { dataset, timezone, format, formatOptions, filename, ...input }
      : entry.call === 'core.fieldSuggestions' ? { dataset, field, q }
        : entry.call === 'lineage.trace' ? { targets }
          : entry.call === 'render.save' ? { filename }
            : typeof request === 'object' && request !== null && !Array.isArray(request) ? request as Record<string, unknown> : { request };
  const kept = Object.entries(shown).filter(([, value]) => value !== undefined && !(Array.isArray(value) && value.length === 0));
  return kept.length ? JSON.stringify(Object.fromEntries(kept), null, 2) : aql;
}

/** How many rows a preview holds, of how many came back. */
function rowCount (shown: number, total?: number): string {
  if (shown === 0) return 'No rows.';
  return total !== undefined && total > shown ? `First ${shown} of ${total} rows` : plural(shown, 'row');
}

/** Where a filter comes from, in a sentence. */
function filterSource (input: QueryInput): string {
  if (input.kind === 'declared') return "Declared in this query's AQL filters block";
  if (input.kind === 'control') return `Applied from the control "${input.source}"`;
  return input.self ? "Cross-filter from selecting rows in this query's own result" : `Cross-filter from selecting rows in "${input.source}"`;
}

const fieldOf = (column: ColumnMeta) => (column.modelId ? `${column.modelId}.${column.fieldName}` : column.fieldName);

export default function queriesTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel } = context;
  const state = stateOf(context.mount);
  const view = state.queries;
  const canViewSql = context.mount.user.permissions.canViewGeneratedSql;
  const hasLineage = context.plugins.some((plugin) => plugin.name === 'lineage');
  // What is open inside a row, kept while the tab is up: inner sections by key, each query's code
  // tab, the rows showing all their filters and all their requests.
  const inner = new Map<string, boolean>();
  const codeTabs = new Map<string, CodeTab>();
  const allFilters = new Set<string>();
  const allRequests = new Set<string>();

  const toggle = (set: Set<string>, key: string) => {
    if (set.has(key)) set.delete(key);
    else set.add(key);
    tab.redraw();
  };

  const iconButton = (name: IconName, label: string, onclick: () => void, attributes: Record<string, string> = {}) => h(doc, 'button', {
    type: 'button',
    class: 'row-action',
    'data-tip': label,
    'aria-label': label,
    ...attributes,
    onclick: (event: Event) => {
      event.stopPropagation();
      onclick();
    },
  }, icon(doc, name));

  /** A section of an app: its count, "N of M" while a search or filter hides some. */
  function section (id: Section, app: number | undefined, title: string, count: number, shown: number | undefined, empty: string, body: () => Children): HTMLElement {
    // `<app>:<id>`, or `other` for the frame's other requests.
    const closedId = app === undefined ? id : `${app}:${id}`;
    const details = collapsible(doc, app === undefined ? `section:${id}` : `section:${id}:${app}`,
      [title, h(doc, 'span', { class: 'count' }, shown === undefined ? String(count) : `${shown} of ${count}`)],
      () => ((shown ?? count) === 0 ? note(doc, empty) : body()),
      { open: !view.closed.has(closedId), onToggle: (open) => (open ? view.closed.delete(closedId) : view.closed.add(closedId)), class: 'section' });
    details.dataset.testid = app === undefined ? `section-${id}` : `section-${id}-${app}`;
    return details;
  }

  /** A query's or control's row, open as the mount keeps it. */
  function entityRow (key: string, summary: Children, body: () => Children): HTMLElement {
    const row = collapsible(doc, key, summary, body, {
      open: view.open.has(key),
      onToggle: (open) => (open ? view.open.add(key) : view.open.delete(key)),
      class: 'row entity',
    });
    row.dataset.testid = key.replace(':', '-');
    return row;
  }

  /** A titled part of an opened row. */
  function sub (key: string, title: Children, body: () => Children, open = true): HTMLElement {
    const details = collapsible(doc, key, title, body, { open: inner.get(key) ?? open, onToggle: (o) => inner.set(key, o), class: 'sub' });
    details.dataset.testid = key.slice(key.lastIndexOf(':') + 1);
    return details;
  }

  /** Where an entity is drawn: "In N places", opening to its blocks, each to highlight or show in Structure. */
  function drawn (structure: InspectedStructure | undefined, kind: LocateKind, app: number, name: string): Children {
    const usage = usageOf(structure, kind, app, name);
    if (!usage) return h(doc, 'span', { class: 'muted' }, 'Not on the page yet');
    const entity = entityKey(kind, app, name);
    return collapsible(doc, `${entity}:places`, [
      `In ${plural(usage.markers, 'place')}${usage.blocks.length ? '' : ', outside any block'}`,
      copyIcon(doc, 'Copy the handle', () => entityAttribute(kind, app, name)),
    ], () => h(doc, 'ul', { class: 'places' },
      usage.blocks.length === 0 && h(doc, 'li', {},
        h(doc, 'span', { class: 'muted' }, 'Outside any block'),
        iconButton('locate', 'Highlight on the page', () => panel.locate({ app, kind, name }), { 'data-testid': 'locate-entity' })),
      usage.blocks.map((block) => {
        const node = blockNodeKey(structure, block, kind, app, name);
        return h(doc, 'li', { onmouseenter: () => node && panel.highlight(node), onmouseleave: () => panel.highlight(null) },
          h(doc, 'span', {}, block),
          node && [
            iconButton('locate', 'Highlight on the page', () => panel.locate({ node }), { 'data-testid': 'locate-block' }),
            iconButton('tree', 'Show in Structure', () => state.reveal(panel, { node }), { 'data-testid': 'show-in-structure' }),
          ]);
      })), { class: 'inline' });
  }

  /** The queries something reaches: a count that opens to a link to each. */
  function mapsTo (key: string, app: number, targets: { query: string, field?: string, self?: boolean }[], empty: Children): Children {
    if (targets.length === 0) return empty;
    return collapsible(doc, key, queriesText(targets.length), () => h(doc, 'ul', { class: 'places' }, targets.map((target) => h(doc, 'li', {},
      h(doc, 'span', {}, target.query,
        target.field && h(doc, 'span', { class: 'muted' }, ` on ${target.field}`),
        target.self && h(doc, 'span', { class: 'muted' }, ' (itself)')),
      iconButton('query', 'Show query', () => state.reveal(panel, { entity: 'query', app, name: target.query }), { 'data-testid': 'show-query' })))), { class: 'inline' });
  }

  function requestList (scope: string, items: readonly Attributed[], card: boolean): Children {
    const all = allRequests.has(scope);
    return [
      h(doc, 'div', { class: card ? 'request-cards' : 'requests' }, (all ? items : items.slice(0, FIRST)).map((item) => requestRow(scope, item, card))),
      items.length > FIRST && link(doc, all ? 'Show fewer' : `Show all ${items.length}`, () => toggle(allRequests, scope), { 'data-testid': 'requests-more' }),
    ];
  }

  /** One request: in a query, its time, how it ended and how long it took; on its own, a card like a query's row. */
  function requestRow (scope: string, { entry, shared }: Attributed, card: boolean): HTMLElement {
    const dot = h(doc, 'span', { class: `dot ${entry.status}`, role: 'img', 'aria-label': `Status: ${entry.status}`, 'data-tip': entry.status });
    const summary = card
      ? h(doc, 'span', { class: 'entry' },
        h(doc, 'span', { class: 'line' },
          h(doc, 'span', { class: 'name' }, titleOf(entry)),
          h(doc, 'span', { class: 'condition' }, summaryOf(entry)),
          h(doc, 'span', { class: 'spacer' }), dot),
        entry.error && h(doc, 'span', { class: 'error-text', role: 'alert' }, entry.error))
      : h(doc, 'span', { class: 'line' },
        dot,
        h(doc, 'span', { class: 'time' }, formatTime(entry.at)),
        h(doc, 'span', { class: 'condition' }, summaryOf(entry)),
        shared && h(doc, 'span', { class: 'pill', 'data-tip': 'Another query declares the same dataset and AQL' }, 'shared'),
        h(doc, 'span', { class: 'spacer' }),
        h(doc, 'span', { class: 'rows' }, entry.durationMs === undefined ? '…' : `${entry.durationMs} ms`));
    const details = collapsible(doc, `${scope}:request:${entry.id}`, summary, () => [
      card ? fields(doc, [
        ['Type', labelOf(entry)],
        ['Call', entry.call],
        !!entry.dataset && ['Dataset', entry.dataset],
        ['Caller', entry.caller],
        !!entry.trace && ['Middleware', entry.trace.join(' → ')],
        ['Sent at', formatTime(entry.at)],
        ['Duration', entry.durationMs === undefined ? 'Still running' : `${entry.durationMs} ms`],
      ]) : entry.error && h(doc, 'p', { class: 'error-text' }, entry.error),
      entry.preview && entry.columns && [
        h(doc, 'p', { class: 'preview-count' }, rowCount(entry.preview.length, entry.numRows)),
        entry.preview.length > 0 && table(doc, entry.columns, entry.preview, { 'data-testid': 'request-preview' }),
      ],
      codeBlock(doc, { input: { label: 'Input', code: inputOf(entry), lang: 'js', empty: 'No input.' } }, 'input'),
    ], { class: card ? 'row entity' : 'row request' });
    details.dataset.testid = `request-${entry.id}`;
    return details;
  }

  /** The metrics or dimensions a query returns, each opening to what its column says of it. */
  function columnList (key: string, title: string, columns: ColumnMeta[]): Children {
    if (columns.length === 0) return null;
    return sub(`${key}:${title.toLowerCase()}`, [title, h(doc, 'span', { class: 'count' }, `· ${columns.length}`)], () => h(doc, 'div', { class: 'list' },
      columns.map((column) => {
        const unlabelled = !column.label || column.label === column.name;
        return collapsible(doc, `${key}:column:${column.name}`, [
          h(doc, 'span', { class: 'name', 'data-tip': columnKind(column) }, column.name),
          column.adhoc && h(doc, 'span', { class: 'pill warn' }, 'ad hoc'),
          h(doc, 'span', { class: 'spacer' }),
          unlabelled ? h(doc, 'span', { class: 'pill warn' }, 'no label') : h(doc, 'span', { class: 'label' }, column.label),
        ], () => fields(doc, [
          ['Label', column.label || h(doc, 'span', { class: 'muted' }, 'None')],
          ['Type', columnKind(column)],
          ['Field', h(doc, 'code', { class: 'wrap' }, fieldOf(column))],
          column.adhoc && ['Ad hoc', "The query's own AQL expression, not a field the dataset defines"],
          ['Database type', column.db_type],
        ]), { class: 'item' });
      })));
  }

  /** Every input that filters a query, its active ones first: each one's source, field and value. */
  function filterList (key: string, app: number, filters: QueryInput[], active: QueryInput[]): Children {
    if (filters.length === 0) return null;
    const all = allFilters.has(key);
    const shown = all ? filters : active;
    return sub(`${key}:filters`, ['Filters', h(doc, 'span', { class: 'count' }, `· ${shown.length}`)], () => [
      h(doc, 'div', { class: 'segments small', role: 'group', 'aria-label': 'Which filters' },
        h(doc, 'button', { type: 'button', 'aria-pressed': String(!all), 'data-testid': 'filters-active', onclick: () => all && toggle(allFilters, key) }, `Active (${active.length})`),
        h(doc, 'button', { type: 'button', 'aria-pressed': String(all), 'data-testid': 'filters-all', onclick: () => !all && toggle(allFilters, key) }, `All (${filters.length})`)),
      shown.length === 0 ? h(doc, 'p', { class: 'muted' }, 'No active filters.') : h(doc, 'ul', { class: 'list filters', 'data-testid': 'filter-list' }, shown.map((input) => h(doc, 'li', { 'data-tip': filterSource(input) },
        h(doc, 'div', {},
          input.kind === 'declared' ? h(doc, 'span', { class: 'muted' }, 'query')
            : input.self ? h(doc, 'span', { class: 'muted' }, 'itself')
              : link(doc, input.source, () => state.reveal(panel, { entity: input.kind === 'control' ? 'control' : 'query', app, name: input.source }), { 'data-testid': 'filter-source' }),
          h(doc, 'div', { class: 'field' }, input.field)),
        h(doc, 'div', {},
          h(doc, 'span', { class: isActive(input) ? undefined : 'muted' }, input.value),
          input.pending && h(doc, 'div', { class: 'pending' }, `pending: ${input.pending}`))))),
    ]);
  }

  function queryRow (app: InspectedApp, index: number, name: string, query: InspectedQuery, requests: readonly Attributed[], structure: InspectedStructure | undefined): HTMLElement {
    const key = entityKey('query', index, name);
    const declaration = app.declaration.queries[name];
    const marker = structure && entityNodeKey(structure.nodes, 'query', index, name);
    const filters = [...declaredFilters(declaration?.aql), ...queryInputs(app, name)];
    const active = filters.filter(isActive);
    const summary = h(doc, 'span', { class: 'entry' },
      h(doc, 'span', { class: 'line' },
        h(doc, 'span', { class: 'name' }, name),
        h(doc, 'span', { class: 'rows', 'data-tip': query.state === 'success' ? plural(query.rowCount, 'row') : 'No rows yet' },
          query.state === 'success' ? query.rowCount.toLocaleString() : '—'),
        query.isDirty ? h(doc, 'span', { class: 'pill warn' }, 'dirty') : query.debug?.fromCache && h(doc, 'span', { class: 'pill' }, 'cache'),
        h(doc, 'span', { class: 'spacer' }),
        filterIndicator(doc, active),
        hasLineage && lineageButton(doc, panel, marker),
        trustIndicator(doc, queryTrust(declaration, query)),
        stateDot(doc, query.state)),
      // An error shows open or not: it is what a failing app is inspected for.
      query.error && h(doc, 'span', { class: 'error-text', role: 'alert', 'data-testid': 'query-error' }, errorText(query.error)));

    return entityRow(key, summary, () => {
      // The newest run that ended; one another query may have made isn't this one's.
      const latest = requests.find(({ entry, shared }) => !shared && entry.call === 'core.submitQuery' && entry.status !== 'pending')?.entry;
      const notRun = query.state === 'success' || query.state === 'error' ? 'Not returned.' : 'Not run yet.';
      const lastRun = [
        query.debug && (query.debug.fromCache ? 'cache' : 'database'),
        query.debug?.executedAt && formatAgo(query.debug.executedAt),
        latest?.durationMs !== undefined && `${latest.durationMs} ms`,
      ].filter(Boolean).join(' · ');
      return [
        sub(`${key}:general`, 'General', () => [
          fields(doc, [
            ['Dataset', declaration?.dataset || h(doc, 'span', { class: 'muted' }, 'none declared')],
            !!lastRun && ['Last run', lastRun],
            ['On the page', drawn(structure, 'query', index, name)],
            ['Cross-filters', mapsTo(`${key}:cross`, index, crossFilterTargets(app, name), h(doc, 'span', { class: 'muted' }, 'No query'))],
            query.selectedRowCount > 0 && ['Selected', plural(query.selectedRowCount, 'row')],
            (query.hasMore || query.page > 1) && ['Page', `${query.page}${query.hasMore ? ', more available' : ''}`],
          ]),
          h(doc, 'div', { class: 'detail-actions' },
            link(doc, [icon(doc, 'table'), 'Show data'], () => state.reveal(panel, { table: name, app: index }), { 'data-testid': 'show-data' })),
        ]),
        query.columns.length === 0
          ? sub(`${key}:definition`, 'Definition', () => h(doc, 'p', { class: 'muted' }, notRun === 'Not run yet.' ? 'Known once it runs.' : 'No fields.'))
          : [
            columnList(key, 'Metrics', query.columns.filter((column) => column.isMeasure)),
            columnList(key, 'Dimensions', query.columns.filter((column) => !column.isMeasure)),
          ],
        filterList(key, index, filters, active),
        sub(`${key}:code`, 'Query', () => codeBlock(doc, {
          declared: { label: 'Declared AQL', code: declaration?.aql ? dedent(declaration.aql) : undefined, lang: 'aql', empty: 'Not declared.' },
          aql: { label: 'Executed AQL', code: query.debug?.executedAql, lang: 'aql', empty: notRun },
          sql: { label: 'Executed SQL', code: query.debug?.executedSql, lang: 'sql', empty: canViewSql ? notRun : 'Needs canViewGeneratedSql.' },
          signature: { label: 'Signature', code: query.signature ? prettyJson(query.signature) : undefined, empty: 'No signature.' },
        }, codeTabs.get(key) ?? 'aql', (t) => codeTabs.set(key, t)), false),
        requests.length > 0 && sub(`${key}:requests`, ['Requests', h(doc, 'span', { class: 'count' }, `· ${requests.length}`)],
          () => requestList(key, requests, false), false),
      ];
    });
  }

  function controlRow (app: InspectedApp, index: number, name: string, control: InspectedControl, structure: InspectedStructure | undefined): HTMLElement {
    const key = entityKey('control', index, name);
    const declaration = app.declaration.filters[name] ?? app.declaration.dateDrills[name];
    const filter = declaration && 'field' in declaration ? declaration : undefined;
    const targets = controlTargets(app, name);
    const fixed = filter?.options;
    const options = fixed ?? control.options;
    const fallback = declaration?.default;
    const summary = h(doc, 'span', { class: 'line' },
      h(doc, 'span', { class: 'name' }, name),
      h(doc, 'span', { class: 'condition' }, control.condition.values?.length ? formatCondition(control.condition) : ''),
      control.isDirty && h(doc, 'span', { class: 'pill warn' }, 'unapplied'),
      h(doc, 'span', { class: 'spacer' }),
      isChanged(app, name, control) && h(doc, 'span', { class: 'changed', role: 'img', 'aria-label': 'Changed from the default', 'data-tip': 'Changed from the default value' }, 'M'),
      h(doc, 'span', {
        class: 'indicator targets',
        'data-tip': targets.length ? `Applies to ${queriesText(targets.length)}\n${targets.map((t) => `${t.query} on ${t.field}`).join('\n')}` : 'Applies to no query',
      }, String(targets.length), icon(doc, 'query')));
    return entityRow(key, summary, () => fields(doc, [
      ['Kind', [control.kind === 'dateDrill' ? 'Date drill' : 'Filter',
        filter?.field && h(doc, 'span', { class: 'muted' }, ` on ${filter.field}`),
        filter?.type && h(doc, 'span', { class: 'muted' }, ` · ${filter.type}`)]],
      !!declaration?.label && ['Label', declaration.label],
      ['Set to', formatCondition(control.condition)],
      ['Applied', formatCondition(control.appliedCondition)],
      !!fallback && ['Default', typeof fallback === 'string' ? fallback : formatCondition(fallback)],
      options
        ? ['Options', collapsible(doc, `${key}:options`, `${options.length} ${fixed ? 'fixed' : 'loaded'}`, () => {
          const selected = control.condition.values?.map(String) ?? [];
          return h(doc, 'ul', { class: 'chips' }, options.map((option) => h(doc, 'li', { class: selected.includes(String(option)) ? 'selected' : undefined }, String(option))));
        }, { class: 'inline' })]
        : control.kind === 'filter' && ['Options', h(doc, 'span', { class: 'muted' }, 'Not loaded')],
      ['Maps to', mapsTo(`${key}:maps`, index, targets, h(doc, 'span', { class: 'tone-warn' }, 'No query, so it has no effect'))],
      ['On the page', drawn(structure, 'control', index, name)],
    ]));
  }

  function selectionCard (label: string, selection: InspectedSelection): HTMLElement {
    return h(doc, 'div', { class: 'card', 'data-testid': `selection-${label.toLowerCase()}` },
      h(doc, 'header', {}, h(doc, 'strong', {}, label), selection.lossy && h(doc, 'span', { class: 'pill warn' }, 'lossy')),
      selection.lossy && h(doc, 'p', { class: 'tone-warn lossy' },
        "Some selected values can't be expressed as a filter, so the queries it filters see less than was selected."),
      fields(doc, [
        ['From', selection.source],
        ['Rows', String(selection.rowCount)],
        !!selection.expression && ['Expression', h(doc, 'code', { class: 'wrap' }, selection.expression)],
        ...selection.conditions.map((condition) => [condition.field, formatCondition(condition)] as const),
      ]));
  }

  function appSection (app: InspectedApp, index: number, several: boolean, byQuery: Map<string, Attributed[]>, structure: InspectedStructure | undefined): HTMLElement {
    const search = view.search.trim();
    const filtering = search !== '' || view.filter !== 'all';
    const queries = Object.entries(app.queries);
    const controls = Object.entries(app.controls);
    const shownQueries = queries.filter(([name, query]) => showQuery(view, app, name, query));
    const shownControls = controls.filter(([name, control]) => showControl(view, app, name, control));
    const none = (what: string) => (search ? `No ${what} match "${search}".` : `No ${what} match the filter.`);
    const selections = ([['Current', app.selection], ['Applied', app.appliedSelection]] as const).filter(([, selection]) => selection);
    return h(doc, 'div', { class: 'app', 'data-testid': `app-${index}` },
      h(doc, 'p', { class: 'app-line', 'data-testid': 'app-line' },
        several && h(doc, 'strong', {}, `App ${index + 1} · `),
        `${app.title || 'Untitled'} · ${app.timezone || 'tenant timezone'} · `,
        app.hasChanges ? h(doc, 'span', { class: 'tone-warn' }, 'unapplied changes') : 'no unapplied changes'),
      section('queries', index, 'Queries', queries.length, filtering ? shownQueries.length : undefined, filtering ? none('queries') : 'None.',
        () => shownQueries.map(([name, query]) => queryRow(app, index, name, query, byQuery.get(`${index}/${name}`) ?? [], structure))),
      section('controls', index, 'Controls', controls.length, filtering ? shownControls.length : undefined,
        view.filter === 'errors' ? 'Controls have no error state.' : filtering ? none('controls') : 'This app declares no controls.',
        () => shownControls.map(([name, control]) => controlRow(app, index, name, control, structure))),
      selections.length > 0 && section('selection', index, 'Selection', selections.length, undefined, 'None.',
        () => selections.map(([label, selection]) => selectionCard(label, selection!))));
  }

  // The head stays as it is, so typing keeps its focus: a redraw sets its value and counts.
  const input = h(doc, 'input', {
    type: 'search',
    placeholder: 'Search names, datasets, labels',
    'aria-label': 'Search queries and controls',
    'data-testid': 'queries-search',
    oninput: () => {
      view.search = input.value;
      tab.redraw();
    },
    onkeydown: (event: Event) => {
      if ((event as KeyboardEvent).key !== 'Escape' || !input.value) return;
      view.search = '';
      tab.redraw();
    },
  }) as HTMLInputElement;
  const buttons = FILTERS.map((filter) => h(doc, 'button', {
    type: 'button',
    'data-testid': `queries-filter-${filter}`,
    onclick: () => {
      view.filter = filter;
      tab.redraw();
    },
  }));
  const head = h(doc, 'div', { class: 'toolbar' },
    h(doc, 'label', { class: 'search' }, icon(doc, 'search'), input),
    h(doc, 'div', { class: 'segments', role: 'group', 'aria-label': 'Filter' }, buttons));

  const tab = drawTab(root, context, () => {
    const snapshot = panel.snapshot;
    const counts = queryCounts(snapshot?.apps ?? []);
    const labels: Record<QueriesFilter, string> = { all: 'All', errors: `Errors (${counts.error})`, dirty: `Dirty (${counts.dirty + counts.dirtyControls})` };
    if (input.value !== view.search) input.value = view.search;
    buttons.forEach((button, i) => {
      const filter = FILTERS[i]!;
      button.textContent = labels[filter];
      button.setAttribute('aria-pressed', String(view.filter === filter));
    });
    if (!snapshot) return note(doc, 'Waiting for the Data App…', { 'data-testid': 'inspect-waiting' });
    if (!snapshot.apps.length) return note(doc, "This Data App hasn't created an SDK app.", { 'data-testid': 'queries-no-app' });
    const { byQuery, other } = attributeRequests(state.requests.entries(), snapshot.apps);
    return [
      snapshot.apps.map((app, index) => appSection(app, index, snapshot.apps.length > 1, byQuery, snapshot.structure)),
      // At the end, so nothing the frame asked is hidden.
      other.length > 0 && section('other', undefined, 'Other requests', other.length, undefined, 'None.',
        () => requestList('other', other.map((entry) => ({ entry })), true)),
    ];
  }, { head, also: [state.subscribe] });
  root.append(h(doc, 'style', {}, CSS));

  // A link followed to a query or control: bring its row into view once it is drawn.
  const reveal = () => {
    const revealed = state.revealed;
    if (!revealed || handled.get(state) === revealed.nonce || !('entity' in revealed.target)) return;
    const key = entityKey(revealed.target.entity, revealed.target.app, revealed.target.name);
    const row = [...root.querySelectorAll<HTMLElement>('details[data-key]')].find((element) => element.dataset.key === key);
    if (!row) return;
    handled.set(state, revealed.nonce);
    flash(row);
  };
  reveal();
  const stops = [state.subscribe(reveal), panel.subscribe(reveal)];
  return () => {
    stops.forEach((stop) => stop());
    tab.dispose();
    panel.highlight(null);
  };
}

const CSS = `
.app + .app, .app + .section { margin-top: 18px; }
.app-line { margin: 8px 0 0; font-size: 12px; color: var(--anfra-color-muted); }
.app-line strong { color: var(--anfra-color-fg); font-weight: 600; }
.count { font-weight: normal; letter-spacing: normal; color: var(--anfra-color-muted); font-variant-numeric: tabular-nums; }
.entity > summary { align-items: flex-start; }
.entity > summary > .icon { margin-top: 3px; }
.entry { flex: 1; min-width: 0; display: flex; flex-direction: column; }
.line { flex: 1; min-width: 0; display: flex; align-items: center; gap: 6px; }
.entry .error-text { margin: 2px 0 0; font-size: 12px; }
.rows, .time { flex: none; font-size: 11px; color: var(--anfra-color-muted); font-variant-numeric: tabular-nums; }
.time { color: var(--anfra-color-fg); font-weight: 600; }
.condition { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; color: var(--anfra-color-muted); }
.changed { flex: none; display: grid; place-items: center; width: 16px; height: 16px; border-radius: 4px; background: var(--warn-bg); color: var(--warn); font-size: 10px; font-weight: 600; }
.indicator.targets { width: 32px; }
.inline > summary { display: inline-flex; padding: 0; }
.inline > .collapsible-body { padding: 2px 0 4px 4px; }
.places, .chips, .list { list-style: none; margin: 0; padding: 0; }
.places li { display: flex; align-items: center; gap: 2px; padding-left: 6px; border-radius: 4px; }
.places li:hover { background: var(--hover); }
.places li > span:first-child { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.chips { display: flex; flex-wrap: wrap; gap: 4px; }
.chips li { max-width: 100%; overflow: hidden; text-overflow: ellipsis; padding: 0 6px; border: 1px solid var(--anfra-color-border); border-radius: 4px; font-size: 11px; color: var(--anfra-color-muted); }
.chips li.selected { color: var(--anfra-color-fg); font-weight: 600; border-color: var(--anfra-color-accent); background: var(--accent-bg); }
.list, .requests { margin-top: 2px; border: 1px solid var(--anfra-color-border); border-radius: 6px; font-size: 12px; }
.list > * + * { border-top: 1px solid var(--anfra-color-border); }
.item > summary { padding: 4px 8px; }
.item > summary .name { font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.item > .collapsible-body { padding: 2px 8px 8px 28px; }
.filters li { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 8px; padding: 4px 8px; }
.filters .field { font-weight: 600; overflow-wrap: anywhere; }
.filters .link { padding: 0; font-size: 12px; }
.pending { color: var(--warn); font-size: 11px; }
.segments.small { margin-bottom: 6px; }
.segments.small button { padding: 1px 6px; font-size: 11px; }
.requests .row:last-child { border-bottom: 0; }
.request > summary { padding: 4px 8px; }
.request > .collapsible-body { padding: 2px 8px 8px 28px; }
.preview-count { margin: 6px 0 4px; font-size: 11px; color: var(--anfra-color-muted); }
.lossy { margin: 0 0 6px; font-size: 12px; }
`;
