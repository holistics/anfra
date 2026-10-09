/**
 * @holistics/anfra-sdk/api: the core API, for trusted code. A client generated from anfra core's spec, a
 * Backend over it (`coreApiBackend`), and what provisions a Data App (`loadDatasets`). A host's
 * page uses it; a Data App's frame never does.
 */
export { coreClient, type CoreClient } from './client';
export {
  coreApiBackend, toQueryInput, SUGGESTION_LIMIT, type CoreApiBackendOptions,
} from './backend';
export { loadDatasets, toDescriptor, type LoadedDatasets } from './datasets';
export { toSdkError, type ApiErrorBody } from './errors';
export type { paths, components } from './schema';
