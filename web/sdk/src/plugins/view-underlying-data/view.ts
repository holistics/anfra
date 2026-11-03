/**
 * The modal's content: the point's conditions, the rows as a table, and a footer to load more of
 * them or download them all. Framework-free DOM in the modal's shadow root, on anfra-design's class
 * layer. It runs nothing itself: `fetchPage` and `download` are the frame half's.
 */
import { COMPONENT_STYLES } from '../inspect/components';
import type { BackendQueryResult, ColumnMeta } from '../../common/types';
import { PAGE_SIZE, type Plan } from './model';

export interface ViewDeps {
  plan: Plan;
  /** One page of the rows, 1-based. */
  fetchPage: (page: number, signal: AbortSignal) => Promise<BackendQueryResult>;
  /** Every row as a CSV, saved for the reader; absent when the host can't export. */
  download?: (signal: AbortSignal) => Promise<unknown>;
}

const STYLES = `
:host { display: block; height: 100%; color: var(--anfra-color-text-default, var(--anfra-color-fg)); font: var(--anfra-type-body, 400 0.875rem/1.25rem var(--anfra-font, system-ui, sans-serif)); }
* { box-sizing: border-box; }
/* The class layer's buttons set their own display, which would outrank the attribute. */
[hidden] { display: none !important; }
.vud { display: flex; flex-direction: column; height: 100%; max-height: calc(100vh - 96px); }
.chips { display: flex; flex-wrap: wrap; gap: 6px; padding: 12px 16px 0; }
.rows { flex: 1; min-height: 120px; margin: 12px 16px 0; overflow: auto; border: 1px solid var(--anfra-color-border-default, var(--anfra-color-border)); border-radius: var(--anfra-radius-lg, var(--anfra-radius, 10px)); }
table { width: max-content; min-width: 100%; border-collapse: separate; border-spacing: 0; font-variant-numeric: tabular-nums; }
th, td { padding: 6px 12px; text-align: left; white-space: nowrap; border-bottom: 1px solid var(--anfra-color-border-default, var(--anfra-color-border)); }
th { position: sticky; top: 0; z-index: 1; font: var(--anfra-type-label, 500 0.875rem/1.25rem var(--anfra-font, system-ui, sans-serif)); color: var(--anfra-color-text-weak, var(--anfra-color-fg-weak)); background: var(--anfra-color-background-secondary-default, var(--anfra-color-bg-sunken)); }
tr:last-child td { border-bottom: 0; }
td.number, th.number { text-align: right; }
td.empty { color: var(--anfra-color-text-placeholder, var(--anfra-color-muted)); }
.state { padding: 24px 16px; text-align: center; color: var(--anfra-color-text-placeholder, var(--anfra-color-muted)); }
.state.error { color: var(--anfra-color-text-danger-default, var(--anfra-color-danger)); }
footer { display: flex; align-items: center; gap: 8px; padding: 12px 16px; }
.count { flex: 1; font: var(--anfra-type-caption, 400 0.75rem/1rem var(--anfra-font, system-ui, sans-serif)); color: var(--anfra-color-text-placeholder, var(--anfra-color-muted)); }
.problem { color: var(--anfra-color-text-danger-default, var(--anfra-color-danger)); }
`;

function el<K extends keyof HTMLElementTagNameMap> (doc: Document, tag: K, attrs: Record<string, string> = {}, text?: string): HTMLElementTagNameMap[K] {
  const element = doc.createElement(tag);
  Object.entries(attrs).forEach(([name, value]) => element.setAttribute(name, value));
  if (text !== undefined) element.textContent = text;
  return element;
}

const messageOf = (err: unknown) => (err instanceof Error && err.message) || 'Something went wrong.';
const isAbort = (err: unknown) => (err as { name?: string } | null)?.name === 'AbortError';

