/**
 * anfra-sdk/common: the contract the other entrypoints share. Types, the error classes a Backend
 * rejects with, and the bridge's messages. No DOM and no network code: all of it may end up in a
 * Data App's frame.
 */
export {
  DataAppError, ValidationError, QueryError, PermissionError, TransportError,
} from './errors';
export { FILTER_AGGREGATIONS } from './aggregations';
export { PROVISION_ELEMENT_ID } from './bridge';
export type {
  BridgeError, BridgeMethod, FrameMessage, HostMessage, Provision,
} from './bridge';
export type * from './types';
