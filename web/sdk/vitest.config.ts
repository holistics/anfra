import { defineConfig } from 'vitest/config';

// Tests run in jsdom (the sandbox installs onto a window) under a fixed timezone set by the `test`
// script. The frame script and the plugins' frame scripts are the build's (tsup.config.ts): a test that needs one passes its own,
// or builds it (app/bundle.test.ts).
export default defineConfig({
  resolve: {
    alias: [
      { find: /^anfra-sdk:.*$/, replacement: new URL('./src/host/frameScript.none.ts', import.meta.url).pathname },
    ],
  },
  test: {
    environment: 'jsdom',
  },
});
