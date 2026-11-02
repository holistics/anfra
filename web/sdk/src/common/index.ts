/**
 * @holistics/anfra-sdk/common: the contract the other entrypoints share. Types, the error classes a Backend
 * rejects with, and the bridge's messages. No DOM and no network code: all of it may end up in a
 * Data App's frame.
 */
export {
  DataAppError, ValidationError, GENERIC_CODES, callFailed, connectionFailed, forbidden, fromHttpStatus, hasErrorCode, invalidRequest, isDataAppError,
  type ApiErrorCode, type DataAppErrorInit, type ErrorCode, type ErrorContextEntry, type ErrorDetails, type ErrorScope, type FromHttpStatusOptions, type GenericCode,
} from './errors';
export { FILTER_AGGREGATIONS } from './aggregations';
export { PROVISION_ELEMENT_ID } from './bridge';
export {
  checkTarget, type MenuEntry, type MenuTarget, type MenuTargetKind, type NodeTarget, type RowsTarget, type TargetRect,
} from './menu';
export {
  THEME_TOKENS, DEFAULT_TOKENS, DESIGN_TOKENS, designTheme, resolveTheme, type Theme, type ThemeInput, type ThemeScheme, type ThemeToken, type ThemeTokens,
} from './theme';
export type {
  BridgeError, BridgeMessage, FrameMessage, HostMessage, PluginInfo, Provision,
} from './bridge';
export type * from './types';
