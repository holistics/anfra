import createClient, { type Client } from 'openapi-fetch';
import type { paths } from './schema';

/** The core API, typed from anfra core's spec (`api/openapi.yaml`, generated into `schema.d.ts`). */
export type CoreClient = Client<paths>;

/**
 * A client for the core API below `baseUrl`: `/api` on `anfra serve`, `/api/o/{org}` on
 * anfra-cloud. For trusted code only (a host page, a script, a test): a Data App's frame has none.
 */
export function coreClient (baseUrl: string, fetchImpl?: typeof fetch): CoreClient {
  return createClient<paths>({ baseUrl: baseUrl.replace(/\/+$/, ''), ...(fetchImpl ? { fetch: fetchImpl } : {}) });
}
