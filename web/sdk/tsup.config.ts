import { build, type Plugin } from 'esbuild';
import { defineConfig } from 'tsup';

/**
 * `anfra-sdk:frame-script`: the frame script (`@holistics/anfra-sdk/app`'s bootstrap and app runtime as one
 * classic script, which a host injects into a Data App's frame), built afresh each time `host` is,
 * so a host always carries the app it was built with, in watch mode too.
 */
/**
 * A virtual module whose default export is `entry` built as one classic script (an IIFE), built
 * afresh each time `host` is, in watch mode too.
 */
function scriptModule (name: string, specifier: string, entry: string): Plugin {
  const filter = new RegExp(`^${specifier}$`);
  return {
    name,
    setup (b) {
      b.onResolve({ filter }, (args) => ({ path: args.path, namespace: name }));
      b.onLoad({ filter: /.*/, namespace: name }, async () => {
        const result = await build({
          entryPoints: [entry],
          bundle: true,
          format: 'iife',
          target: 'es2020',
          minify: true,
          // Error classes name themselves by their class (`err.name === 'PermissionError'`), which
          // an author checks: minified, they would be 'c'.
          keepNames: true,
          write: false,
          metafile: true,
        });
        return {
          contents: `export default ${JSON.stringify(result.outputFiles[0].text)};`,
          loader: 'js',
          watchFiles: Object.keys(result.metafile.inputs),
        };
      });
    },
  };
}

/**
 * `anfra-sdk:frame-script`: the frame script (`@holistics/anfra-sdk/app`'s bootstrap and app runtime as one
 * classic script, which a host injects into a Data App's frame), so a host always carries the app
 * it was built with.
 */
const frameScript = scriptModule('frame-script', 'anfra-sdk:frame-script', 'src/app/frame.ts');

/**
 * `anfra-sdk:renderer`: the renderer (`src/render`), which a host sends to a frame on its first
 * rendering, so frames that never render don't carry it.
 */
const rendererScript = scriptModule('renderer', 'anfra-sdk:renderer', 'src/render/index.ts');

export default defineConfig((options) => ({
  entry: {
    common: 'src/common/index.ts',
    app: 'src/app/index.ts',
    host: 'src/host/index.ts',
    api: 'src/api/index.ts',
  },
  format: ['esm'],
  dts: true,
  // Not in watch mode (make dev): a clean would leave dist/ empty while it rebuilds, and the
  // appserve dev server, reading the SDK from dist/, would fail to resolve it meanwhile.
  clean: !options.watch,
  target: 'es2020',
  esbuildPlugins: [frameScript, rendererScript],
}));
