import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    // true (audit F9): dist dibersihkan tiap build. false membuat artefak
    // generasi build lama menumpuk dan ikut ter-embed ke biner Go.
    emptyOutDir: true,
    chunkSizeWarningLimit: 600,
    modulePreload: {
      resolveDependencies(filename, deps, { hostType }) {
        if (hostType === 'html') {
          // Jangan preload vendor-charts (Recharts ~378KB, terverifikasi
          // dari build Recharts 3 di tahap 4/B-094) pada HTML awal / Login;
          // pustaka grafik hanya dimuat ketika membuka Dashboard atau Observabilitas.
          return deps.filter((dep) => !dep.includes('vendor-charts'));
        }
        return deps;
      },
    },
    // Vite 8 (Rolldown): bentuk objek `rollupOptions.output.manualChunks`
    // sudah tidak didukung dan membuat build gagal ("manualChunks is not a
    // function"); panduan migrasi resmi Vite 8 mengarahkan ke
    // `rolldownOptions` + `output.codeSplitting`. Nama chunk dipertahankan
    // persis (`vendor-charts`/`vendor-icons`) supaya penyaringan preload di
    // atas dan perilaku pemuatan malas grafik/ikon tidak berubah.
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            { name: 'vendor-charts', test: /\/node_modules\/recharts\// },
            { name: 'vendor-icons', test: /\/node_modules\/lucide-react\// },
          ],
        },
      },
    },
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
      '/v1': {
        target: 'http://127.0.0.1:8080',
        changeOrigin: true,
      },
    },
  },
});
