// Lebar bucket deret waktu (detik) diturunkan dari jarak timestamp dua
// titik pertama series itu sendiri.
//
// Backend yang menentukan granularitas bucket (1 jam untuk rentang <= 48
// jam, 1 hari di atasnya), jadi klien TIDAK boleh menebaknya dari label
// jendela: konstanta per-jendela lama (60 detik untuk jendela 1h) membuat
// throughput tampil sampai 60x lebih besar dari sebenarnya
// (audit3 V-FJ-12). Bila jarak tidak bisa diturunkan (kurang dari dua
// titik atau timestamp cacat), dipakai 3600 — bucket tersempit backend.
export function bucketSecondsFromSeries(
  points: readonly { timestamp: string }[],
): number {
  if (points.length >= 2) {
    const jarak =
      (Date.parse(points[1].timestamp) - Date.parse(points[0].timestamp)) /
      1000;
    if (Number.isFinite(jarak) && jarak > 0) return jarak;
  }
  return 3600;
}
