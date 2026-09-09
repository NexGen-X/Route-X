import { test, expect } from '@playwright/test';

// Alur 1: login menolak kredensial salah dan menerima kredensial benar.
// Prasyarat env: BASE_URL, E2E_EMAIL, E2E_PASSWORD (user staging khusus e2e).
test('login menolak password salah lalu menerima password benar', async ({ page }) => {
  const email = process.env.E2E_EMAIL || '';
  const password = process.env.E2E_PASSWORD || '';
  test.skip(!email || !password, 'E2E_EMAIL/E2E_PASSWORD belum diset');

  await page.goto('/');
  await page.getByLabel('Alamat Email').fill(email);
  await page.getByLabel('Kata Sandi').fill('salah-salah-salah');
  await page.getByRole('button', { name: 'Masuk ke Konsol' }).click();
  await expect(page.getByRole('alert')).toBeVisible({ timeout: 15000 });

  await page.getByLabel('Kata Sandi').fill(password);
  await page.getByRole('button', { name: 'Masuk ke Konsol' }).click();
  await expect(
    page.getByRole('heading', { name: 'Route-X Mission Control' }),
  ).toBeVisible({ timeout: 20000 });
});

// Alur 2: dashboard termuat dengan indikator in-flight.
test('dashboard menampilkan mission control dan in-flight', async ({ page }) => {
  const email = process.env.E2E_EMAIL || '';
  const password = process.env.E2E_PASSWORD || '';
  test.skip(!email || !password, 'E2E_EMAIL/E2E_PASSWORD belum diset');

  await page.goto('/');
  await page.getByLabel('Alamat Email').fill(email);
  await page.getByLabel('Kata Sandi').fill(password);
  await page.getByRole('button', { name: 'Masuk ke Konsol' }).click();
  await expect(
    page.getByRole('heading', { name: 'Route-X Mission Control' }),
  ).toBeVisible({ timeout: 20000 });
  await expect(page.getByText('in-flight')).toBeVisible();
});

// Alur 3: inspector request terbuka dan memuat daftar.
test('inspector request terbuka dan daftar termuat', async ({ page }) => {
  const email = process.env.E2E_EMAIL || '';
  const password = process.env.E2E_PASSWORD || '';
  test.skip(!email || !password, 'E2E_EMAIL/E2E_PASSWORD belum diset');

  await page.goto('/');
  await page.getByLabel('Alamat Email').fill(email);
  await page.getByLabel('Kata Sandi').fill(password);
  await page.getByRole('button', { name: 'Masuk ke Konsol' }).click();
  await expect(
    page.getByRole('heading', { name: 'Route-X Mission Control' }),
  ).toBeVisible({ timeout: 20000 });

  await page.goto('/#/requests');
  await expect(
    page.getByRole('heading', { name: 'Requests Inspector' }),
  ).toBeVisible({ timeout: 20000 });
});
