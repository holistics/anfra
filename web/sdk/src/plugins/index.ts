/**
 * @holistics/anfra-sdk/plugins: anfra's own plugins, released with the SDK. A host installs the ones
 * its Data Apps get: `core` always, the others as it can serve them.
 */
export { corePlugin, type CoreBackend } from './core';
export { exportPlugin } from './export';
export { lineagePlugin } from './lineage';
export { renderPlugin, pictureFilename, type RenderPluginOptions } from './render';
export { inspectPlugin } from './inspect';
