/**
 * @holistics/anfra-sdk/app: what a Data App runs on. The app runtime (an app, its queries, controls and
 * cross-filters) given a Backend, and the frame bootstrap that provisions it from its host. It
 * imports only `common`: a Data App's frame carries no API client.
 *
 * See docs/designs/data-apps.md, at the repository root, for the design.
 */
export {
  createSdk, Sdk, InstalledPlugins, VERSION, type DeprecatedFeatures,
} from './sdk';
export { installSandbox, SANDBOX_GLOBAL } from './sandbox';
export { App } from './app';
export { Query } from './query';
export { Control, Filter, DateDrillControl } from './controls';
export { ATTR as STRUCTURE_ATTRIBUTES, buildStructure, parseRef } from './structure';
export { OVERLAY_ID } from './overlay';
export { Mapping, CrossFilter, type Edge } from './mapping';
export { bootstrap, type PluginSetup } from './bootstrap';
export { type FrameKernel } from './kernel';
export {
  type FrameMenuItem, type ModalInput, type PopoverInput, type Opened,
} from './menu';
export type {
  AnfraPlugins, PluginApi, PluginMembers, PluginName,
} from './pluginTypes';
export { LAYER_ATTRIBUTE } from './layer';
export * from '../common';
