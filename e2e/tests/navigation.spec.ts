import { test, expect } from '@playwright/test';

test.describe('Navigation', () => {
  test('sidebar shows all modules', async ({ page }) => {
    await page.goto('/');
    await expect(page.getByText('DNS Filter')).toBeVisible();
    await expect(page.getByText('Network Monitor')).toBeVisible();
    await expect(page.getByText('System Monitor')).toBeVisible();
    await expect(page.getByText('URL Check')).toBeVisible();
  });

  test('clicking a module navigates to it', async ({ page }) => {
    await page.goto('/');
    await page.getByText('DNS Filter').click();
    await expect(page).toHaveURL(/dnsfilter/);
  });

  test('clicking Dashboard returns to home', async ({ page }) => {
    await page.goto('/');
    await page.getByText('DNS Filter').click();
    await page.getByText('Dashboard').first().click();
    await expect(page.locator('h1')).toContainText('Dashboard');
  });

  test('clicking Biglybigly title returns to dashboard', async ({ page }) => {
    await page.goto('/');
    await page.getByText('DNS Filter').click();
    await page.getByText('Biglybigly').click();
    await expect(page.locator('h1')).toContainText('Dashboard');
  });
});
