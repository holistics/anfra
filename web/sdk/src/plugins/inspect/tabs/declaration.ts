/**
 * Declaration: how each app is put together. Its counts, the checks for mistakes the SDK reports
 * late or never, the declaration written back out as JS, and the wiring of controls and
 * cross-filters to queries.
 */
import type { InspectedApp } from '../../../common/types';
import type { TabContext } from '../../../host/contributions';
import {
  type Check, type CheckLevel, controlMappings, crossFilters, wiringChecks,
} from '../model/checks';
import { declarationCode } from '../model/declaration';
import type { InspectState } from '../state';
import { stateOf } from '../state';
import {
  codeBlock, collapsible, drawTab, flash, h, icon, link, note, popover, type Children,
} from '../view';

const CSS = `
.app-title { margin: 12px 0 0; font-weight: 600; }
.app-group + .app-group { margin-top: 20px; padding-top: 4px; border-top: 1px solid var(--anfra-color-border); }
.counts { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 10px; }
.hint { color: var(--anfra-color-muted); }
.checks { list-style: none; margin: 0; padding: 0; }
.check { display: flex; align-items: flex-start; gap: 8px; width: 100%; border: 0; border-radius: 4px; background: none; padding: 4px 6px; text-align: left; overflow-wrap: anywhere; }
.check:hover { background: var(--hover); }
.check.info { color: var(--anfra-color-muted); }
.check .mark { flex: none; width: 14px; text-align: center; font-weight: 600; }
.check.info .mark { color: var(--anfra-color-muted); }
.checks .flash { border-radius: 4px; }
.declaration-code .code-text { max-height: 384px; }
.wiring .collapsible-body { padding-left: 0; }
.wiring td.cell { text-align: center; }
.wiring .unmapped { color: var(--anfra-color-muted); opacity: 0.5; }
.wiring .mapped { cursor: default; }
.wiring .link { padding: 0 4px; font-size: 12px; }
`;

const MARKS: Record<CheckLevel, { mark: string, label: string, tone: string }> = {
  error: { mark: '✕', label: 'error', tone: 'tone-error' },
  warn: { mark: '⚠', label: 'warning', tone: 'tone-warn' },
  info: { mark: 'ℹ', label: 'info', tone: '' },
};

/** The last reveal each mount's Declaration brought into view, so mounting the tab again doesn't repeat it. */
const handled = new WeakMap<InspectState, number>();

