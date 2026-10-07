// The frame script, built into `host` by the build's `anfra-sdk:frame-script` plugin
// (tsup.config.ts), so a host always injects the app it was built with.
import frameScript from 'anfra-sdk:frame-script';

export function builtFrameScript (): string | undefined {
  return frameScript || undefined;
}
