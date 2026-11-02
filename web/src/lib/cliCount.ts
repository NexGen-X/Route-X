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
      // Badge harus menampilkan tool yang TERDETEKSI di mesin ini
      // (total_detected), bukan panjang seluruh registri (tools) — versi
      // lama selalu menampilkan jumlah registri penuh walau tidak ada satu
      // pun tool yang terpasang (audit3 V-FJ-11).
      .then((r) => (typeof r.total_detected === 'number' ? r.total_detected : null))
      .catch(() => null);
  }
  return cache;
}
