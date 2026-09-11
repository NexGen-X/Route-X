import { test, expect, request as pwRequest } from '@playwright/test';

// Repro bug produksi: dashboard crash React #31 saat grafik Latensi ada data.
// Akar: AreaChart dataKey='latency' membaca objek DistributionDTO.
//
// Deterministik: traffic sampel dibuat sendiri via API di beforeAll
// (pola sisa-kritis: 1 login admin, key RPM besar, prompt unik +
// cache-buster, revoke di finally), jadi tak peduli urutan eksekusi.

const ASAL = process.env.BASE_URL || 'http://127.0.0.1:18080';

let ctx: any;
let H: Record<string, string>;

test.beforeAll(async () => {
  // Nol login tambahan: pakai sesi admin dari auth.setup (proyek setup
  // adalah dependency, jadi file sudah ada). Login API langsung di sini
  // menjebol limiter 20/IP/15 mnt pada proyek ketiga + retry.
  ctx = await pwRequest.newContext({ baseURL: ASAL, storageState: 'e2e/.auth-admin.json' });
  const me = await ctx.get('/api/auth/me');
  expect(me.ok()).toBeTruthy();
  H = { Origin: ASAL, 'X-CSRF-Token': (await me.json()).csrf_token as string };

  // 3 sampel chat unik agar series Latensi berisi data.
  const buatKey = await ctx.post('/api/admin/access/api-keys', {
    headers: H,
    data: { name: 'e2e-latensi-key-' + Date.now(), rate_limit_rpm: 6000 },
  });
  expect(buatKey.ok()).toBeTruthy();
  const badan = await buatKey.json();
  const keyId = badan.key?.id ?? badan.id;
  const mentah = badan.raw_key ?? badan.key?.raw_key ?? '';
  expect(mentah).toBeTruthy();
  try {
    for (let i = 0; i < 3; i++) {
      const chat = await ctx.post('/v1/chat/completions', {
        headers: { Authorization: `Bearer ${mentah}` },
        data: {
          model: 'gpt-5',
          messages: [{ role: 'user', content: `latensi-probe-${Date.now()}-${i}` }],
        },
      });
      expect(chat.ok()).toBeTruthy();
    }
  } finally {
    if (keyId) {
      await ctx.post(`/api/admin/access/api-keys/${keyId}/revoke`, {
        headers: H,
      });
    }
  }
});

test.afterAll(async () => {
  await ctx?.dispose();
});

test.use({ storageState: 'e2e/.auth-viewer.json' });

test('dashboard: pil Latensi tampil tanpa crash saat ada sampel', async ({ page }) => {
  const errors: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto('/#/');
  await expect(page.getByText('Volume & Dinamika Lalu Lintas')).toBeVisible({ timeout: 20000 });
  // Klik pil Latensi — dulu crash error #31.
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
