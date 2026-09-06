import { test, expect } from '@playwright/test';

test.describe('API Keys Management', () => {
  test.beforeEach(async ({ page }) => {
    // Navigate to API Keys page directly (assuming we bypass auth for local dev 
    // or we mock the auth state if implemented)
    await page.goto('/keys');
  });

  test('should display API keys list and allow opening create modal', async ({ page }) => {
    await expect(page.locator('h1')).toContainText('Kunci API');
    
    // Temukan tombol buat kunci API
    const createButton = page.getByRole('button', { name: /Buat Kunci Baru/i });
    await expect(createButton).toBeVisible();
    
    await createButton.click();
    
    // Modal seharusnya terbuka
    await expect(page.locator('form')).toBeVisible();
    await expect(page.locator('input[required]')).toBeVisible(); // Nama kunci
  });
});
