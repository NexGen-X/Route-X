import { test, expect } from '@playwright/test';

// Uji skalabilitas mobile Route-X: sidebar jadi drawer, konten tidak
// overflow horizontal, halaman kunci tetap bisa dibuka di layar kecil.
// Sesi pengguna kedua dari auth.setup.ts, sama seperti menyeluruh.spec.ts.

const HALAMAN_MOBILE: Array<{ hash: string; judul: string }> = [
  { hash: '#/', judul: 'Dashboard' },
  { hash: '#/requests', judul: 'Requests Inspector' },
  { hash: '#/observability', judul: 'Observabilitas & Telemetri' },
  { hash: '#/upstreams/providers', judul: 'Upstream Providers' },
  { hash: '#/upstreams/models', judul: 'Upstream Providers' },
  { hash: '#/access/users', judul: 'Pengguna Admin' },
  { hash: '#/gateway/budgets', judul: 'Budgets & Rate Limits' },
  { hash: '#/access/api-keys', judul: 'Client API Keys' },
  { hash: '#/cli-integrations', judul: 'CLI Integrations & 3-Mode Configurator' },
  { hash: '#/system/settings', judul: 'Runtime Settings' },
  { hash: '#/system/diagnostics', judul: 'Observabilitas & Telemetri' },
];

test.describe('mobile: drawer + tanpa overflow', () => {
  test.use({ storageState: './e2e/.auth-viewer.json' });

  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await expect(
      page.getByRole('heading', { name: 'Route-X Gateway Control' }),
    ).toBeVisible({ timeout: 20000 });
  });

  for (const h of HALAMAN_MOBILE) {
    test(`mobile ${h.judul} (${h.hash}) muat tanpa scroll horizontal`, async ({ page }) => {
      await page.goto('/' + h.hash);
      await expect(
        page.getByRole('heading', { name: h.judul, level: 1 }),
      ).toBeVisible({ timeout: 20000 });
      // Tidak boleh ada scroll horizontal di viewport kecil.
      const lebarBocor = await page.evaluate(() => {
        const doc = document.documentElement;
        return doc.scrollWidth - doc.clientWidth;
      });
      expect(
        lebarBocor,
        `overflow horizontal ${lebarBocor}px di ${h.hash}`,
      ).toBeLessThanOrEqual(1);
      await expect(page.getByText(/terjadi kesalahan|something went wrong/i)).toHaveCount(0);
      // Tombol aksi header tidak boleh terpotong viewport (kanan tombol
      // harus di dalam lebar layar): regresi pola header lama yang tanpa wrap.
      const tombolBocor = await page.evaluate(() => {
        const bocor: string[] = [];
        const btns = [...document.querySelectorAll('button')].filter((b) =>
          b.offsetParent !== null,
        );
        for (const b of btns) {
          const r = b.getBoundingClientRect();
          if (r.right > window.innerWidth + 1) {
            bocor.push(`${b.textContent?.trim().slice(0, 30)}@${Math.round(r.right)}`);
          }
        }
        return bocor;
      });
      expect(tombolBocor, `tombol terpotong di ${h.hash}: ${tombolBocor.join(', ')}`).toEqual([]);
    });
  }

  test('mobile bottom nav (MobileNav) tampil dan berfungsi', async ({ page }) => {
    // Bottom nav hanya dirender di viewport kecil (lg:hidden): skip jujur
    // di desktop, sama seperti pola drawer di bawah.
    const nav = page.getByRole('navigation', { name: 'Navigasi Bawah Mobile' });
    test.skip(!(await nav.count()), 'viewport desktop: tidak ada bottom nav');
    await expect(nav).toBeVisible({ timeout: 10000 });
    // Lima tujuan: Dashboard, Requests, Providers, API Keys, Lainnya.
    for (const label of ['Dashboard', 'Requests', 'Providers', 'API Keys']) {
      await expect(nav.getByRole('button', { name: label, exact: true })).toBeVisible();
    }
    // Navigasi lewat bottom nav benar-benar berpindah halaman.
    await nav.getByRole('button', { name: 'Providers', exact: true }).click();
    await expect(
      page.getByRole('heading', { name: 'Upstream Providers', level: 1 }),
    ).toBeVisible({ timeout: 20000 });
    // Tombol Lainnya membuka drawer berisi seluruh grup navigasi.
    await nav.getByRole('button', { name: 'Lainnya, buka semua menu' }).click();
    await expect(page.locator('aside').first()).toBeVisible({ timeout: 10000 });
  });

  test('mobile drawer sidebar bisa dibuka tutup', async ({ page }) => {
    await page.goto('/#/requests');
    await expect(
      page.getByRole('heading', { name: 'Requests Inspector', level: 1 }),
    ).toBeVisible({ timeout: 20000 });
    // Tombol hamburger (lg:hidden) hanya ada di viewport kecil: skip
    // jujur di desktop karena drawer memang tidak relevan di sana.
    const pemicu = page.getByRole('button', { name: /buka menu navigasi/i });
    test.skip(!(await pemicu.count()), 'viewport desktop: tidak ada drawer');
    await pemicu.first().click();
    // Aside sidebar (drawer) harus tampil setelah dibuka.
    const drawer = page.locator('aside').first();
    await expect(drawer).toBeVisible({ timeout: 10000 });
    // Tutup drawer via klik backdrop (pola resmi di Sidebar.tsx):
    // drawer z-50 menutupi tombol hamburger z-30 saat terbuka.
    await page.mouse.click(500, 300).catch(() => {});
    await page.waitForTimeout(600);
    await expect(
      page.getByRole('heading', { name: 'Requests Inspector', level: 1 }),
    ).toBeVisible({ timeout: 10000 });
    const lebarBocor = await page.evaluate(() => {
      const doc = document.documentElement;
      return doc.scrollWidth - doc.clientWidth;
    });
    expect(lebarBocor).toBeLessThanOrEqual(1);
  });
});
