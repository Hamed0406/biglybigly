import { test, expect } from '@playwright/test';

const endpoints = [
  '/api/modules',
  '/api/dashboard',
  '/api/dnsfilter/stats',
  '/api/netmon/flows',
  '/api/sysmon/snapshots',
  '/api/urlcheck/urls',
];

test.describe('API Health', () => {
  for (const endpoint of endpoints) {
    test(`${endpoint} responds`, async ({ request }) => {
      const response = await request.get(endpoint);
      expect(response.status()).toBeLessThan(500);
    });
  }
});
