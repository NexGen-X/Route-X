import { defineConfig, devices } from '@playwright/test';

// Konfigurasi e2e Route-X: 3 alur inti (login, dashboard, inspector).
// Jalankan: BASE_URL=http://127.0.0.1:18080 E2E_EMAIL=... E2E_PASSWORD=... npx playwright test
// Server staging dan kredensial disiapkan di luar (lihat runbook e2e), bukan di sini,
// supaya test tidak membuat user sendiri dan tidak bergantung pada state database.
export default defineConfig({
  testDir: './e2e',
  timeout: 60 * 1000,
  retries: 1,
  // Sesi tersimpan hasil auth.setup: spec UI tidak login lagi per test,
  // supaya limiter login (20/IP/15 mnt) tidak menjegal suite.
  // File .auth-*.json diabaikan git (lihat .gitignore).
  use: {
    baseURL: process.env.BASE_URL || 'http://127.0.0.1:18080',
    trace: 'on-first-retry',
  },
  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
      dependencies: ['setup'],
    },
    {
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
      dependencies: ['setup'],
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
      dependencies: ['setup'],
    },
    {
      name: 'mobile-chrome',
      use: { ...devices['Pixel 7'] },
      dependencies: ['setup'],
    },
    {
      name: 'mobile-safari',
      use: { ...devices['iPhone 14'] },
      dependencies: ['setup'],
    },
  ],
});
