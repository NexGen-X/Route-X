import type { CLITool } from '../../types';

// Status konfigurasi tool CLI (Fase 2 R4), diturunkan dari data pindaian
// backend apa adanya. Backend menandai konfigurasi yang dikelola Route-X
// lewat `updated_at` pada setelan cli:config:<id> — hanya terisi bila
// konfigurasi tool itu pernah disimpan lewat Route-X.
//
// Backend TIDAK membaca target aktual dari file konfigurasi asli tool,
// jadi dasbor tidak mengklaim keadaan "terarah ke endpoint lain" dari
// pindaian biasa: keadaan itu hanya terdeteksi lewat verifikasi sesudah
// apply (verifikasiHasilApply), saat hasil pindai ulang tidak cocok
// dengan konfigurasi yang baru saja diterapkan.
export type StatusKonfigurasiTool =
  | 'tidak-terpasang'
  | 'belum-dikonfigurasi'
  | 'terkonfigurasi-routex';

export function statusKonfigurasiTool(tool: CLITool): StatusKonfigurasiTool {
  if (!tool.installed) return 'tidak-terpasang';
  if (!tool.updated_at) return 'belum-dikonfigurasi';
  return 'terkonfigurasi-routex';
}

export interface HasilVerifikasiApply {
  ok: boolean;
  // Penjelasan kegagalan dalam Bahasa operator, siap ditampilkan.
  pesan?: string;
}

// Verifikasi sesudah apply: tool hasil pindai ulang harus tercatat
// terkonfigurasi dan mode/target aktifnya persis yang diterapkan.
// Bila tidak, keberhasilan tidak boleh dilaporkan sebagai sukses penuh
// (pelajaran audit F5: jangan sukses palsu).
export function verifikasiHasilApply(
  hasilScan: CLITool | undefined,
  modeDiminta: string,
  targetDiminta: string,
): HasilVerifikasiApply {
  if (!hasilScan) {
    return { ok: false, pesan: 'tool tidak ditemukan pada pindaian ulang' };
  }
  if (!hasilScan.updated_at) {
    return {
      ok: false,
      pesan: 'konfigurasi tidak tercatat di Route-X sesudah diterapkan',
    };
  }
  if (hasilScan.active_mode !== modeDiminta || hasilScan.active_target !== targetDiminta) {
    return {
      ok: false,
      pesan: `hasil pindai ulang berbeda dari yang diterapkan — mode aktif "${hasilScan.active_mode}", target aktif "${hasilScan.active_target}"`,
    };
  }
  return { ok: true };
}