export default function declarationTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel } = context;
  const state = stateOf(context.mount);

  const hint = (text: string) => popover(doc, icon(doc, 'info'), () => text, { class: 'pop-trigger hint', 'aria-label': 'About this section' });

  function counts (app: InspectedApp): HTMLElement {
    const { queries, filters, dateDrills } = app.declaration;
    const items = [
      [Object.keys(queries).length, 'queries', 'query'],
      [Object.keys(filters).length + Object.keys(dateDrills).length, 'controls', 'control'],
      [controlMappings(app).length, 'mappings', 'mapping'],
      [crossFilters(app).length, 'cross-filters', 'cross-filter'],
    ] as const;
    return h(doc, 'div', { class: 'counts', 'data-testid': 'counts' },
      items.map(([n, label, one]) => h(doc, 'span', { class: 'pill', 'data-testid': `count-${label}` }, `${n} ${n === 1 ? one : label}`)));
  }

  function checks (list: Check[], index: number): Children {
    return [
      h(doc, 'h5', {}, 'Checks', hint('Static checks on the declaration for mistakes the SDK reports late or never, such as cross-filters across datasets or controls mapped to no query. Click a row to jump to it.')),
      list.length === 0
        ? note(doc, 'No issues found.', { 'data-testid': 'checks-none' })
        : h(doc, 'ul', { class: 'checks', 'data-testid': 'checks' }, list.map((check, i) => {
          const { mark, label, tone } = MARKS[check.level];
          return h(doc, 'li', { 'data-check': `${index}:${i}` },
            h(doc, 'button', {
              type: 'button',
              class: `check ${check.level}`,
              'data-tip': `Show ${check.about.name} in Queries`,
              'data-key': `check:${index}:${i}`,
              'data-testid': `check-${index}-${i}`,
              onclick: () => state.reveal(panel, { entity: check.about.entity, app: index, name: check.about.name }),
            },
            h(doc, 'span', { class: `mark ${tone}`, role: 'img', 'aria-label': label }, mark),
            h(doc, 'span', {}, check.message)));
        })),
    ];
  }

  function declaration (app: InspectedApp, index: number): Children {
    const code = declarationCode(app);
    return [
      h(doc, 'h5', {}, 'Declaration', hint('The app\'s queries, controls and mappings, as code reconstructed from what the Data App declared.')),
      h(doc, 'div', { class: 'declaration-code', 'data-testid': `declaration-${index}` },
        codeBlock(doc, { js: { label: 'declaration', code: code || undefined, lang: 'js', empty: 'Nothing declared.' } }, 'js')),
    ];
  }

  /** Queries down, controls across: which control filters which query, and on what. */
  function matrix (app: InspectedApp, index: number): Children {
    const queries = Object.keys(app.declaration.queries);
    if (queries.length === 0) return null;
    const controls = [...Object.keys(app.declaration.filters), ...Object.keys(app.declaration.dateDrills)];
    const mappings = controlMappings(app);
    const crosses = crossFilters(app);
    const entity = (kind: 'query' | 'control', name: string) => link(doc, name, () => state.reveal(panel, { entity: kind, app: index, name }), {
      'data-tip': `Show ${name} in Queries`, 'data-testid': `wiring-${kind}-${name}`,
    });
    return collapsible(doc, `wiring:${index}`, ['Wiring', hint('Which controls and cross-filters feed which queries, as declared by the app.')], () => h(doc, 'div', { class: 'table-wrap', 'data-testid': `wiring-${index}` },
      h(doc, 'table', {},
        h(doc, 'thead', {}, h(doc, 'tr', {},
          h(doc, 'th', {}, 'query'),
          controls.map((name) => h(doc, 'th', {}, entity('control', name))),
          h(doc, 'th', {}, 'cross-filtered by'))),
        h(doc, 'tbody', {}, queries.map((query) => {
          const by = crosses.filter((c) => c.to.name === query).map((c) => c.from.name);
          return h(doc, 'tr', { 'data-testid': `wiring-row-${query}` },
            h(doc, 'td', {}, entity('query', query)),
            controls.map((control) => {
              const m = mappings.find((x) => x.from.name === control && x.to.name === query);
              return h(doc, 'td', { class: 'cell', 'data-testid': `wiring-cell-${query}-${control}` },
                m
                  ? h(doc, 'span', { class: 'mapped', 'data-tip': `${m.field}${m.aggregation ? ` (${m.aggregation})` : ''}` }, '●')
                  : h(doc, 'span', { class: 'unmapped' }, '·'));
            }),
            h(doc, 'td', { class: 'muted', 'data-testid': `wiring-by-${query}` }, by.length ? by.join(', ') : '—'));
        })))), { class: 'section wiring' });
  }

  const tab = drawTab(root, context, () => {
    const apps = panel.snapshot?.apps;
    if (!apps) return note(doc, 'Waiting for the Data App…', { 'data-testid': 'inspect-waiting' });
    if (apps.length === 0) return note(doc, 'This Data App hasn\'t created an SDK app.', { 'data-testid': 'declaration-no-apps' });
    return apps.map((app, index) => h(doc, 'section', { class: 'app-group', 'data-testid': `app-${index}` },
      apps.length > 1 && h(doc, 'p', { class: 'app-title' }, `App ${index + 1} · ${app.title || 'Untitled'}`),
      counts(app),
      checks(wiringChecks(app), index),
      declaration(app, index),
      matrix(app, index)));
  }, { also: [state.subscribe] });
  root.append(h(doc, 'style', {}, CSS));

  // A check linked to from elsewhere (Overview's issues): scrolled to and flashed, once per request.
  // A reveal shows this tab before it tells its listeners, so it is looked for on the state's change,
  // the last redraw; a panel change looks again only for a check not drawn yet (no snapshot then).
  let seen = handled.get(state);
  let missing: number | undefined;
  const reveal = () => {
    const revealed = state.revealed;
    if (!revealed || seen === revealed.nonce || !('check' in revealed.target)) return;
    const { app, check } = revealed.target;
    const row = root.querySelector<HTMLElement>(`[data-check="${app}:${check}"]`);
    if (!row) {
      missing = revealed.nonce;
      return;
    }
    seen = revealed.nonce;
    handled.set(state, seen);
    flash(row);
  };
  reveal();
  const stops = [state.subscribe(reveal), panel.subscribe(() => {
    if (missing !== undefined && missing === state.revealed?.nonce) reveal();
  })];
  return () => {
    stops.forEach((stop) => stop());
    tab.dispose();
  };
}
