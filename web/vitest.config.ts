import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Konfigurasi terpisah dari vite.config.ts agar build produksi tidak membawa
// dependensi test. Environment jsdom dipakai testing-library di tes komponen
// (RoutingRules dan modul hasil pecahannya); tes pustaka murni tidak
// bergantung padanya.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    coverage: {
      include: ['src/lib/**', 'src/utils/**'],
    },
  },
});
