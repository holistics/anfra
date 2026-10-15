// The kernel a host injects into a Data App's frame, built as an IIFE that leaves its exports in
// `__anfraKernel`: the host's frame script calls `bootstrap` once the plugins' setups are in hand,
// which provisions the SDK and installs `Anfra` before the definition's first script runs.
export { bootstrap } from './bootstrap';
