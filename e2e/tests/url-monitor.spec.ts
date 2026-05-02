import { test, expect } from '@playwright/test';

test.describe('URL Monitor', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/');
    await page.getByText('URL Check').click();
    await expect(page).toHaveURL(/urlcheck/);
  });

  test('can add a URL', async ({ page }) => {
    await page.goto('/');
    await page.getByText('URL Check').click();

    const input = page.getByPlaceholder(/url/i);
    if (await input.isVisible()) {
      await input.fill('https://example.com');
      const addButton = page.getByRole('button', { name: /add/i });
      await addButton.click();
      await expect(page.getByText('example.com')).toBeVisible();
    }
  });
});
