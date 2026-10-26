// The frame script, built into `host` by the build's `anfra-sdk:frame-script` plugin
// (tsup.config.ts), so a host always injects the app it was built with.
import frameScript from 'anfra-sdk:frame-script';
import uiBody from 'anfra-sdk:ui';

export function builtFrameScript (): string | undefined {
  return frameScript || undefined;
}

/** The kernel's UI body (the menu, the modal, the popover), sent to a frame on first use. */
export function builtUiBody (): string | undefined {
  return uiBody || undefined;
}
