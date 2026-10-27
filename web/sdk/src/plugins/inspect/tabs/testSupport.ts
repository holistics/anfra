/**
 * What `inspect`'s tab tests share: a host with `core` and `inspect`, a frame that posts a
 * snapshot, and a tab drawn into the page. Tests only.
 */
import { expect, vi } from 'vitest';
import type { BridgeMessage } from '../../../common/bridge';
import type { BackendQueryResult, ColumnMeta, InspectedApp, User } from '../../../common/types';
import { salesDataset, testUser } from '../../../app/testSupport';
import { createHost, type Host, type MountedDataApp } from '../../../host/runtime';
import type { MountContext, Plugin } from '../../../host/plugin';
import type { Snapshot } from '../../../host/panel';
import { corePlugin } from '../../core';
import { inspectPlugin } from '..';
import { stateOf, type InspectState } from '../state';

export const column = (name: string, measure = false, extra: Partial<ColumnMeta> = {}): ColumnMeta => ({
  name,
  fieldName: name,
  modelId: measure ? undefined : 'orders',
  label: name.replace(/_/g, ' '),
  adhoc: false,
  isMeasure: measure,
  aggregation: measure ? 'sum' : undefined,
  semantic_type: measure ? 'number' : 'text',
  arrow_type: measure ? 'float64' : 'utf8',
  db_type: measure ? 'DOUBLE' : 'VARCHAR',
  ...extra,
} as ColumnMeta);

export const REVENUE_AQL = 'explore sales {\n  dimensions { orders.region }\n  measures { revenue }\n}';
export const ORDERS_AQL = 'explore sales {\n  dimensions { orders.status }\n  measures { order_count }\n  filters { orders.status != \'void\' }\n}';

/**
 * One app: `revenue` (succeeded, on the page in the `trend` block, filtered by the `region`
 * control and by `orders`' selection), and `orders` (failed, not on the page). `month` is a date
 * drill mapped to nothing, so the Declaration has a warning.
 */
export function salesApp (overrides: Partial<InspectedApp> = {}): InspectedApp {
  return {
    title: 'Sales',
    timezone: 'Asia/Singapore',
    hasChanges: false,
    declaration: {
      title: 'Sales',
      queries: { revenue: { dataset: 'sales', aql: REVENUE_AQL }, orders: { dataset: 'sales', aql: ORDERS_AQL, pageSize: 50 } },
      filters: { region: { field: 'orders.region', label: 'Region', default: { operator: 'is', values: [] } } },
      dateDrills: { month: { default: 'month' } },
      mappings: [
        { kind: 'control', id: 'm1', from: { kind: 'filter', name: 'region' }, to: { kind: 'query', name: 'revenue' }, field: 'orders.region' },
        { kind: 'crossFilter', id: 'c1', from: { kind: 'query', name: 'orders' }, to: { kind: 'query', name: 'revenue' } },
      ],
    },
    queries: {
      revenue: {
        state: 'success',
        isDirty: false,
        page: 1,
        hasMore: false,
        columns: [column('region'), column('revenue', true)],
        rowCount: 2,
        selectedRowCount: 0,
        signature: 'sig-revenue',
        debug: { fromCache: false, executedAql: 'explore sales { ... }', executedSql: 'SELECT region, SUM(amount) FROM orders GROUP BY 1' },
      },
      orders: {
        state: 'error',
        isDirty: false,
        page: 1,
        hasMore: false,
        columns: [],
        rowCount: 0,
        selectedRowCount: 0,
        signature: 'sig-orders',
        error: { name: 'QueryError', message: 'Unknown field orders.status' },
      },
    },
    controls: {
      region: { kind: 'filter', condition: { operator: 'is', values: ['EU'] }, appliedCondition: { operator: 'is', values: ['EU'] }, isDirty: false },
      month: { kind: 'dateDrill', condition: { operator: 'transform_date_drill', values: ['datetrunc month'] }, isDirty: false },
    },
    ...overrides,
  } as InspectedApp;
}

