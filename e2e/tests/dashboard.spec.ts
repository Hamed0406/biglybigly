import { test, expect } from '@playwright/test';

test.describe('Dashboard', () => {
  test('page loads and shows heading', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('h1')).toContainText('Dashboard');
  });

  test('shows overview stat cards', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByText('Agents Online')).toBeVisible();
    await expect(page.getByText('DNS Queries')).toBeVisible();
    await expect(page.getByText('DNS Blocked')).toBeVisible();
    await expect(page.getByText('Network Flows')).toBeVisible();
  });

  test('refreshes data when refresh button is clicked', async ({ page }) => {
    await page.goto('/');
    const refreshButton = page.getByRole('button', { name: /refresh/i });
    if (await refreshButton.isVisible()) {
      await refreshButton.click();
      await expect(page.getByText('Agents Online')).toBeVisible();
    }
  });
});
