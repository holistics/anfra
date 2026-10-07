import { build, type Plugin } from 'esbuild';
import { defineConfig } from 'tsup';

/**
 * `anfra-sdk:frame-script`: the frame script (`anfra-sdk/app`'s bootstrap and app runtime as one
 * classic script, which a host injects into a Data App's frame), built afresh each time `host` is,
 * so a host always carries the app it was built with, in watch mode too.
 */
const frameScript: Plugin = {
  name: 'frame-script',
  setup (b) {
    b.onResolve({ filter: /^anfra-sdk:frame-script$/ }, (args) => ({ path: args.path, namespace: 'frame-script' }));
    b.onLoad({ filter: /.*/, namespace: 'frame-script' }, async () => {
      const result = await build({
        entryPoints: ['src/app/frame.ts'],
        bundle: true,
        format: 'iife',
        target: 'es2020',
        minify: true,
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

export default defineConfig({
  entry: {
    common: 'src/common/index.ts',
    app: 'src/app/index.ts',
    host: 'src/host/index.ts',
    api: 'src/api/index.ts',
  },
  format: ['esm'],
  dts: true,
  clean: true,
  target: 'es2020',
  esbuildPlugins: [frameScript],
});
