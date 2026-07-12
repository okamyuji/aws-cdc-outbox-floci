import { defineConfig } from '@playwright/test';

// APIのE2E設定。ブラウザは使わずAPIRequestContextで検証する
export default defineConfig({
  testDir: './tests',
  timeout: 90_000,
  fullyParallel: false,
  retries: 0,
  reporter: [['list']],
  use: {
    extraHTTPHeaders: { 'Content-Type': 'application/json' },
  },
});
