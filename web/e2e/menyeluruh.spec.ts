import { test, expect } from '@playwright/test';

// Uji menyeluruh Route-X: kunjungi SELURUH halaman lewat hash, pastikan
// judul Header + konten utama tampil dan tidak ada error boundary.
// Sesi pengguna kedua tersimpan dari auth.setup.ts (hemat limiter login).
// Prasyarat env: BASE_URL, E2E_EMAIL, E2E_PASSWORD. Arsitektur Route-X kini
// single-admin (RBAC dihapus di migrasi 0010): setiap pengguna
// terautentikasi adalah admin penuh. Test ini hanya memastikan halaman
// TERBUKA dan memuat; alur tulis diuji terpisah di aksi-tulis.spec.ts.

const HALAMAN: Array<{ hash: string; judul: string; penanda: string }> = [
  { hash: '#/', judul: 'Dashboard', penanda: 'Route-X Gateway Control' },
  { hash: '#/requests', judul: 'Requests Inspector', penanda: 'Requests Inspector' },
  { hash: '#/observability', judul: 'Observabilitas & Telemetri', penanda: 'Observabilitas & Telemetri' },
  { hash: '#/upstreams/providers', judul: 'Upstream Providers', penanda: 'Upstream Providers' },
  { hash: '#/upstreams/models', judul: 'Upstream Providers', penanda: 'Upstream Providers' },
  { hash: '#/access/users', judul: 'Pengguna Admin', penanda: 'Tambah Pengguna' },
  { hash: '#/upstreams/egress', judul: 'Egress Proxy Pools', penanda: 'Egress Proxy Pools' },
  { hash: '#/gateway/routing', judul: 'Routing Rules', penanda: 'Routing Rules Engine' },
  { hash: '#/cli-integrations', judul: 'CLI Integrations & 3-Mode Configurator', penanda: 'Integrasi CLI & Editor AI' },
  { hash: '#/gateway/rate-limits', judul: 'Rate Limits', penanda: 'Rate Limits' },
  { hash: '#/gateway/budgets', judul: 'Budgets & Rate Limits', penanda: 'Budgets & Cost Control' },
  { hash: '#/gateway/breakers', judul: 'Routing & Failover', penanda: 'Routing & Failover' },
  { hash: '#/access/api-keys', judul: 'Client API Keys', penanda: 'Client API Keys' },
  { hash: '#/system/settings', judul: 'Runtime Settings', penanda: 'Pengaturan Sistem' },
  { hash: '#/system/diagnostics', judul: 'Observabilitas & Telemetri', penanda: 'Kesehatan Sistem' },
  { hash: '#/webhooks', judul: 'Webhooks', penanda: 'Webhooks' },
];

test.describe('semua halaman viewer', () => {
  test.use({ storageState: './e2e/.auth-viewer.json' });
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await expect(
      page.getByRole('heading', { name: 'Route-X Gateway Control' }),
    ).toBeVisible({ timeout: 20000 });
  });

  for (const h of HALAMAN) {
    test(`halaman ${h.judul} (${h.hash}) terbuka dan memuat`, async ({ page }) => {
      // beforeEach memastikan sesi di dashboard lalu pindah hash.
      await page.goto('/' + h.hash);
      // Judul Header (h1) + penanda konten utama keduanya harus tampil.
      await expect(
        page.getByRole('heading', { name: h.judul, level: 1 }),
      ).toBeVisible({ timeout: 20000 });
      await expect(page.getByText(h.penanda).first()).toBeVisible({
        timeout: 20000,
      });
      // Tidak ada error boundary maupun halaman kosong.
      await expect(page.getByText(/terjadi kesalahan|something went wrong/i)).toHaveCount(0);
    });
  }

  test('rute tak dikenal jatuh ke dashboard, bukan blank', async ({ page }) => {
    await page.goto('/#/rute-tidak-ada-xyz');
    await expect(
      page.getByRole('heading', { name: 'Route-X Gateway Control' }),
    ).toBeVisible({ timeout: 20000 });
  });
});
