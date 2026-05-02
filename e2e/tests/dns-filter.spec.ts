import { test, expect } from '@playwright/test';

test.describe('DNS Filter', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/');
    await page.getByText('DNS Filter').click();
    await expect(page).toHaveURL(/dnsfilter/);
  });

  test('shows tabs', async ({ page }) => {
    await page.goto('/');
    await page.getByText('DNS Filter').click();
    const tabs = page.getByRole('tab');
    await expect(tabs.first()).toBeVisible();
  });

  test('can switch between tabs', async ({ page }) => {
    await page.goto('/');
    await page.getByText('DNS Filter').click();
    const tabs = page.getByRole('tab');
    const count = await tabs.count();
    if (count > 1) {
      await tabs.nth(1).click();
      await expect(tabs.nth(1)).toHaveAttribute('aria-selected', 'true');
    }
  });
});
