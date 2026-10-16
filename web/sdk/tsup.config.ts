import { build, type Plugin } from 'esbuild';
import { defineConfig } from 'tsup';

/**
 * A virtual module whose default export is `entry` built as one classic script (an IIFE) that
 * leaves its exports in `globalName`, built afresh each time the module importing it is, in watch
 * mode too.
 */
async function buildScript (entry: string, globalName: string) {
  const result = await build({
    entryPoints: [entry],
    bundle: true,
    format: 'iife',
    globalName,
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
    loader: 'js' as const,
    watchFiles: Object.keys(result.metafile.inputs),
  };
}

/**
 * - `anfra-sdk:frame-script`: the kernel (`src/app/frame.ts`, as `__anfraKernel`), which a host
 *   injects into a Data App's frame, so a host always carries the app runtime it was built with.
 * - `anfra-sdk:setup/<plugin>`: a plugin's frame setup (`src/plugins/<plugin>/frame.ts`, as
 *   `__anfraPlugin`), injected after the kernel.
 * - `anfra-sdk:body/<plugin>`: a plugin's frame body (`src/plugins/<plugin>/body.ts`, as
 *   `__anfraBody`), sent to a frame on its first use, so frames that never use it don't carry it.
 */
const scripts: Plugin = {
  name: 'anfra-sdk-scripts',
  setup (b) {
    b.onResolve({ filter: /^anfra-sdk:/ }, (args) => ({ path: args.path, namespace: 'anfra-sdk' }));
    b.onLoad({ filter: /.*/, namespace: 'anfra-sdk' }, (args) => {
      if (args.path === 'anfra-sdk:frame-script') return buildScript('src/app/frame.ts', '__anfraKernel');
      const match = /^anfra-sdk:(setup|body)\/([a-z][a-z0-9-]*)$/.exec(args.path);
      if (!match) throw new Error(`No such script: ${args.path}`);
      const [, kind, plugin] = match;
      return kind === 'setup'
        ? buildScript(`src/plugins/${plugin}/frame.ts`, '__anfraPlugin')
        : buildScript(`src/plugins/${plugin}/body.ts`, '__anfraBody');
    });
  },
};

export default defineConfig((options) => ({
  entry: {
    common: 'src/common/index.ts',
    app: 'src/app/index.ts',
    host: 'src/host/index.ts',
    api: 'src/api/index.ts',
    plugins: 'src/plugins/index.ts',
    testing: 'src/testing/index.ts',
    build: 'src/build/index.ts',
  },
  format: ['esm'],
  dts: true,
  // Not in watch mode (make dev): a clean would leave dist/ empty while it rebuilds, and the
  // appserve dev server, reading the SDK from dist/, would fail to resolve it meanwhile.
  clean: !options.watch,
  target: 'es2020',
  // `build` loads esbuild only when it runs, from the plugin author's own install.
  external: ['esbuild'],
  esbuildPlugins: [scripts],
}));
