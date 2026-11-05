import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import type { StructureNode } from '../../../common/types';
import { PLUGIN_VERSION } from '../../version';
import {
  byTestId, inspect, REVENUE_AQL, salesApp, salesSnapshot, type Harness,
} from './testSupport';

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

const OVERVIEW = 'inspect.overview';

async function overview (options: Parameters<typeof inspect>[0] = {}) {
  harness = await inspect({ tab: OVERVIEW, ...options });
  return (await harness.tab(OVERVIEW)).root;
}

/** Open a collapsible by its key, and wait for its body. */
async function open (root: ShadowRoot, key: string) {
  const details = root.querySelector<HTMLDetailsElement>(`details[data-key="${key}"]`)!;
  details.open = true;
  details.dispatchEvent(new Event('toggle'));
  await vi.waitFor(() => expect(details.querySelector('.collapsible-body')).not.toBeNull());
  return details;
}

describe('Overview', () => {
  it('counts the queries in the status bar, and opens Queries on a count', async () => {
    const root = await overview();
    expect(byTestId(root, 'status-trust')?.textContent).toBe('L2Verified, incomplete');
    expect(byTestId(root, 'status-ok')?.textContent).toBe('1 ok');
    expect(byTestId(root, 'status-error')?.textContent).toBe('1 error');
    expect(byTestId(root, 'status-dirty')).toBeNull();
    byTestId(root, 'status-error')!.click();
    expect(harness!.mounted.panel.tab).toBe('inspect.queries');
    expect(harness!.state.queries.filter).toBe('errors');
  });

  it('opens the trust levels from the status bar', async () => {
    const root = await overview();
    byTestId(root, 'status-trust')!.click();
    expect(byTestId(root, 'trust-levels')?.querySelectorAll('.levels > li')).toHaveLength(4);
    expect(byTestId(root, 'trust-levels')?.querySelector('[aria-current]')?.textContent).toContain('2. Verified, incomplete');
  });

  it('says "No queries" with nothing to count', async () => {
    const root = await overview({ snapshot: salesSnapshot([salesApp({ queries: {}, controls: {} })]) });
    expect(byTestId(root, 'status-none')?.textContent).toBe('No queries');
    expect(byTestId(root, 'trust-card')?.textContent).toContain('No queries found');
  });

  it('draws a trust card per app', async () => {
    const root = await overview();
    const card = byTestId(root, 'trust-card')!;
    expect(card.querySelector('.trust-level')?.textContent).toBe('Level 2Verified, incomplete');
    expect(card.querySelector('.trust-why')?.textContent).toBe('1 of 2 queries failed');
    harness!.post(salesSnapshot([salesApp(), salesApp()]));
    const cards = root.querySelectorAll('[data-testid="trust-card"]');
    expect(cards).toHaveLength(2);
    expect(cards[1]?.querySelector('.eyebrow')?.textContent).toBe('App 2 · Trust level');
  });

  it('draws the stat cards', async () => {
    const root = await overview();
    expect(byTestId(root, 'stat-problems')?.querySelector('.stat-detail')?.textContent).toBe('1 error · 2 warnings');
    expect(byTestId(root, 'stat-queries')?.querySelector('.stat-detail')?.textContent).toBe('1 ok · 1 failed');
    expect(byTestId(root, 'stat-controls')?.querySelector('.stat-detail')?.textContent).toBe('All applied');
    expect(byTestId(root, 'stat-last-request')?.querySelector('.stat-detail')?.textContent).toBe('Nothing has run yet');
    expect(byTestId<HTMLButtonElement>(root, 'stat-last-request')?.disabled).toBe(true);
    byTestId(root, 'stat-problems')!.click();
    expect(harness!.state.queries.filter).toBe('errors');
  });

  it('shows the last request, which opens its query', async () => {
    const root = await overview();
    await harness!.frameCall('core.submitQuery', { dataset: 'sales', aql: REVENUE_AQL, input: {} });
    const card = byTestId<HTMLButtonElement>(root, 'stat-last-request')!;
    expect(card.querySelector('.stat-value')?.textContent).toBe('< 1m ago');
    expect(card.querySelector('.stat-detail')?.textContent).toBe('revenue');
    expect(byTestId(root, 'stat-last-duration')?.querySelector('.stat-detail')?.textContent).toBe('from database');
    card.click();
    expect(harness!.mounted.panel.tab).toBe('inspect.queries');
    expect(harness!.state.queries.open.has('query:0/revenue')).toBe(true);
  });

  it('lists the problems, errors first, five until "Show all"', async () => {
    const snapshot = salesSnapshot();
    const extra: StructureNode[] = [2, 3, 4].map((i) => ({ key: String(i), kind: 'block', id: `b${i}`, problems: [`Problem ${i}.`], children: [] }));
    snapshot.structure.nodes.push(...extra);
    const root = await overview({ snapshot });
    const messages = () => [...root.querySelectorAll('[data-testid="problem"]')].map((problem) => problem.textContent);
    expect(messages()).toEqual([
      'orders failed: Unknown field orders.status',
      'month is mapped to no query, so it has no effect.',
      'data-anfra-block="trend": Another block is marked trend.',
      'data-anfra-block="b2": Problem 2.',
      'data-anfra-block="b3": Problem 3.',
    ]);
    byTestId(root, 'problems-all')!.click();
    expect(messages()).toHaveLength(6);
    expect(byTestId(root, 'problems-all')?.textContent).toBe('Show fewer');
    root.querySelectorAll<HTMLElement>('[data-testid="problem"]')[2]!.click();
    expect(harness!.mounted.panel.tab).toBe('inspect.structure');
    expect(harness!.mounted.panel.selection).toBe('1');
  });

  it('wraps a long change, and keeps the count beside the title on one line', async () => {
    const root = await overview();
    const css = [...root.querySelectorAll('style')].map((style) => style.textContent).join('\n');
    expect(css).toMatch(/\.a-button\.link \{[^}]*white-space: normal;[^}]*overflow-wrap: anywhere;/);
    expect(css).toMatch(/\.tile\.collapsible > summary \.muted \{[^}]*flex: none;[^}]*white-space: nowrap;/);
  });

  it('lists the frame\'s runtime errors among the errors, counted, to their entity or open to their stack', async () => {
    const root = await overview();
    harness!.event('anfra.error', { kind: 'app.listener', message: 'echarts is not defined', stack: 'TypeError: echarts is not defined\n    at draw' });
    harness!.event('anfra.error', { kind: 'app.listener', message: 'echarts is not defined', stack: 'TypeError: echarts is not defined\n    at draw' });
    harness!.event('anfra.error', { kind: 'app.uncaught', message: 'No access.', code: 'forbidden', scope: 'user', entity: 'orders' });
    harness!.event('anfra.error', { kind: 'call.query', message: 'not listed: the query says so', entity: 'orders' });
    const problems = () => [...root.querySelectorAll<HTMLElement>('[data-testid="problem"]')];
    await vi.waitFor(() => expect(problems()).toHaveLength(5));
    expect(problems().slice(0, 4).map((p) => p.textContent)).toEqual([
      'orders failed: Unknown field orders.status',
      'Uncaught error: No access.',
      'A subscribe() listener threw: echarts is not defined×2',
      'month is mapped to no query, so it has no effect.',
    ]);
    expect(byTestId(root, 'stat-problems')?.querySelector('.stat-value')?.textContent).toBe('5');
    problems()[2].click();
    expect(byTestId(root, 'problem-detail')?.textContent).toBe('TypeError: echarts is not defined\n    at draw');
    problems()[2].click();
    expect(byTestId(root, 'problem-detail')).toBeNull();
    problems()[1].click();
    expect(harness!.mounted.panel.tab).toBe('inspect.queries');
  });

  it('keeps the frame\'s console lines in a section of their own, collapsed', async () => {
    const root = await overview();
    expect(root.querySelector('details[data-key="console"]')).toBeNull();
    harness!.event('anfra.console', { level: 'warn', message: 'rows: 3', at: '2026-11-02T00:00:00.000Z' });
    harness!.event('anfra.console', { level: 'error', message: 'oops', at: '2026-11-02T00:00:01.000Z' });
    await vi.waitFor(() => expect(root.querySelector('details[data-key="console"]')?.textContent).toContain('2 lines'));
    const details = await open(root, 'console');
    expect([...details.querySelectorAll('.console > li')].map((li) => [li.className, li.lastChild?.textContent])).toEqual([['warn', 'rows: 3'], ['error', 'oops']]);
  });

  it('lists the controls changed from the original, to highlight or show', async () => {
    const root = await overview();
    const details = await open(root, 'changes');
    expect(details.querySelector('summary')?.textContent).toContain('1 change');
    const change = byTestId(details, 'change')!;
    expect(change.querySelector('.link')?.textContent).toBe('regionno value → is EU');
    byTestId(change, 'change-locate')!.click();
    expect(harness!.posted).toContainEqual({ type: 'anfra:event', name: 'anfra.locate', payload: { app: 0, kind: 'control', name: 'region' } });
    byTestId(change, 'change-show')!.click();
    expect(harness!.mounted.panel.tab).toBe('inspect.queries');
    expect(harness!.state.queries.open.has('control:0/region')).toBe(true);
  });

  it('says when no control changed', async () => {
    const app = salesApp();
    app.controls.region = { ...app.controls.region!, condition: { operator: 'is', values: [] } };
    const root = await overview({ snapshot: salesSnapshot([app]) });
    const details = await open(root, 'changes');
    expect(details.querySelector('summary')?.textContent).toContain('none');
    expect(details.textContent).toContain('Every control is still as the app declared it.');
  });

  it('shows the environment, warning of a timezone unlike the reader\'s', async () => {
    const root = await overview({ snapshot: salesSnapshot([salesApp({ timezone: 'Europe/Paris' })]) });
    await harness!.frameCall('core.submitQuery', { dataset: 'sales', aql: REVENUE_AQL, input: {} });
    const details = await open(root, 'environment');
    const text = [...byTestId(details, 'environment')!.querySelectorAll('dt')].map((dt) => `${dt.textContent}: ${dt.nextElementSibling?.textContent}`);
    expect(text).toEqual([
      `SDK: ${PLUGIN_VERSION}`,
      'Reader: Ada Lovelace · ada@example.com · analyst',
      'Reader timezone: Asia/Singapore',
      'App timezone: Europe/Paris',
      'Generated SQL: can view',
      'Export: allowed',
      'Requests: 1 since the app loaded',
      'Datasets (1): Sales',
    ]);
    expect(byTestId(details, 'timezone-warning')?.textContent).toBe('The reader is in Asia/Singapore, but the app runs in Europe/Paris: dates are computed in the app\'s timezone.');
  });

  it('waits for the frame', async () => {
    harness = await inspect();
    harness.mounted.panel.hide();
    harness.mounted.panel.show(OVERVIEW);
    const { root } = await harness.tab(OVERVIEW);
    expect(byTestId(root, 'inspect-waiting')).not.toBeNull();
  });
});
