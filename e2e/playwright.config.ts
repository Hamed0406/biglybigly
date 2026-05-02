import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  retries: 1,
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8082',
    screenshot: 'only-on-failure',
  },
  projects: [
    { name: 'chromium', use: { browserType: 'chromium' } },
  ],
});
