import { test, expect } from '@playwright/test';

// Uji alur TULIS kritis Route-X lewat UI asli + verifikasi API.
// Dua test UI memakai sesi admin tersimpan (auth.setup.ts, hemat limiter).
// Dua test API-only login lewat fixture `request` miliknya sendiri
// (masing-masing 1 hit limiter).
// Tiap kasus membersihkan datanya agar staging kembali steril.
//
// Pelajaran yang ditanam di sini:
// - page.request membawa cookie sesi TAPI tidak mengirim Origin maupun
//   token CSRF otomatis, padahal middleware CSRF menuntut keduanya.
//   Tanpa headerMutasi, cleanup API gagal 403 diam-diam dan data sisa
//   (mis. satu budget global unik) menggagalkan run berikutnya.
// - strategy routing rule valid: priority/round_robin/weighted/
//   lowest_latency/lowest_cost/capability (CHECK DB 0008). `model_only`
//   adalah mode form UI, bukan strategy DB.
//
// Prasyarat env: BASE_URL, E2E_ADMIN_EMAIL, E2E_ADMIN_PASSWORD,
// serta E2E_EMAIL + E2E_PASSWORD (Viewer, untuk test penolakan).

test.describe('tulis admin via UI', () => {
  test.use({ storageState: './e2e/.auth-admin.json' });
  // Header mutasi untuk page.request: Origin + token CSRF sesi browser.
  async function headerMutasi(page: any): Promise<Record<string, string>> {
    const me = await page.request.get('/api/auth/me');
    const tokenCSRF = ((await me.json()).csrf_token ?? '') as string;
    return {
      Origin: process.env.BASE_URL || 'http://127.0.0.1:18080',
      'X-CSRF-Token': tokenCSRF,
    };
  }

  test('buat API key lewat UI lalu muncul di daftar', async ({ page }) => {
    await page.goto('/');
    await expect(
      page.getByRole('heading', { name: 'Route-X Mission Control' }),
    ).toBeVisible({ timeout: 20000 });
    const nama = 'e2e-tulis-' + Date.now();
    await page.goto('/#/access/api-keys');
    await expect(
      page.getByRole('heading', { name: 'Client API Keys', level: 1 }),
    ).toBeVisible({ timeout: 20000 });
    await page.getByRole('button', { name: 'Buat API Key' }).first().click();
    await page.locator('#api-key-name').fill(nama);
    await page.getByRole('button', { name: 'Hasilkan Kunci API' }).click();
    // Modal sukses menampilkan kunci mentah sekali: bukti tulis berhasil.
    await expect(page.getByText('Simpan kunci ini sekarang')).toBeVisible({
      timeout: 20000,
    });
    // Tutup modal lalu pastikan nama muncul di daftar.
    await page.keyboard.press('Escape');
    await expect(page.getByText(nama).first()).toBeVisible({ timeout: 20000 });

    // Bersihkan lewat sesi browser yang sama: revoke semua key e2e-tulis-*.
    const H = await headerMutasi(page);
    const daftar = await page.request.get('/api/admin/access/api-keys');
    expect(daftar.ok()).toBeTruthy();
    const badan = await daftar.json();
    const items = badan.items ?? badan.data ?? badan;
    for (const k of items as any[]) {
      if (k.name?.startsWith('e2e-tulis-')) {
        await page.request.post(`/api/admin/access/api-keys/${k.id}/revoke`, {
          headers: H,
        });
      }
    }
  });

  test('alokasikan budget lewat UI lalu muncul di daftar', async ({ page }) => {
    await page.goto('/');
    await expect(
      page.getByRole('heading', { name: 'Route-X Mission Control' }),
    ).toBeVisible({ timeout: 20000 });
    // Backend hanya mengizinkan satu budget per scope global: bersihkan
    // sisa e2e lebih dulu lewat sesi browser yang sama.
    const H = await headerMutasi(page);
    const pra = await page.request.get('/api/admin/gateway/budgets');
    if (pra.ok()) {
      const badanPra = await pra.json();
      const itemsPra = badanPra.items ?? badanPra.data ?? badanPra;
      for (const b of itemsPra as any[]) {
        if (b.name?.startsWith('e2e-budget-')) {
          await page.request.delete(`/api/admin/gateway/budgets/${b.id}`, {
            headers: H,
          });
        }
      }
    }
    const nama = 'e2e-budget-' + Date.now();
    await page.goto('/#/gateway/budgets');
    await expect(
      page.getByRole('heading', { name: 'Budgets & Cost Limits', level: 1 }),
    ).toBeVisible({ timeout: 20000 });
    await page.getByRole('button', { name: 'Alokasikan Anggaran' }).first().click();
    await page.getByPlaceholder('Budget Bulanan Tim Internal').fill(nama);
    // Batas maksimal USD: input number setelah label Batas Maksimal.
    await page.locator('input[type="number"]').first().fill('25');
    await page.getByRole('button', { name: 'Simpan Anggaran' }).click();
    await expect(page.getByText(nama).first()).toBeVisible({ timeout: 20000 });

    // Bersihkan lewat sesi browser yang sama.
    const daftar = await page.request.get('/api/admin/gateway/budgets');
    expect(daftar.ok()).toBeTruthy();
    const badan = await daftar.json();
    const items = badan.items ?? badan.data ?? badan;
    for (const b of items as any[]) {
      if (b.name?.startsWith('e2e-budget-')) {
        await page.request.delete(`/api/admin/gateway/budgets/${b.id}`, {
          headers: H,
        });
      }
    }
  });
});

