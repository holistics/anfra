import { defineConfig } from 'vitest/config';

// The model and the Vite plugin's server side run in Node; the frame agent's tests opt into jsdom
// with a `@vitest-environment jsdom` comment.
export default defineConfig({
  test: { include: ['src/**/*.test.ts'], environment: 'node' },
});
