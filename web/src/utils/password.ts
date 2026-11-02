// Validasi kata sandi di klien yang MENCERMINKAN kebijakan server persis
// (internal/security/password.go, audit3 V-FF-12/V-FF-13). Dulu setiap form
// punya aturan sendiri yang lebih longgar — form pengguna memeriksa min 8
// karakter yang SUDAH di-trim lalu mengirim teks mentah, form ganti sandi
// hanya memeriksa panjang — sehingga sandi yang lolos UI ditolak server
// dengan pesan buram, atau sebaliknya lolos berisi spasi tepi yang tak
// pernah bisa diketik ulang persis saat masuk.
//
// Aturan server (ValidatePasswordPolicy):
// - panjang >= 12 RUNE (bukan byte);
// - panjang <= 1024 BYTE UTF-8;
// - minimal 2 dari 3 kelas: huruf [a-zA-Z], digit [0-9], karakter lain;
// - tidak termasuk daftar kata sandi umum (perbandingan case-insensitive).

const KATA_SANDI_UMUM = [
  'password1234',
  '123456789012',
  'qwertyuiop12',
  'admin123456',
  'routex12345',
  'password12345',
  '1234567890ab',
  'iloveyou1234',
  'adminpassword',
  'changeme1234',
  'letmein12345',
  'welcome12345',
];

/**
 * Mengembalikan pesan galat Bahasa Indonesia bila kata sandi melanggar
 * kebijakan server, atau null bila sah. Nilai dinilai MENTAH apa adanya —
 * persis seperti yang akan dikirim dan diperiksa server.
 */
export function validasiPassword(kataSandi: string): string | null {
  if ([...kataSandi].length < 12) {
    return 'Kata sandi minimal 12 karakter.';
  }
  if (new TextEncoder().encode(kataSandi).length > 1024) {
    return 'Kata sandi terlalu panjang (maksimal 1024 byte).';
  }
  if (KATA_SANDI_UMUM.includes(kataSandi.toLowerCase())) {
    return 'Kata sandi ini termasuk daftar kata sandi umum yang mudah ditebak — pilih kata sandi lain.';
  }
  let kelas = 0;
  if (/[a-zA-Z]/.test(kataSandi)) kelas += 1;
  if (/[0-9]/.test(kataSandi)) kelas += 1;
  if (/[^a-zA-Z0-9]/.test(kataSandi)) kelas += 1;
  if (kelas < 2) {
    return 'Kata sandi harus memuat minimal 2 dari 3 jenis karakter: huruf, angka, dan simbol/tanda baca.';
  }
  return null;
}

/**
 * Penolakan eksplisit spasi di awal/akhir untuk form yang MENETAPKAN sandi
 * bagi pengguna lain (buat pengguna & reset paksa): spasi tepi ikut terkirim
 * apa adanya dan lolos kebijakan server, tetapi pengguna hampir pasti tidak
 * akan mengetiknya persis saat masuk. Mengembalikannya sebagai galat yang
 * terlihat jauh lebih jujur daripada memotongnya diam-diam (perilaku lama:
 * validasi atas nilai trim, kirim nilai mentah).
 */
export function validasiSpasiTepiKataSandi(kataSandi: string): string | null {
  if (kataSandi !== kataSandi.trim()) {
    return 'Kata sandi tidak boleh diawali atau diakhiri spasi — spasi tersebut ikut tersimpan apa adanya dan harus diketik persis saat masuk.';
  }
  return null;
}
