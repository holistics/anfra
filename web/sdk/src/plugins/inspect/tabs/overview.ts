/**
 * Overview: the tab the panel opens on. Is this app healthy, and where to look: a status bar of
 * the queries' states, a trust card per app, stat cards, the issues, what the reader changed, and
 * the environment the frame runs in. Every count and issue links to where it is.
 */
import type { InspectedApp } from '../../../common/types';
import type { TabContext } from '../../../host/contributions';
import type { Snapshot } from '../../../host/panel';
import { changesOf } from '../model/changes';
import { formatAgo, formatTime, plural, usageOf } from '../model/format';
import { controlTargets } from '../model/inputs';
import { type Issue, issuesOf, queryCounts } from '../model/issues';
import { queriesOf, type RequestEntry } from '../model/requests';
import {
  appTrust, LEVELS, TRUST_LEVELS, type Trust,
} from '../model/trust';
import { QUERIES_TAB, type QueriesFilter, type RevealTarget } from '../model/types';
import { stateOf } from '../state';
import { PLUGIN_VERSION } from '../../version';
import {
  type Children, badge, collapsible, control, drawTab, fields, h, icon, link, note, popover, trustBars,
} from '../view';

/** How many issues the list shows before "Show all". */
const FIRST_ISSUES = 5;

/** How often "how long ago" moves on. */
const TICK_MS = 30_000;

const CSS = `
.overview { display: flex; flex-direction: column; gap: 10px; padding-top: 8px; }
.statusbar { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; }
.statusbar .pop-trigger { gap: 5px; cursor: pointer; }
.count { border: 0; font-variant-numeric: tabular-nums; cursor: pointer; }
.count:hover { filter: brightness(0.95); }
.count.executing::before { animation: pulse 1.2s ease-in-out infinite; }
.tile { padding: 12px 14px; border-radius: 10px; background: var(--subtle); min-width: 0; }
.eyebrow { margin: 0; display: flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 500; color: var(--anfra-color-muted); }
.eyebrow strong { color: var(--anfra-color-fg); }
.eyebrow .pop-trigger { color: var(--anfra-color-muted); cursor: help; }
.trust-row { display: flex; align-items: center; gap: 12px; margin-top: 8px; }
.trust-level { margin: 0; font-size: 18px; font-weight: 600; line-height: 1.2; }
.trust-level span { display: block; font-size: 13px; font-weight: 500; }
.trust-why { margin: 2px 0 0; font-size: 12px; color: var(--anfra-color-muted); }
.stats { display: grid; gap: 10px; }
.stats.three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.stats.two { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.stat { position: relative; display: flex; flex-direction: column; align-items: flex-start; gap: 2px; border: 0; text-align: left; }
.stat:hover:not(:disabled) { background: var(--hover); }
.stat .go { position: absolute; top: 12px; right: 12px; color: var(--anfra-color-muted); opacity: 0; }
.stat:hover .go, .stat:focus-visible .go { opacity: 1; }
.stat-value { font-size: 18px; font-weight: 600; font-variant-numeric: tabular-nums; }
.stat-detail { width: 100%; font-size: 12px; color: var(--anfra-color-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.issues { list-style: none; margin: 4px 0 0; padding: 0; }
.issue { display: flex; align-items: flex-start; gap: 8px; width: 100%; border: 0; border-radius: 6px; background: none; padding: 5px 6px; text-align: left; overflow-wrap: anywhere; }
.issue:hover { background: var(--hover); }
.issue .mark { flex: none; width: 8px; height: 8px; margin-top: 6px; border-radius: 50%; }
.issue .mark.error { background: var(--anfra-color-danger); } .issue .mark.warn { background: var(--warn); }
.overview > h5 { margin: 6px 0 0; }
.tile.collapsible { padding: 6px 12px; }
.tile.collapsible > summary { font-weight: 500; }
.tile.collapsible > summary .muted { margin-left: auto; font-weight: normal; font-size: 12px; }
.changes { list-style: none; margin: 0; padding: 0; }
.change { display: flex; align-items: center; gap: 2px; padding: 3px 0; }
.change > .link { flex: 1; min-width: 0; flex-direction: column; align-items: flex-start; color: var(--anfra-color-fg); font-size: 12px; text-align: left; }
.change .from { text-decoration: line-through; color: var(--anfra-color-muted); }
.warning { margin: 0 0 8px; padding: 6px 8px; border-radius: 6px; background: var(--warn-bg); color: var(--warn); font-size: 12px; }
.levels { list-style: none; margin: 6px 0 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
.levels > li { display: flex; align-items: flex-start; gap: 8px; padding: 3px 6px; border-radius: 6px; }
.levels > li > .bars { margin-top: 3px; }
.levels > li[aria-current] { background: var(--hover); }
.levels ul { margin: 1px 0 0; padding-left: 14px; color: var(--anfra-color-muted); }
.trust-pop { display: block; width: 300px; }
.trust-pop header { display: flex; align-items: center; gap: 8px; font-weight: 600; }
`;

