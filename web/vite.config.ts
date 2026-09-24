import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: false,
    chunkSizeWarningLimit: 600,
    modulePreload: {
      resolveDependencies(filename, deps, { hostType }) {
        if (hostType === 'html') {
          // Jangan preload vendor-charts (Recharts ~517KB) pada HTML awal / Login;
          // pustaka grafik hanya dimuat ketika membuka Dashboard atau Observabilitas.
          return deps.filter((dep) => !dep.includes('vendor-charts'));
        }
        return deps;
      },
    },
    rollupOptions: {
      output: {
        manualChunks: {
          'vendor-charts': ['recharts'],
          'vendor-icons': ['lucide-react'],
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