/** A value as the reader reads it: a number in their locale, anything else as the result has it. */
export function display (value: unknown, column: ColumnMeta | undefined, format: Intl.NumberFormat): string {
  if (value === null || value === undefined) return '';
  if (column?.semantic_type === 'number' && (typeof value === 'string' || typeof value === 'number')) {
    // A number arrives as a string with every digit; formatting the string keeps them.
    if (typeof value === 'string' && !/^-?\d+(\.\d+)?(e[+-]?\d+)?$/i.test(value)) return value;
    return format.format(value as number);
  }
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/** How many rows the table holds: all there are, or the ones loaded so far. */
export function countText (shown: number, hasMore: boolean): string {
  if (!shown) return '';
  const rows = `${shown.toLocaleString()} ${shown === 1 ? 'row' : 'rows'}`;
  if (hasMore) return `Showing the first ${rows}`;
  return shown === 1 ? 'The only row' : `All ${rows}`;
}

/** Draw the modal's content into `root`; what it returns stops what is in flight. */
export function view (root: ShadowRoot, deps: ViewDeps): () => void {
  const doc = root.ownerDocument;
  const controller = new AbortController();
  const format = new Intl.NumberFormat(undefined, { maximumFractionDigits: 20 });
  const { plan } = deps;

  const style = el(doc, 'style');
  style.textContent = `${COMPONENT_STYLES}\n${STYLES}`;
  const shell = el(doc, 'div', { class: 'vud' });

  if (plan.chips.length) {
    const chips = el(doc, 'div', { class: 'chips', 'data-testid': 'vud-conditions' });
    plan.chips.forEach((chip) => {
      chips.append(el(doc, 'span', { class: 'a-badge', 'data-variant': chip.warning ? 'warning' : 'secondary' }, chip.text));
    });
    shell.append(chips);
  }

  const rowsBox = el(doc, 'div', { class: 'rows' });
  const table = el(doc, 'table');
  const head = el(doc, 'tr');
  plan.columns.forEach((column) => head.append(el(doc, 'th', { scope: 'col', ...(column.isMeasure ? { class: 'number' } : {}) }, column.label)));
  const thead = el(doc, 'thead');
  thead.append(head);
  const tbody = el(doc, 'tbody');
  table.append(thead, tbody);
  const state = el(doc, 'div', { class: 'state', 'data-testid': 'vud-state' }, 'Loading rows…');
  rowsBox.append(table, state);

  const footer = el(doc, 'footer');
  const count = el(doc, 'span', { class: 'count', 'data-testid': 'vud-count' });
  const more = el(doc, 'button', {
    type: 'button', class: 'a-button', 'data-variant': 'outline', 'data-size': 'sm', 'data-testid': 'vud-more',
  }, 'Load more');
  more.hidden = true;
  footer.append(count, more);
  let downloadButton: HTMLButtonElement | undefined;
  if (deps.download) {
    downloadButton = el(doc, 'button', {
      type: 'button', class: 'a-button', 'data-variant': 'outline', 'data-size': 'sm', 'data-testid': 'vud-download',
    }, 'Download CSV');
    footer.append(downloadButton);
  }
  const problem = el(doc, 'span', { class: 'count problem', 'data-testid': 'vud-problem' });
  problem.hidden = true;
  footer.insertBefore(problem, more);

  shell.append(rowsBox, footer);
  root.append(style, shell);

  let page = 0;
  let shown = 0;
  let loading = false;

  async function load (): Promise<void> {
    if (loading) return;
    loading = true;
    more.disabled = true;
    problem.hidden = true;
    const next = page + 1;
    try {
      const result = await deps.fetchPage(next, controller.signal);
      if (controller.signal.aborted) return;
      page = next;
      const metas = new Map((result.columns ?? []).map((column) => [column.name, column]));
      const index = new Map((result.columns ?? []).map((column, i) => [column.name, i]));
      for (const values of result.values ?? []) {
        const tr = el(doc, 'tr');
        plan.columns.forEach((column) => {
          const at = index.get(column.name);
          const value = at === undefined ? null : values[at];
          const text = display(value, metas.get(column.name), format);
          const classes = [column.isMeasure ? 'number' : '', text === '' ? 'empty' : ''].filter(Boolean).join(' ');
          tr.append(el(doc, 'td', classes ? { class: classes } : {}, text));
        });
        tbody.append(tr);
      }
      shown += result.values?.length ?? 0;
      const hasMore = result.meta?.hasMore ?? (result.values?.length ?? 0) >= PAGE_SIZE;
      state.hidden = shown > 0;
      state.textContent = 'No rows match this point.';
      count.textContent = countText(shown, hasMore);
      more.hidden = !hasMore;
    } catch (err) {
      if (isAbort(err) || controller.signal.aborted) return;
      if (page === 0) {
        state.hidden = false;
        state.classList.add('error');
        state.textContent = messageOf(err);
      } else {
        problem.hidden = false;
        problem.textContent = messageOf(err);
      }
    } finally {
      loading = false;
      more.disabled = false;
    }
  }

  more.addEventListener('click', () => { void load(); });
  downloadButton?.addEventListener('click', () => {
    const button = downloadButton!;
    button.disabled = true;
    problem.hidden = true;
    deps.download!(controller.signal)
      .catch((err: unknown) => {
        if (isAbort(err) || controller.signal.aborted) return;
        problem.hidden = false;
        problem.textContent = messageOf(err);
      })
      .finally(() => { button.disabled = false; });
  });

  void load();
  return () => controller.abort();
}