export default function overviewTab (root: ShadowRoot, context: TabContext): () => void {
  const doc = context.document;
  const { panel, mount } = context;
  const state = stateOf(mount);
  let allIssues = false;
  const reveal = (target: RevealTarget) => state.reveal(panel, target);
  const showQueries = (filter: QueriesFilter) => reveal({ tab: QUERIES_TAB, filter });

  /** Where this app stands and why, then every level with what it means. */
  const trustLevels = (trust: Trust) => h(doc, 'span', { class: 'trust-pop', 'data-testid': 'trust-levels' },
    h(doc, 'header', {}, trustBars(doc, trust, 'md'), h(doc, 'span', { class: `tone-${trust.tone}` }, `Level ${trust.level} · ${TRUST_LEVELS[trust.level].name}`)),
    h(doc, 'span', { class: 'trust-why' }, trust.text),
    h(doc, 'p', { class: 'trust-why' }, 'How far the numbers trace back to the semantic layer:'),
    h(doc, 'ol', { class: 'levels' }, LEVELS.map((level) => {
      const { name, points, tone } = TRUST_LEVELS[level];
      return h(doc, 'li', { 'aria-current': level === trust.level ? 'true' : undefined },
        trustBars(doc, { level, tone, text: '' }, 'sm'),
        h(doc, 'span', {}, h(doc, 'strong', {}, `${level}. ${name}`), h(doc, 'ul', {}, points.map((point) => h(doc, 'li', {}, point)))));
    })));

  function statusBar (apps: InspectedApp[]): HTMLElement {
    const counts = queryCounts(apps);
    const items = [
      { n: counts.success, label: 'ok', dot: 'success', tone: 'success', filter: 'all' },
      { n: counts.error, label: 'error', dot: 'error', tone: 'danger', filter: 'errors' },
      { n: counts.executing, label: 'running', dot: 'executing', tone: 'highlight', filter: 'all' },
      { n: counts.idle, label: 'idle', dot: 'idle', tone: undefined, filter: 'all' },
      { n: counts.dirty + counts.dirtyControls, label: 'dirty', dot: 'dirty', tone: 'warning', filter: 'dirty' },
    ] as const;
    const shown = items.filter((item) => item.n > 0);
    const trust = apps.length ? apps.map(appTrust).reduce((lowest, t) => (t.level < lowest.level ? t : lowest)) : undefined;
    return h(doc, 'div', { class: 'statusbar', 'data-testid': 'status-bar' },
      trust && popover(doc, [h(doc, 'span', { class: `tone-${trust.tone}` }, `L${trust.level}`), TRUST_LEVELS[trust.level].name], () => trustLevels(trust), {
        ...badge('neutral', 'pop-trigger'), 'data-key': 'status:trust', 'data-testid': 'status-trust', 'aria-label': `Trust level ${trust.level}: ${TRUST_LEVELS[trust.level].name}`,
      }),
      shown.length === 0 && h(doc, 'span', { class: 'a-pill', 'data-testid': 'status-none' }, 'No queries'),
      shown.map((item) => h(doc, 'button', {
        type: 'button',
        // A status pill: its dot is the pill's own.
        class: `a-pill count ${item.dot}`,
        'data-tone': item.tone,
        'data-tip': 'Show in Queries',
        'data-key': `status:${item.label}`,
        'data-testid': `status-${item.label}`,
        onclick: () => showQueries(item.filter),
      }, `${item.n} ${item.label}`)));
  }

  function trustCard (app: InspectedApp, label: string | undefined): HTMLElement {
    const trust = appTrust(app);
    return h(doc, 'section', { class: 'tile', 'data-testid': 'trust-card' },
      h(doc, 'p', { class: 'eyebrow' },
        h(doc, 'span', {}, label && h(doc, 'strong', {}, `${label} · `), 'Trust level'),
        popover(doc, icon(doc, 'info'), () => trustLevels(trust), { 'aria-label': 'About trust levels', 'data-key': `trust:${label ?? ''}` })),
      h(doc, 'div', { class: 'trust-row' },
        trustBars(doc, trust, 'lg'),
        h(doc, 'div', {},
          h(doc, 'p', { class: `trust-level tone-${trust.tone}` }, `Level ${trust.level}`, h(doc, 'span', {}, TRUST_LEVELS[trust.level].name)),
          h(doc, 'p', { class: 'trust-why' }, trust.text))));
  }

  function stat (id: string, title: string, value: Children, detail: string, onclick?: () => void, tone?: 'warn' | 'error'): HTMLElement {
    return h(doc, 'button', {
      type: 'button',
      class: 'tile stat',
      disabled: !onclick,
      'data-key': `stat:${id}`,
      'data-testid': `stat-${id}`,
      onclick,
    },
    h(doc, 'span', { class: 'eyebrow' }, title),
    h(doc, 'span', { class: `stat-value${tone ? ` tone-${tone}` : ''}` }, value),
    h(doc, 'span', { class: 'stat-detail', 'data-tip': detail }, detail),
    onclick && h(doc, 'span', { class: 'go' }, icon(doc, 'chevron')));
  }

  function stats (apps: InspectedApp[], issues: Issue[], requests: readonly RequestEntry[]): HTMLElement[] {
    const counts = queryCounts(apps);
    const errors = issues.filter((issue) => issue.level === 'error').length;
    const warnings = issues.length - errors;
    const controls = apps.reduce((n, app) => n + Object.keys(app.controls).length, 0);
    const unapplied = counts.dirtyControls;
    const last = requests.findLast((entry) => entry.call === 'core.submitQuery' && entry.status !== 'pending');
    const lastQuery = last?.aql && last.dataset ? queriesOf({ ...last, dataset: last.dataset, aql: last.aql }, apps)[0] : undefined;
    const first = issues[0];
    const queries = [
      `${counts.success} ok`,
      counts.error && `${counts.error} failed`,
      counts.executing && `${counts.executing} running`,
      counts.idle && `${counts.idle} idle`,
    ].filter(Boolean).join(' · ');
    return [
      h(doc, 'div', { class: 'stats three' },
        stat('issues', 'Issues', String(issues.length), issues.length ? `${plural(errors, 'error')} · ${plural(warnings, 'warning')}` : 'None found',
          // Failed queries are what Queries' Errors filter lists; warnings have no such list.
          counts.error ? () => showQueries('errors') : first ? () => reveal(first.target) : undefined,
          errors ? 'error' : warnings ? 'warn' : undefined),
        stat('queries', 'Queries', String(counts.total), queries, () => showQueries('all'), counts.error ? 'error' : undefined),
        stat('controls', 'Controls', String(controls), unapplied ? `${unapplied} unapplied` : 'All applied', () => showQueries('all'), unapplied ? 'warn' : undefined)),
      h(doc, 'div', { class: 'stats two' },
        stat('last-request', 'Last request', last ? h(doc, 'span', { 'data-tip': formatTime(last.at) }, formatAgo(last.at)) : '—',
          last ? (lastQuery?.name ?? 'no query') : 'Nothing has run yet',
          lastQuery && (() => reveal({ entity: 'query', ...lastQuery })),
          last?.status === 'error' ? 'error' : undefined),
        stat('last-duration', 'Last run duration', last?.durationMs !== undefined ? `${last.durationMs} ms` : '—',
          !last ? '—' : last.status !== 'ok' ? last.status : last.fromCache ? 'from cache' : 'from database')),
    ];
  }

  function issuesList (issues: Issue[]): Children {
    if (issues.length === 0) return undefined;
    const shown = allIssues ? issues : issues.slice(0, FIRST_ISSUES);
    return [
      h(doc, 'h5', {}, 'Issues'),
      h(doc, 'ul', { class: 'issues', 'data-testid': 'issues' }, shown.map((issue, i) => h(doc, 'li', {},
        h(doc, 'button', {
          type: 'button',
          class: 'issue',
          'data-key': `issue:${i}`,
          'data-testid': 'issue',
          onclick: () => reveal(issue.target),
        }, h(doc, 'span', { class: `mark ${issue.level}`, role: 'img', 'aria-label': issue.level === 'error' ? 'Error' : 'Warning' }), issue.message)))),
      issues.length > FIRST_ISSUES && link(doc, allIssues ? 'Show fewer' : `Show all ${issues.length}`, () => {
        allIssues = !allIssues;
        tab.redraw();
      }, { 'data-key': 'issues:all', 'data-testid': 'issues-all' }),
    ];
  }

  /** What the reader changed from the app as declared: controls off their declared default. */
  function changes (snapshot: Snapshot): HTMLElement {
    const { apps, structure } = snapshot;
    const list = changesOf(apps);
    return collapsible(doc, 'changes', ['Changes from the original', h(doc, 'span', { class: 'muted' }, list.length ? plural(list.length, 'change') : 'none')], () => (list.length === 0
      ? h(doc, 'p', { class: 'muted' }, 'Every control is still as the app declared it.')
      : h(doc, 'ul', { class: 'changes', 'data-testid': 'changes' }, list.map((change) => {
        const app = apps[change.app] as InspectedApp;
        const targets = controlTargets(app, change.name).map((target) => target.query);
        const show = () => reveal({ entity: 'control', app: change.app, name: change.name });
        return h(doc, 'li', { class: 'change', 'data-testid': 'change' },
          link(doc, [
            h(doc, 'strong', {}, `${apps.length > 1 ? `App ${change.app + 1} · ` : ''}${change.name}`),
            h(doc, 'span', {}, h(doc, 'span', { class: 'from' }, change.from), ' → ', change.to),
          ], show, { 'data-tip': targets.length ? `Applies to ${targets.join(', ')}` : 'Applies to no query', 'data-key': `change:${change.app}/${change.name}` }),
          usageOf(structure, 'control', change.app, change.name) && h(doc, 'button', {
            type: 'button',
            ...control('row-action'),
            'data-tip': 'Highlight on the page',
            'aria-label': 'Highlight on the page',
            'data-testid': 'change-locate',
            onclick: () => panel.locate({ app: change.app, kind: 'control', name: change.name }),
          }, icon(doc, 'locate')),
          h(doc, 'button', {
            type: 'button', ...control('row-action'), 'data-tip': 'Show in Queries', 'aria-label': 'Show in Queries', 'data-testid': 'change-show', onclick: show,
          }, icon(doc, 'query')));
      }))), { class: 'tile' });
  }

  function environment (apps: InspectedApp[]): HTMLElement {
    const { user } = mount;
    const datasets = Object.values(mount.datasets).map((dataset) => dataset.label || dataset.name);
    // An app without a timezone of its own runs in the tenant's, which the host doesn't know.
    const label = (index: number) => (apps.length > 1 ? `App ${index + 1}` : 'the app');
    const mismatched = apps.flatMap((app, index) => (app.timezone && app.timezone !== user.timezone ? [`${label(index)} runs in ${app.timezone}`] : []));
    return collapsible(doc, 'environment', 'Environment', () => [
      mismatched.length > 0 && h(doc, 'p', { class: 'warning', role: 'note', 'data-testid': 'timezone-warning' },
        `The reader is in ${user.timezone}, but ${mismatched.join(', ')}: dates are computed in the app's timezone.`),
      fields(doc, [
        ['SDK', PLUGIN_VERSION],
        ['Reader', `${user.name} · ${user.email} · ${user.role}`],
        ['Reader timezone', user.timezone],
        ...apps.map((app, index) => [apps.length > 1 ? `App ${index + 1} timezone` : 'App timezone', app.timezone || 'tenant default'] as const),
        ['Generated SQL', user.permissions.canViewGeneratedSql ? 'can view' : 'hidden'],
        ['Export', user.permissions.canExportData ? 'allowed' : 'not allowed'],
        // The log keeps the newest 200, so this stops there.
        ['Requests', `${state.requests.entries().length} since the app loaded`],
        [`Datasets (${datasets.length})`, datasets.join(', ') || 'none'],
      ], { 'data-testid': 'environment' }),
    ], { class: 'tile' });
  }

  const tab = drawTab(root, context, () => {
    const { snapshot } = panel;
    if (!snapshot) return note(doc, 'Waiting for the Data App…', { 'data-testid': 'inspect-waiting' });
    const { apps } = snapshot;
    const issues = issuesOf(snapshot);
    return h(doc, 'div', { class: 'overview' },
      statusBar(apps),
      apps.length === 0
        ? note(doc, "This Data App hasn't created an SDK app.", { 'data-testid': 'no-apps' })
        : apps.map((app, index) => trustCard(app, apps.length > 1 ? `App ${index + 1}` : undefined)),
      stats(apps, issues, state.requests.entries()),
      issuesList(issues),
      changes(snapshot),
      environment(apps));
  }, { also: [state.subscribe] });
  root.append(h(doc, 'style', {}, CSS));
  const timer = setInterval(tab.redraw, TICK_MS);
  return () => {
    clearInterval(timer);
    tab.dispose();
  };
}
