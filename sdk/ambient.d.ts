/**
 * Ambient declaration for the sandbox global.
 *
 * The provisioner sets up an SDK and installs it as `Anfra` before author code runs, so an
 * author — or an agent — writes `Anfra.createApp({})` and no setup at all. Include
 * this file in the sandbox's TypeScript config; it is also the entry point an agent reads to reach
 * the surface, which is `Sdk` and the types it leads to.
 *
 * A query is a dataset and an AQL body: the SDK does not parse the AQL, so a syntax error or an
 * unknown field arrives as a `QueryError` at execute rather than at the declaring line. See
 * DESIGN.md for the shape of a whole app and CONTEXT.md for what each term means.
 */
import type { Sdk } from './src/sdk';

declare global {
  const Anfra: Sdk;

  interface Window {
    Anfra: Sdk;
  }
}

export {};
