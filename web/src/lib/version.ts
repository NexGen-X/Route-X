// Satu sumber kebenaran versi tampil frontend (Fase 3 P3).
// Semua permukaan (Login, Header, Sidebar, fallback diagnostik) membaca
// dari sini — jangan menulis literal versi di komponen.
import pkg from '../../package.json';

export const APP_VERSION = `v${pkg.version}`;
