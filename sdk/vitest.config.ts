import { defineConfig } from 'vitest/config';

// Tests run in jsdom (the sandbox installs onto a window) under a fixed timezone set by the `test`
// script, the same environment hdev's root config gave this package before it was extracted.
export default defineConfig({
  test: {
    environment: 'jsdom',
  },
});
