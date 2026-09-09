import { test as setup, expect } from '@playwright/test';

// Setup auth e2e: login tepat 1 kali per peran lalu simpan storage state.
// Semua spec UI memakai ulang storage ini TANPA login lagi, supaya tidak
// menghabiskan limiter login (20/IP per 15 menit: 22 test x login = 429).
// Prasyarat env: BASE_URL, E2E_ADMIN_EMAIL/PASSWORD, E2E_EMAIL/PASSWORD.

const ADMIN_JSON = './e2e/.auth-admin.json';
const VIEWER_JSON = './e2e/.auth-viewer.json';

setup('login admin sekali', async ({ page }) => {
  const email = process.env.E2E_ADMIN_EMAIL || '';
  const password = process.env.E2E_ADMIN_PASSWORD || '';
  setup.skip(!email || !password, 'E2E_ADMIN_EMAIL/PASSWORD belum diset');
  await page.goto('/');
  await page.getByLabel('Alamat Email').fill(email);
  await page.getByLabel('Kata Sandi').fill(password);
  await page.getByRole('button', { name: 'Masuk ke Konsol' }).click();
  await expect(
    page.getByRole('heading', { name: 'Route-X Mission Control' }),
  ).toBeVisible({ timeout: 20000 });
  await page.context().storageState({ path: ADMIN_JSON });
});

setup('login viewer sekali', async ({ page }) => {
  const email = process.env.E2E_EMAIL || '';
  const password = process.env.E2E_PASSWORD || '';
  setup.skip(!email || !password, 'E2E_EMAIL/PASSWORD belum diset');
  await page.goto('/');
  await page.getByLabel('Alamat Email').fill(email);
  await page.getByLabel('Kata Sandi').fill(password);
  await page.getByRole('button', { name: 'Masuk ke Konsol' }).click();
  await expect(
    page.getByRole('heading', { name: 'Route-X Mission Control' }),
  ).toBeVisible({ timeout: 20000 });
  await page.context().storageState({ path: VIEWER_JSON });
});
