// Scenarios: what the API answers in a frame, so a screen can be shown in each of its states
// (signed in with data, empty, failing, no access) without a backend. A scenario maps op names to
// answers, and extends another (by default the one named `default`), so each states only what
// differs.

/** What a mock is asked: the op, the org it was addressed to (`/api/o/<org>/<op>`), its input. */
export interface MockRequest {
  op: string;
  org?: string;
  body: unknown;
}

/** An answer other than a success: `{ $status: 403, $body: { code: 'forbidden', … } }`. */
export interface MockResponse {
  $status: number;
  $body?: unknown;
  /** Answer after this long, to show a screen's loading state. */
  $delay?: number;
}

/** An answer, or a function of the request that answers it. */
export type Mock = unknown;

export interface Scenario {
  label?: string;
  description?: string;
  /** The scenario this one adds to; `default` unless it is `default`, or this is `false`. */
  extends?: string | false;
  ops: Record<string, Mock>;
}

export type Scenarios = Record<string, Scenario>;

export const DEFAULT_SCENARIO = 'default';

/** The ops a scenario answers, with those of the scenarios it extends under them. */
export function scenarioOps (scenarios: Scenarios, name: string): Record<string, Mock> {
  const chain: Scenario[] = [];
  const seen = new Set<string>();
  let at: string | false | undefined = name;
  while (at && !seen.has(at) && scenarios[at]) {
    seen.add(at);
    const s: Scenario = scenarios[at];
    chain.unshift(s);
    at = s.extends ?? (at === DEFAULT_SCENARIO ? false : DEFAULT_SCENARIO);
  }
  return Object.assign({}, ...chain.map((s) => s.ops));
}

/** The op an API address names: `/api/o/acme/apps.list` is `apps.list` in the org `acme`. */
export function opOf (pathname: string, base = '/api'): { op: string, org?: string } | undefined {
  if (!pathname.startsWith(`${base}/`)) return undefined;
  const rest = pathname.slice(base.length + 1);
  const org = /^o\/([^/]+)\/(.+)$/.exec(rest);
  return org ? { op: org[2], org: decodeURIComponent(org[1]) } : { op: rest };
}

export const isMockResponse = (v: unknown): v is MockResponse =>
  typeof v === 'object' && v !== null && typeof (v as MockResponse).$status === 'number';

/** Answer one request from a scenario's ops: the mock's value, called if it is a function. */
export async function answer (ops: Record<string, Mock>, request: MockRequest): Promise<MockResponse> {
  if (!(request.op in ops)) {
    return { $status: 501, $body: { code: 'not_mocked', message: `No mock for ${request.op} in this scenario` } };
  }
  const mock = ops[request.op];
  const value = typeof mock === 'function' ? await (mock as (r: MockRequest) => unknown)(request) : mock;
  return isMockResponse(value) ? value : { $status: 200, $body: value ?? {} };
}
