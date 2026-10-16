/**
 * @holistics/anfra-sdk/host: what hosts a Data App, in the page around it. A host runtime holds the
 * plugins a page installed and the middleware around their calls; it provisions each frame (the
 * definition, with the SDK, its data and the plugins' frame halves ahead of it), mounts it
 * sandboxed, and answers its calls. anfra's own plugins are at `@holistics/anfra-sdk/plugins/*`.
 * See docs/designs/sdk-plugins.md.
 */
export {
  createHost, type Host, type HostOptions, type MountOptions, type MountedDataApp,
} from './runtime';
export {
  definePlugin, type Plugin, type HostHalf, type FrameHalf, type CallDefinition, type CallHandler, type CallContext,
  type HandlerContext, type HandlerCallOptions, type CallOptions, type EventCheck, type Caller, type Middleware, type MountContext,
} from './plugin';
export { provisionDocument, composeFrameScript, type ProvisionOptions } from './provision';
