// Jumlah tool CLI terdeteksi untuk badge navigasi (Fase 3 P3).
// Angkanya diambil dari backend (api.cli.detected) — bukan konstanta
// karangan UI. Hasil di-cache seumur sesi halaman karena registri tool
// hanya berubah saat deploy; kegagalan memuat menghasilkan null dan
// badge disembunyikan, bukan diganti angka karangan.
import { api } from '../api/client';

let cache: Promise<number | null> | null = null;

export function muatJumlahToolCli(): Promise<number | null> {
  if (!cache) {
    cache = api.cli
      .detected()
      .then((r) => (Array.isArray(r.tools) ? r.tools.length : null))
      .catch(() => null);
  }
  return cache;
}
