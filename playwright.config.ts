import { defineConfig, devices } from '@playwright/test';

// Each test starts its own `anfra serve --apps` (with the e2e fake) on a throwaway Repo; see test/serve.ts.
export default defineConfig({
  testDir: 'test/e2e',
  globalSetup: './test/global-setup.ts',
  timeout: 60_000,
  fullyParallel: true,
  workers: process.env.CI ? 2 : undefined,
  reporter: process.env.CI ? 'line' : 'list',
  use: {
    ...devices['Desktop Chrome'],
    trace: 'retain-on-failure',
  },
});
