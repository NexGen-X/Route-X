// Validasi nama domain untuk form Domain Publik (audit3 V-FF-19).
//
// Regex lama menolak SEMUA label yang mengandung "--" di mana pun
// (/^(?!-)(?!.*--)([a-z0-9-]{1,63}\.)+[a-z]{2,}$/), sehingga domain
// internationalized yang sah dalam bentuk punycode — mis.
// xn--mnchen-3ya.de — selalu ditolak klien. Aturan yang benar bekerja per
// label: label boleh memuat tanda hubung di tengah (termasuk awalan
// punycode "xn--"), tetapi tidak boleh diawali/diakhiri tanda hubung,
// panjangnya 1–63, dan hanya huruf kecil/angka/tanda hubung.

/**
 * Mengembalikan pesan galat Bahasa Indonesia bila domain tidak sah, atau
 * null bila sah. Nilai dinilai sesudah trim + huruf kecil, sama seperti
 * yang dikirim ke server.
 */
export function validasiDomain(domainMentah: string): string | null {
  const domain = domainMentah.trim().toLowerCase();
  if (!domain) {
    return 'Domain wajib diisi.';
  }
  if (domain.length > 253) {
    return 'Domain terlalu panjang (maksimal 253 karakter).';
  }
  const labels = domain.split('.');
  if (labels.length < 2) {
    return 'Domain harus memuat minimal satu titik, mis. api.contoh.com.';
  }
  for (const label of labels) {
    if (label.length === 0) {
      return 'Domain mengandung label kosong (titik ganda atau titik di awal/akhir).';
    }
    if (label.length > 63) {
      return `Label "${label.slice(0, 24)}…" melebihi batas 63 karakter.`;
    }
    if (!/^[a-z0-9-]+$/.test(label)) {
      return `Label "${label}" mengandung karakter tak sah — hanya huruf kecil, angka, dan tanda hubung.`;
    }
    if (label.startsWith('-') || label.endsWith('-')) {
      return `Label "${label}" tidak boleh diawali atau diakhiri tanda hubung.`;
    }
  }
  const tld = labels[labels.length - 1];
  if (tld.length < 2) {
    return 'Ekstensi domain (label terakhir) minimal 2 karakter.';
  }
  return null;
}
