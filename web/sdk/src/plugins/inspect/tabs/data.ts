/**
 * Data: the tables behind the page, for someone who doesn't write queries. One card per query, with
 * the rows it last returned, a plain word for when it has none, and a CSV of the whole table when
 * the rows kept are all of them. Everything technical is in Queries, one click away.
 */
import type { ColumnMeta, InspectedApp, InspectedQuery } from '../../../common/types';
import type { TabContext } from '../../../host/contributions';
import { columnKind } from '../model/columns';
import { entityNodeKey, formatAgo, plural } from '../model/format';
import { declaredFilters, isActive, queryInputs } from '../model/inputs';
import { attributeRequests, type RequestEntry } from '../model/requests';
import { queryTrust } from '../model/trust';
import { stateOf, type InspectState } from '../state';
import {
  collapsible, columnIcon, drawTab, filterIndicator, flash, h, icon, lineageButton, note, table, trustIndicator,
} from '../view';

const CSS = `
.search-bar { flex: none; padding: 8px 14px 4px; }
.app-title { margin: 12px 0 6px; font-size: 13px; font-weight: 500; }
.table-card > .collapsible-body { padding-top: 0; }
/* Each card is in its own element, so the kit's gap between adjacent tiles doesn't reach them. */
.table-item + .table-item { margin-top: 8px; }
.col { display: inline-flex; align-items: center; gap: 4px; cursor: help; }
.col .icon { color: var(--anfra-color-muted); }
.count { display: flex; align-items: center; gap: 6px; margin-top: 6px; font-size: 12px; color: var(--anfra-color-muted); }
.count .row-action { margin-left: auto; }
.table-state { margin: 4px 0; font-size: 12px; color: var(--anfra-color-muted); }
`;

