import { test, expect } from '@playwright/test';

// Repro bug produksi: dashboard crash React #31 saat grafik Latensi ada data.
// Akar: AreaChart dataKey='latency' membaca objek DistributionDTO.
test.use({ storageState: 'e2e/.auth-viewer.json' });

test('dashboard: pil Latensi tampil tanpa crash saat ada sampel', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto('/#/');
  await expect(page.getByText('Volume & Dinamika Lalu Lintas')).toBeVisible({ timeout: 20000 });
  // Klik pil Latensi (ada sampel: 15 chat seed) — dulu crash error #31.
  await page.getByRole('button', { name: 'Latensi', exact: true }).click();
  await page.waitForTimeout(3000);
  // Arahkan kursor ke tengah grafik agar Tooltip recharts tampil:
  // crash #31 terjadi saat Tooltip merender nilai objek DistributionDTO.
  const chart = page.locator('.recharts-wrapper').first();
  await expect(chart).toBeVisible({ timeout: 10000 });
  const box = await chart.boundingBox();
  if (box) {
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 5 });
    await page.waitForTimeout(1500);
  }
  // Kembali ke Req + Token juga aman.
  await page.getByRole('button', { name: 'Req', exact: true }).click();
  await page.waitForTimeout(1500);
  await page.getByRole('button', { name: 'Token', exact: true }).click();
  await page.waitForTimeout(1500);
  const crash = errors.filter((m) => m.includes('Minified React error') || m.includes('Objects are not valid'));
  expect(crash).toEqual([]);
  await expect(page.getByText('Volume & Dinamika Lalu Lintas')).toBeVisible();
});