export function salesSnapshot (apps: InspectedApp[] = [salesApp()]): Snapshot {
  return {
    apps,
    structure: {
      nodes: [{
        key: '0', kind: 'block', id: 'trend', label: 'Trend', problems: [], children: [
          { key: '0.0', kind: 'query', name: 'revenue', app: 0, problems: [], children: [] },
          { key: '0.1', kind: 'control', name: 'region', app: 0, problems: [], children: [] },
        ],
      }, {
        key: '1', kind: 'block', id: 'trend', problems: ['Another block is marked trend.'], children: [],
      }],
      usage: {
        'query:0/revenue': { markers: 1, blocks: ['trend'] },
        'control:0/region': { markers: 1, blocks: ['trend'] },
      },
    },
  } as Snapshot;
}

/** `revenue`'s result, as `core.submitQuery` answers it. */
export const revenueResult = {
  columns: [column('region'), column('revenue', true)],
  values: [['EU', '1200.5'], ['US', '980']],
  meta: { numRows: 2 },
  debug: { fromCache: false },
} as unknown as BackendQueryResult;

export interface Harness {
  host: Host;
  mounted: MountedDataApp;
  /** `inspect`'s state for the mount: its request log, and what the tabs keep. */
  state: InspectState;
  posted: BridgeMessage[];
  /** Post a snapshot from the frame. */
  post: (snapshot: Snapshot) => void;
  /** Make a call as the frame does, through the pipeline, so `inspect` logs it. */
  frameCall: (name: string, request: unknown) => Promise<unknown>;
  /** Draw a tab, by full id, once its code has loaded. */
  tab: (id: string) => Promise<{ root: ShadowRoot, unmount: () => void }>;
  unmount: () => void;
}

/**
 * Mount a frame, as `user` (`testUser` unless given), with `core` (answering `submitQuery` with `submitQuery`) and `inspect`, open the
 * panel at `tab` and post `snapshot`.
 */
export async function inspect (options: { snapshot?: Snapshot, tab?: string, submitQuery?: (request: unknown) => unknown, plugins?: Plugin[], user?: User } = {}): Promise<Harness> {
  const submitQuery = vi.fn(options.submitQuery ?? (() => revenueResult));
  let mount: MountContext | undefined;
  const probe: Plugin = { name: 'probe', version: '1', host: { mount: (context) => { mount = context; } } };
  const host = createHost({
    plugins: [corePlugin({ submitQuery, fieldSuggestions: vi.fn() } as never), inspectPlugin(), probe, ...(options.plugins ?? [])],
    frameScript: 'x',
  });
  const mounted = host.mount({ container: document.body, definition: '', datasets: { sales: salesDataset }, user: options.user ?? testUser });
  const posted: BridgeMessage[] = [];
  vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { posted.push(message as BridgeMessage); });
  const post = (snapshot: Snapshot) => window.dispatchEvent(new MessageEvent('message', {
    data: { type: 'anfra:event', name: 'anfra.snapshot', payload: snapshot }, source: mounted.frame.contentWindow,
  }));
  await mounted.run('inspect.toggle');
  if (options.tab) mounted.panel.show(options.tab);
  post(options.snapshot ?? salesSnapshot());
  let nextId = 1;
  const frameCall = async (name: string, request: unknown) => {
    const id = nextId++;
    window.dispatchEvent(new MessageEvent('message', { data: { type: 'anfra:call', id, name, request }, source: mounted.frame.contentWindow }));
    const answer = await vi.waitFor(() => {
      const found = posted.find((message) => message.type === 'anfra:result' && message.id === id);
      if (!found) throw new Error('no answer yet');
      return found as Extract<BridgeMessage, { type: 'anfra:result' }>;
    });
    if (!answer.ok) throw Object.assign(new Error(answer.error.message), answer.error);
    return answer.result;
  };
  const tab = async (id: string) => {
    const element = document.createElement('div');
    document.body.append(element);
    const unmount = mounted.panel.mountTab(id, element);
    // The tabs' code loads on first use.
    await vi.waitFor(() => expect(element.shadowRoot?.querySelector('.pane')).not.toBeNull());
    return { root: element.shadowRoot!, unmount };
  };
  return { host, mounted, state: stateOf(mount!), posted, post, frameCall, tab, unmount: () => mounted.unmount() };
}

/** The element with `data-testid="<id>"` under `root`. */
export const byTestId = <T extends HTMLElement = HTMLElement>(root: ParentNode, id: string) => root.querySelector<T>(`[data-testid="${id}"]`);
