import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30_000,
  fullyParallel: false, // tests share one server + fixture DB
  use: {
    baseURL: 'http://127.0.0.1:9990',
  },
  webServer: {
    command: 'node serve-fixture.mjs',
    url: 'http://127.0.0.1:9990/nearby',
    reuseExistingServer: false,
    timeout: 30_000,
  },
});
