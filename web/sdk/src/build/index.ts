/**
 * @holistics/anfra-sdk/build: building an SDK plugin's frame half into the script source a host
 * provisions (`plugin.frame`). Node only, at build time; needs `esbuild`, an optional peer
 * dependency. See docs/usage/sdk-plugins.md.
 */
import type { FrameHalf } from '../host/plugin';

export interface BuildFrameHalfOptions {
  /** The setup's entry: a module whose default export takes the kernel (`(kernel: FrameKernel) => void`). */
  setup: string;
  /** The body's entry, if the plugin has one: a module whose default export is what `kernel.body()` resolves to. */
  body?: string;
  /** Minify the scripts; true unless false. */
  minify?: boolean;
}

/**
 * What a frame half may not bundle: the core API's client and the host's code. A frame reaches data
 * only over the bridge (docs/designs/data-apps.md), and these would put a way around it, or the
 * host's own code, beside the definition.
 */
const FORBIDDEN = /^(@holistics\/anfra-sdk\/(api|host|plugins|build)|openapi-fetch)(\/|$)/;

async function buildScript (entry: string, globalName: string, minify: boolean): Promise<string> {
  const { build } = await import('esbuild');
  const result = await build({
    entryPoints: [entry],
    bundle: true,
    format: 'iife',
    globalName,
    target: 'es2020',
    minify,
    // The SDK's error classes name themselves by their class: minified, they would be 'c'.
    keepNames: true,
    write: false,
    logLevel: 'silent',
    plugins: [{
      name: 'anfra-frame-half',
      setup (b) {
        b.onResolve({ filter: FORBIDDEN }, (args) => ({
          errors: [{ text: `A frame half can't import '${args.path}': the frame reaches data only over the bridge. Import types only, or move the code to the host half.` }],
        }));
      },
    }],
  });
  return result.outputFiles[0].text;
}

/** Build a frame half: the setup, and the body if there is one, as script source for `plugin.frame`. */
export async function buildFrameHalf (options: BuildFrameHalfOptions): Promise<FrameHalf> {
  const minify = options.minify !== false;
  const setup = await buildScript(options.setup, '__anfraPlugin', minify);
  return options.body ? { setup, body: await buildScript(options.body, '__anfraBody', minify) } : { setup };
}
