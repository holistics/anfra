import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import { flush, testFrame } from './testFrame';
import type { FrameKernel } from './kernel';
import type { BackendQueryResult } from '../common/types';
import ui from './ui/body';
import { isLayer } from './layer';
import { numberTypes, textTypes } from './testSupport';

let added: [string, EventListenerOrEventListenerObject][] = [];
beforeEach(() => {
  const add = window.addEventListener.bind(window);
  vi.spyOn(window, 'addEventListener').mockImplementation((type: string, listener: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions) => {
    added.push([type, listener]);
    add(type, listener, options);
  });
});

// Every frame's apps, whose queries go after each test, so no frame keeps watching the shared document.
const frames: { app: { queries: Record<string, { remove: () => void }> } }[] = [];

afterEach(async () => {
  frames.splice(0).forEach(({ app }) => Object.values(app.queries).forEach((query) => query.remove()));
  await flush();
  vi.restoreAllMocks();
  added.forEach(([type, listener]) => window.removeEventListener(type, listener));
  added = [];
  document.body.innerHTML = '';
});

const RESULT: BackendQueryResult = {
  columns: [
    { name: 'region', modelId: 'users', fieldName: 'region', label: 'Region', adhoc: false, isMeasure: false, ...textTypes },
    { name: 'revenue', modelId: 'orders', fieldName: 'amount', label: 'Revenue', adhoc: false, isMeasure: true, aggregation: 'sum', ...numberTypes },
  ],
  values: [['APAC', '10'], ['EMEA', '30']],
};

const ANNOTATIONS = [
  { kind: 'mark', anchor: { rows: [{ region: 'EMEA' }] }, label: 'Spike', detail: 'Three times the usual', severity: 'warning' },
  { kind: 'line', anchor: { column: 'revenue', value: '20' }, label: 'Mean' },
  { kind: 'band', anchor: { column: 'revenue', from: '15', to: '25' }, severity: 'info' },
  { kind: 'series', anchor: { measure: 'revenue', points: [{ at: { region: 'LATAM' }, value: '5' }] }, severity: 'critical' },
] as const;

async function frame (markup: string, setup?: (kernel: FrameKernel) => void) {
  document.body.innerHTML = markup;
  let kernel!: FrameKernel;
  const f = testFrame({
    setups: [['anomalies', (k) => { kernel = k; setup?.(k); }]],
    bodies: { anfra: ui },
    answers: { 'core.submitQuery': () => RESULT },
  });
  const app = f.anfra.createApp();
  const query = app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
  await app.execute();
  frames.push({ app });
  return { f, app, query, kernel };
}

const settle = async () => { await flush(); await flush(); };
const badgeRoot = () => document.querySelector('[data-anfra-layer] [data-part="anfra-badges"]')?.shadowRoot;
const badges = () => [...(badgeRoot()?.querySelectorAll<HTMLButtonElement>('button.badge') ?? [])];
const popover = () => document.querySelector('[data-anfra-layer] [data-part="anfra-ui"]')?.shadowRoot?.querySelector('.popover');

