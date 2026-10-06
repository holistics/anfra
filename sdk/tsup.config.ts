import { defineConfig } from 'tsup';

export default defineConfig([
  // For bundlers: `import { createSdk } from 'anfra-sdk'`.
  {
    entry: { index: 'index.ts' },
    format: ['esm'],
    dts: true,
    clean: true,
    target: 'es2020',
  },
  // For a script tag: exposes the library as `AnfraSdk`, so a provisioner can create an SDK and
  // install it as the `Anfra` global without a bundler.
  {
    entry: { 'anfra-sdk': 'index.ts' },
    format: ['iife'],
    globalName: 'AnfraSdk',
    target: 'es2020',
  },
]);
