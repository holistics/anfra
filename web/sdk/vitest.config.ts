import { defineConfig } from 'vitest/config';

// Tests run in jsdom (the sandbox installs onto a window) under a fixed timezone set by the `test`
// script. The frame script and the plugins' frame scripts are the build's (tsup.config.ts): a test that needs one passes its own,
// or builds it (app/bundle.test.ts).
export default defineConfig({
  resolve: {
    alias: [
      { find: /^anfra-sdk:.*$/, replacement: new URL('./src/host/frameScript.none.ts', import.meta.url).pathname },
      // The examples import the SDK by its package names, as a plugin's own code does.
      { find: /^@holistics\/anfra-sdk\/(.*)$/, replacement: `${new URL('./src/', import.meta.url).pathname}$1/index.ts` },
    ],
  },
  test: {
    environment: 'jsdom',
    // anfra-design's class layer, which plugin tabs import as text (`?raw`): vitest leaves out CSS it doesn't process.
    css: { include: [/[\\/]design[\\/]components\.css/] },
  },
});