describe('the annotations badge', () => {
  it('counts on each marked element the kinds it doesn\'t draw: all with no attribute, none with off', async () => {
    const { query, kernel } = await frame(
      '<section id="drawn" data-anfra-query="revenue" data-anfra-annotations="mark line"></section>'
      + '<section id="plain" data-anfra-query="revenue"></section>'
      + '<section id="off" data-anfra-query="revenue" data-anfra-annotations="off"></section>'
      + '<section id="all" data-anfra-query="revenue" data-anfra-annotations="mark focus line band series"></section>',
    );
    expect(kernel.annotate(query, [...ANNOTATIONS], { result: query.result! })).toBe(true);
    await settle();

    expect(badges().map((badge) => [badge.dataset.for, badge.textContent, badge.getAttribute('aria-label'), badge.dataset.severity])).toEqual([
      ['0/revenue#0', '2', '2 annotations, highest: critical', 'critical'],
      ['0/revenue#1', '4', '4 annotations, highest: critical', 'critical'],
    ]);
  });

  it('shows none when no annotation is left, and says one annotation without a severity plainly', async () => {
    const { query, kernel } = await frame('<section data-anfra-query="revenue" data-anfra-annotations="mark"></section>');
    kernel.annotate(query, [ANNOTATIONS[0]], { result: query.result! });
    await settle();
    expect(badgeRoot()).toBeUndefined();

    kernel.annotate(query, [ANNOTATIONS[1]], { result: query.result! });
    await settle();
    expect(badges().map((badge) => badge.getAttribute('aria-label'))).toEqual(['1 annotation']);

    kernel.annotate(query, [], { result: query.result! });
    await settle();
    expect(badges()).toEqual([]);
  });

  it('follows the page: an element marked later, and a query whose result is replaced', async () => {
    const { app, query, kernel } = await frame('');
    kernel.annotate(query, [ANNOTATIONS[1]], { result: query.result! });
    await settle();
    expect(badges()).toEqual([]);

    const chart = document.createElement('div');
    chart.setAttribute('data-anfra-query', 'revenue');
    document.body.append(chart);
    await settle();
    expect(badges()).toHaveLength(1);

    chart.setAttribute('data-anfra-annotations', 'line');
    await settle();
    expect(badges()).toEqual([]);

    chart.removeAttribute('data-anfra-annotations');
    await settle();
    expect(badges()).toHaveLength(1);
    await app.refresh();
    await settle();
    expect(badges()).toEqual([]);
  });

  it('lives in the plugin layer, which renderings leave out', async () => {
    const { query, kernel } = await frame('<section data-anfra-query="revenue"></section>');
    kernel.annotate(query, [ANNOTATIONS[1]], { result: query.result! });
    await settle();
    expect(isLayer(badgeRoot()!.host.parentElement!)).toBe(true);
  });

  it('opens a popover listing the annotations with their plugin\'s rows menu items', async () => {
    const explain = vi.fn();
    const { query, kernel } = await frame('<section data-anfra-query="revenue"></section>', (k) => {
      k.menuItem({ id: 'explain', label: 'Explain this anomaly', target: 'rows', when: (target) => target.kind === 'rows' && !!target.annotations?.length, run: explain });
    });
    kernel.annotate(query, [ANNOTATIONS[0], ANNOTATIONS[1]], { result: query.result! });
    await settle();

    badges()[0].click();
    await settle();

    const items = [...popover()!.querySelector<HTMLElement>('.content')!.shadowRoot!.querySelectorAll('li')];
    expect(items.map((item) => [item.querySelector('.label')?.textContent, item.querySelector('.detail')?.textContent ?? null, item.querySelector('.plugin')?.textContent])).toEqual([
      ['Spike', 'Three times the usual', 'anomalies'],
      ['Mean', null, 'anomalies'],
    ]);
    const actions = items.map((item) => [...item.querySelectorAll('button')].map((button) => button.textContent));
    expect(actions).toEqual([['Explain this anomaly'], []]);

    items[0].querySelector('button')!.click();
    await settle();
    expect(explain).toHaveBeenCalledWith(expect.objectContaining({
      kind: 'rows', query: 'revenue', rows: [{ region: 'EMEA', revenue: '30' }], annotations: [expect.objectContaining({ label: 'Spike' })],
    }));
  });

  it('runs an action with a target at the badge, so what it opens sits by the badge', async () => {
    const explain = vi.fn();
    const { query, kernel } = await frame('<section data-anfra-query="revenue"></section>', (k) => {
      k.menuItem({ id: 'explain', label: 'Explain this anomaly', target: 'rows', run: explain });
    });
    kernel.annotate(query, [ANNOTATIONS[0]], { result: query.result! });
    await settle();
    vi.spyOn(badges()[0], 'getBoundingClientRect').mockReturnValue({
      x: 300, y: 40, width: 24, height: 18, left: 300, top: 40, right: 324, bottom: 58, toJSON: () => ({}),
    });

    badges()[0].click();
    await settle();
    popover()!.querySelector<HTMLElement>('.content')!.shadowRoot!.querySelector('button')!.click();
    await settle();
    expect(explain).toHaveBeenCalledWith(expect.objectContaining({ kind: 'rows', at: { x: 300, y: 58 } }));
  });

  it('keeps its popover open while the app changes something else', async () => {
    const { app, query, kernel } = await frame('<section data-anfra-query="revenue"></section>');
    kernel.annotate(query, [ANNOTATIONS[1]], { result: query.result! });
    await settle();
    badges()[0].click();
    await settle();
    expect(popover()).not.toBeNull();

    app.createQuery('other', { dataset: 'sales', aql: 'explore {}' });
    await app.execute();
    await settle();
    expect(popover()).not.toBeNull();
  });
});
