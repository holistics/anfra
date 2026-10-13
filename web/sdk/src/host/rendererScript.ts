// The renderer's script, built into `host` by the build's `anfra-sdk:renderer` plugin
// (tsup.config.ts), and sent to a frame on its first rendering.
import renderer from 'anfra-sdk:renderer';

export function builtRenderer (): string | undefined {
  return renderer || undefined;
}