test('routing rule + rate limit via API: buat, baca, hapus', async ({
  request,
}) => {
  const email = process.env.E2E_ADMIN_EMAIL || '';
  const password = process.env.E2E_ADMIN_PASSWORD || '';
  test.skip(!email || !password, 'E2E_ADMIN_EMAIL/E2E_ADMIN_PASSWORD belum diset');
  // Fixture request tidak mengirim Origin browser: CSRF double-submit
  // menuntut Origin + token sesi, jadi ambil dari /me lalu sertakan.
  const asal = process.env.BASE_URL || 'http://127.0.0.1:18080';
  const login = await request.post('/api/auth/login', {
    data: { email, password },
  });
  expect(login.ok()).toBeTruthy();
  const me = await request.get('/api/auth/me');
  const tokenCSRF = (await me.json()).csrf_token as string;
  const H = { Origin: asal, 'X-CSRF-Token': tokenCSRF };

  const namaRule = 'e2e-rule-' + Date.now();
  const buatRule = await request.post('/api/admin/gateway/routing-rules', {
    headers: H,
    data: { name: namaRule, strategy: 'priority', priority: 1 },
  });
  expect(buatRule.ok()).toBeTruthy();
  const rule = await buatRule.json();
  const ruleId = rule.id ?? rule.data?.id;
  expect(ruleId).toBeTruthy();

  const buatRL = await request.post('/api/admin/gateway/rate-limits', {
    headers: H,
    data: { scope: 'global', requests_per_minute: 5000 },
  });
  expect(buatRL.ok()).toBeTruthy();
  const rl = await buatRL.json();
  const rlId = rl.id ?? rl.data?.id;
  expect(rlId).toBeTruthy();

  const daftarRule = await request.get('/api/admin/gateway/routing-rules');
  expect(await daftarRule.text()).toContain(namaRule);

  await request.delete(`/api/admin/gateway/routing-rules/${ruleId}`, {
    headers: H,
  });
  await request.delete(`/api/admin/gateway/rate-limits/${rlId}`, {
    headers: H,
  });
  const sesudah = await request.get('/api/admin/gateway/routing-rules');
  expect(await sesudah.text()).not.toContain(namaRule);
});

test('Viewer ditolak tulis: bukti guard permission bekerja', async ({
  request,
}) => {
  const email = process.env.E2E_EMAIL || '';
  const password = process.env.E2E_PASSWORD || '';
  test.skip(!email || !password, 'E2E_EMAIL/E2E_PASSWORD belum diset');
  const login = await request.post('/api/auth/login', {
    data: { email, password },
  });
  expect(login.ok()).toBeTruthy();
  const meV = await request.get('/api/auth/me');
  const tokenV = (await meV.json()).csrf_token as string;
  const HV = {
    Origin: process.env.BASE_URL || 'http://127.0.0.1:18080',
    'X-CSRF-Token': tokenV,
  };
  const coba = await request.post('/api/admin/gateway/routing-rules', {
    headers: HV,
    data: { name: 'e2e-coba-ditolak', strategy: 'priority' },
  });
  expect(coba.status()).toBe(403);
});
