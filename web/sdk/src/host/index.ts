/**
 * @holistics/anfra-sdk/host: what hosts a Data App, in the page around it. Provisions its frame (the
 * definition, with the SDK and its data ahead of it), mounts it sandboxed, and answers its Backend
 * calls over the bridge. The same on every host: what answers is the Backend it is given.
 */
export { mountDataApp, type MountOptions, type MountedDataApp } from './mount';
export { provisionDocument, type ProvisionOptions } from './provision';
export { serveBridge, type BridgeCallbacks, type BridgeHandle } from './bridge';
