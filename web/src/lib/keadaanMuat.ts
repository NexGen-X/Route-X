// Gerbang keadaan muat BERSAMA untuk klaim kosong/gagal di
// dashboard (Fase 18, menutup pola Putaran Bersih #5: tiga belas
// permukaan mengklaim "kosong/tidak ada/0/nonaktif" secara
// definitif selagi muatan pertamanya GAGAL — data tidak pernah
// terukur, tetapi UI menyatakannya terukur kosong).
//
// Satu-satunya pertanyaan yang dijawab modul ini: "apa yang
// sebenarnya diketahui permukaan ini tentang datanya?" Empat
// keadaan, dengan urutan keputusan yang disengaja:
//
//   - 'ada'    : ada data (jumlah > 0). Menang atas segalanya —
//                data lama yang masih tampil saat refresh gagal
//                adalah data nyata, bukan klaim kosong.
//   - 'memuat' : belum ada data dan muatan sedang berjalan.
//   - 'gagal'  : belum ada data dan muatan terakhir gagal. Klaim
//                kosong DILARANG pada keadaan ini; permukaan
//                menampilkan teks netral "belum terukur" dan/atau
//                QueryError, bukan "tidak ada X".
//   - 'kosong' : muatan berhasil dan hasilnya memang kosong —
//                satu-satunya keadaan yang boleh mengklaim kosong.
//
// Pemanggil yang memuat dari BEBERAPA sumber meneruskan keadaan
// sumber yang relevan untuk tiap klaim (bukan satu bendera global
// halaman), persis seperti banner galat menamai sumbernya.
export type KeadaanMuat = 'memuat' | 'gagal' | 'kosong' | 'ada';

export interface MasukanKeadaanMuat {
  /** Muatan (pertama atau ulang) sedang berjalan. */
  memuat: boolean;
  /** Muatan terakhir untuk sumber ini gagal. */
  gagal: boolean;
  /** Jumlah item data yang dimiliki saat ini (0 bila belum ada). */
  jumlah: number;
}

export function keadaanMuat({ memuat, gagal, jumlah }: MasukanKeadaanMuat): KeadaanMuat {
  if (jumlah > 0) return 'ada';
  if (memuat) return 'memuat';
  if (gagal) return 'gagal';
  return 'kosong';
}