/** One CSV field: empty for no value, quoted when it must be, and text a spreadsheet would run as a formula kept as text. */
export function csvField (value: unknown): string {
  if (value === null || value === undefined) return '';
  let text = value instanceof Date ? value.toISOString() : typeof value === 'object' ? JSON.stringify(value) : String(value);
  if (typeof value === 'string' && /^[=+\-@\t\r]/.test(text)) text = `'${text}`;
  return /[",\r\n]/.test(text) ? `"${text.replace(/"/g, '""')}"` : text;
}

/** A table as CSV: the labels as headers, CRLF line ends, and a BOM so Excel reads it as UTF-8. */
export function toCsv (columns: readonly string[], rows: readonly (readonly unknown[])[], labels: Readonly<Record<string, string>>): string {
  const lines = [
    columns.map((column) => csvField(labels[column] || column)),
    ...rows.map((row) => columns.map((_, index) => csvField(row[index]))),
  ];
  return `﻿${lines.map((line) => line.join(',')).join('\r\n')}`;
}

/** Save `text` as a file from the host page. */
function download (doc: Document, filename: string, text: string): void {
  const win = doc.defaultView;
  if (!win) return;
  const url = win.URL.createObjectURL(new win.Blob([text], { type: 'text/csv;charset=utf-8' }));
  const a = doc.createElement('a');
  a.href = url;
  a.download = filename;
  // Some browsers only follow a link that is in the page.
  doc.body.append(a);
  a.click();
  a.remove();
  win.setTimeout(() => win.URL.revokeObjectURL(url), 0);
}

/** How many rows a result holds, or how many of them the kept rows are. */
function rowCount (shown: number, total: number | undefined): string {
  if (shown === 0) return 'No rows.';
  return total !== undefined && total > shown ? `First ${shown} of ${total} rows` : plural(shown, 'row');
}

/**
 * The request whose rows a table shows: the newest that succeeded and kept rows, on the first page
 * (a later page isn't the start of the table), and not shared with another query, which may have made it.
 */
export function latestRows (requests: readonly { entry: RequestEntry, shared: boolean }[] | undefined): RequestEntry | undefined {
  return requests?.find(({ entry, shared }) => !shared && entry.status === 'ok' && entry.preview && entry.columns && !(entry.page && entry.page > 1))?.entry;
}

/** Which reveal each mount's Data tab has brought into view, so a tab switched back to doesn't flash it again. */
const handled = new WeakMap<InspectState, number>();

export default function dataTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel } = context;
  const state = stateOf(context.mount);
  const hasLineage = context.plugins.some((plugin) => plugin.name === 'lineage');
  let search = '';

  const input = h(doc, 'input', {
    type: 'search',
    placeholder: 'Search tables',
    'aria-label': 'Search tables',
    'data-testid': 'data-search',
    oninput: () => {
      search = (input as HTMLInputElement).value;
      tab.redraw();
    },
    onkeydown: (event: Event) => {
      if ((event as KeyboardEvent).key !== 'Escape' || !search) return;
      event.preventDefault();
      setSearch('');
    },
  }) as HTMLInputElement;
  const head = h(doc, 'div', { class: 'search-bar' }, h(doc, 'label', { class: 'search' }, icon(doc, 'search'), input));
  const setSearch = (text: string) => {
    search = text;
    input.value = text;
    tab.redraw();
  };

  function header (column: string, label: string, meta: ColumnMeta | undefined): HTMLElement | string {
    if (!meta) return label;
    const lines = [
      meta.label || label,
      columnKind(meta),
      meta.isMeasure && `Type · ${meta.semantic_type}`,
      meta.adhoc && "Ad hoc: the query's own expression, not the dataset's",
    ].filter(Boolean);
    return h(doc, 'span', { class: 'col', tabindex: '0', 'data-tip': lines.join('\n'), 'data-testid': `column-${column}` }, columnIcon(doc, meta), label);
  }

  function body (name: string, query: InspectedQuery, latest: RequestEntry | undefined): Node {
    // The rows before an error aren't today's answer, so they don't show; the reason is in Queries.
    if (query.state === 'error') return note(doc, "This table couldn't load.", { class: 'table-state', 'data-testid': 'table-error' });
    if (!latest?.preview || !latest.columns) {
      return query.state === 'success'
        // Its request has left the log, or the log couldn't tell it from another query's.
        ? note(doc, "This table's rows aren't available here.", { class: 'table-state', 'data-testid': 'table-gone' })
        : note(doc, 'Loading this table…', { class: 'table-state', 'data-testid': 'table-loading' });
    }
    const { columns, preview } = latest;
    const metas = new Map(query.columns.map((column) => [column.name, column]));
    const labels = Object.fromEntries(query.columns.filter((column) => column.label).map((column) => [column.name, column.label]));
    // A paged query's first page is the whole table only when no other page follows it.
    const complete = !query.hasMore && query.page <= 1 && (latest.numRows ?? 0) <= preview.length;
    const canExport = context.mount.user.permissions.canExportData && complete;
    // When the rows came back, not when they were asked for.
    const returned = new Date(new Date(latest.at).getTime() + (latest.durationMs ?? 0));
    return h(doc, 'div', {},
      preview.length > 0 && table(doc, columns.map((column) => header(column, labels[column] || column, metas.get(column))), preview, { 'data-testid': 'table-rows' }),
      h(doc, 'div', { class: 'count', 'data-testid': 'table-count' },
        rowCount(preview.length, latest.numRows),
        h(doc, 'span', { 'aria-hidden': 'true' }, '·'),
        h(doc, 'span', { 'data-tip': returned.toLocaleString() }, formatAgo(returned)),
        canExport && h(doc, 'button', {
          type: 'button',
          class: 'row-action',
          'data-tip': 'Download CSV',
          'aria-label': 'Download CSV',
          'data-testid': 'download-csv',
          onclick: () => download(doc, `${name}.csv`, toCsv(columns, preview, labels)),
        }, icon(doc, 'download'))));
  }

  function card (app: InspectedApp, index: number, name: string, query: InspectedQuery, latest: RequestEntry | undefined): HTMLElement {
    const key = `${index}/${name}`;
    const declaration = app.declaration.queries[name];
    const active = [...declaredFilters(declaration?.aql), ...queryInputs(app, name)].filter(isActive);
    return collapsible(doc, `table:${key}`, [
      h(doc, 'span', { class: 'name' }, name),
      query.state === 'error' && h(doc, 'span', { class: 'pill error' }, "couldn't load"),
      h(doc, 'span', { class: 'spacer' }),
      filterIndicator(doc, active),
      trustIndicator(doc, queryTrust(declaration, query)),
      hasLineage && lineageButton(doc, panel, entityNodeKey(panel.snapshot?.structure?.nodes ?? [], 'query', index, name)),
      h(doc, 'button', {
        type: 'button',
        class: 'row-action',
        'data-tip': 'Query',
        'aria-label': 'Query',
        'data-testid': 'show-query',
        onclick: (event: Event) => {
          // In the summary: the click would also open or close the card.
          event.preventDefault();
          event.stopPropagation();
          state.reveal(panel, { entity: 'query', app: index, name });
        },
      }, icon(doc, 'query')),
    ], () => body(name, query, latest), {
      open: state.data.open.has(key),
      class: 'row table-card',
      onToggle: (open) => {
        if (open) state.data.open.add(key);
        else state.data.open.delete(key);
      },
    });
  }

  const tab = drawTab(root, context, () => {
    const apps = panel.snapshot?.apps;
    if (!apps) return note(doc, 'Waiting for the Data App…', { 'data-testid': 'inspect-waiting' });
    if (apps.length === 0) return note(doc, "This Data App hasn't created an SDK app.");
    if (apps.every((app) => Object.keys(app.queries).length === 0)) return note(doc, 'This app has no tables yet.', { 'data-testid': 'data-none' });
    const { byQuery } = attributeRequests(state.requests.entries(), apps);
    const term = search.trim().toLowerCase();
    return apps.map((app, index) => {
      const tables = Object.entries(app.queries).filter(([name]) => !term || name.toLowerCase().includes(term));
      return h(doc, 'section', { 'data-testid': `data-app-${index}` },
        apps.length > 1 && h(doc, 'p', { class: 'app-title' }, `App ${index + 1} · ${app.title || 'Untitled'}`),
        tables.length === 0 && term && note(doc, `No tables match "${search.trim()}".`, { 'data-testid': 'data-no-match' }),
        tables.map(([name, query]) => h(doc, 'div', { class: 'table-item', 'data-testid': `table-${index}/${name}` },
          card(app, index, name, query, latestRows(byQuery.get(`${index}/${name}`))))));
    });
  }, { head, also: [state.subscribe] });
  root.append(h(doc, 'style', {}, CSS));

  // A link to a table: bring its card into view and flash it, once per link followed.
  const reveal = () => {
    const revealed = state.revealed;
    if (!revealed || !('table' in revealed.target) || handled.get(state) === revealed.nonce) return;
    // Held until the tables are there to find: a link can land before the snapshot.
    if (!panel.snapshot) return;
    handled.set(state, revealed.nonce);
    // A search that would hide it goes.
    if (search) setSearch('');
    const key = `table:${revealed.target.app}/${revealed.target.table}`;
    // After the redraws the same link sets off, which would replace the card.
    queueMicrotask(() => {
      const found = [...root.querySelectorAll<HTMLElement>('details[data-key]')].find((element) => element.dataset.key === key);
      if (found) flash(found);
    });
  };
  reveal();
  const stops = [panel.subscribe(reveal), state.subscribe(reveal)];
  return () => {
    stops.forEach((stop) => stop());
    tab.dispose();
  };
}
