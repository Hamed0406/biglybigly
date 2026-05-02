import { test, expect } from '@playwright/test';

test.describe('Setup Flow', () => {
  test('setup page redirects or shows form', async ({ page }) => {
    const response = await page.goto('/');
    expect(response?.status()).toBeLessThan(500);

    // If setup is needed, the app may redirect to a setup page
    const url = page.url();
    if (url.includes('setup')) {
      await expect(page.getByRole('heading')).toBeVisible();
    } else {
      // Already set up — dashboard loads
      await expect(page.locator('h1')).toContainText('Dashboard');
    }
  });
});
