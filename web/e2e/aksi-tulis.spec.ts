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
// serta E2E_EMAIL + E2E_PASSWORD (pengguna kedua, untuk test single-admin).

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
      page.getByRole('heading', { name: 'Route-X Gateway Control' }),
    ).toBeVisible({ timeout: 20000 });
    const nama = 'e2e-tulis-' + Date.now();
    await page.goto('/#/access/api-keys');
    await expect(
      page.getByRole('heading', { name: 'Kunci API Klien', level: 1 }),
    ).toBeVisible({ timeout: 20000 });
    await page.getByRole('button', { name: 'Buat API Key' }).first().click();
    await page.locator('#api-key-name-drawer').fill(nama);
    await page.getByRole('button', { name: 'Hasilkan Kunci API' }).click();
    // Modal sukses menampilkan kunci mentah sekali: bukti tulis berhasil.
    await expect(page.getByText('Kunci API Berhasil Dibuat')).toBeVisible({
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
      page.getByRole('heading', { name: 'Route-X Gateway Control' }),
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
      page.getByRole('heading', { name: 'Anggaran & Batas Laju', level: 1 }),
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

  test('buat user baru lewat UI, verifikasi API, lalu hapus bersih', async ({
    page,
  }) => {
    await page.goto('/');
    await expect(
      page.getByRole('heading', { name: 'Route-X Gateway Control' }),
    ).toBeVisible({ timeout: 20000 });
    const H = await headerMutasi(page);
    const cap = Date.now();
    const emailUser = `e2e-user-ui-${cap}@local`;
    let userId = '';
    try {
      await page.goto('/#/access/users');
      await expect(
        page.getByRole('heading', { name: 'Pengguna Admin', level: 1 }),
      ).toBeVisible({ timeout: 20000 });
      await page.getByRole('button', { name: 'Tambah Pengguna' }).first().click();
      await page.locator('input[type="email"]').fill(emailUser);
      await page.locator('input[type="text"]').first().fill('E2E User UI');
      await page.locator('input[type="password"]').fill('KuatUi2026!Xx9#');
      await page.getByRole('button', { name: 'Simpan Pengguna' }).click();
      // Halaman Pengguna merender dua salinan (tabel >=sm dan kartu <sm,
      // Fase 3 P5): yang harus terlihat adalah salinan yang tampil di
      // viewport proyek ini, bukan sekadar salinan pertama di DOM.
      await expect(
        page.locator('p:visible', { hasText: emailUser }),
      ).toBeVisible({ timeout: 20000 });
      const daftarUser = await page.request.get('/api/admin/access/users');
      const badanUser = await daftarUser.json();
      userId = ((badanUser.items ?? []) as any[]).find(
        (u) => u.email === emailUser,
      )?.id;
      expect(userId).toBeTruthy();
      // Arsitektur single-admin: user baru langsung terbaca sebagai admin
      // penuh (tidak ada lagi peran/izin terpisah sejak migrasi 0010).
      const baca = await page.request.get(
        `/api/admin/access/users/${userId}`,
      );
      expect(baca.ok()).toBeTruthy();
    } finally {
      if (userId) {
        await page.request.delete(`/api/admin/access/users/${userId}`, {
          headers: H,
        });
      }
    }
    const sesudahUser = await page.request.get('/api/admin/access/users');
    expect(await sesudahUser.text()).not.toContain(emailUser);
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

test('pengguna kedua adalah admin penuh (arsitektur single-admin)', async ({
  request,
}) => {
  // RBAC lama dihapus (migrasi 0010): tidak ada lagi peran Viewer yang
  // ditolak tulis. Test ini MENEGASKAN perilaku aktual: pengguna kedua
  // yang terautentikasi BOLEH menulis, sama seperti admin pertama, lalu
  // membersihkan datanya sendiri.
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
  const namaRule = 'e2e-single-admin-' + Date.now();
  const buat = await request.post('/api/admin/gateway/routing-rules', {
    headers: HV,
    data: { name: namaRule, strategy: 'priority', priority: 1 },
  });
  expect(buat.ok()).toBeTruthy();
  const ruleId = (await buat.json()).id;
  expect(ruleId).toBeTruthy();
  await request.delete(`/api/admin/gateway/routing-rules/${ruleId}`, {
    headers: HV,
  });
  const sesudah = await request.get('/api/admin/gateway/routing-rules');
  expect(await sesudah.text()).not.toContain(namaRule);
});
