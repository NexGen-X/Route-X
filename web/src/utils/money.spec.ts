import { describe, it, expect } from 'vitest';
import { formatUSD, percentageOfDecimal, parseJumlahUsd } from './money';

describe('money utility', () => {
  describe('formatUSD', () => {
    it('memformat input angka positif dengan benar', () => {
      expect(formatUSD(100)).toBe('$100');
      expect(formatUSD(1234.56, 2)).toBe('$1,234.56');
      expect(formatUSD(1000000)).toBe('$1,000,000');
      expect(formatUSD(0.005, 3)).toBe('$0.005');
    });

    it('memformat input string desimal dengan benar', () => {
      expect(formatUSD('25.00')).toBe('$25');
      expect(formatUSD('1234.5678', 4)).toBe('$1,234.5678');
      expect(formatUSD('100.50', 2)).toBe('$100.5');
      expect(formatUSD('0.0000700', 6)).toBe('$0.00007');
    });

    it('menangani input null, undefined, dan nilai nol', () => {
      expect(formatUSD(null)).toBe('$0');
      expect(formatUSD(undefined)).toBe('$0');
      expect(formatUSD(0)).toBe('$0');
      expect(formatUSD('0')).toBe('$0');
      expect(formatUSD(0, 0)).toBe('$0');
      expect(formatUSD(null, 2)).toBe('$0');
    });

    it('menangani input tidak valid atau non-finite', () => {
      expect(formatUSD('bukan-angka', 0)).toBe('$0');
      expect(formatUSD('bukan-angka', 2)).toBe('$0.00');
      expect(formatUSD(NaN, 4)).toBe('$0.0000');
      expect(formatUSD(Infinity, 4)).toBe('$0.0000');
    });

    it('mendukung berbagai variasi fractionDigits (0, 2, 4, 6, 8)', () => {
      const val = '1234.56789012';

      // fractionDigits = 0 — pecahan ,5678… membulatkan bagian bulat ke atas
      expect(formatUSD(val, 0)).toBe('$1,235');

      // fractionDigits = 2 — digit terpotong pertama 7, naik
      expect(formatUSD(val, 2)).toBe('$1,234.57');

      // fractionDigits = 4 — digit terpotong pertama 9, naik
      expect(formatUSD(val, 4)).toBe('$1,234.5679');

      // fractionDigits = 6 — digit terpotong pertama 1, tetap
      expect(formatUSD(val, 6)).toBe('$1,234.56789');

      // fractionDigits = 8
      expect(formatUSD(val, 8)).toBe('$1,234.56789012');
    });

    it('membulatkan half-up, bukan memotong (audit3 V-FJ-08)', () => {
      // Bukti lama: '1.999' pada 2 digit tampil $1.99 (terpotong).
      expect(formatUSD('1.999', 2)).toBe('$2');
      expect(formatUSD('9.9999', 2)).toBe('$10');
      expect(formatUSD('0.123456', 4)).toBe('$0.1235');
      expect(formatUSD('0.00005', 4)).toBe('$0.0001');
      expect(formatUSD('2.5', 0)).toBe('$3');
      // Di bawah setengah tetap turun, dan separuh persis naik (half-up).
      expect(formatUSD('1.0049', 2)).toBe('$1');
      expect(formatUSD('1.005', 2)).toBe('$1.01');
      // Negatif membulatkan magnitudenya; rambatan mencapai ribuan.
      expect(formatUSD('-1.999', 2)).toBe('$-2');
      expect(formatUSD('999.999', 2)).toBe('$1,000');
    });

    it('memotong trailing zeros pada bagian pecahan desimal', () => {
      expect(formatUSD('10.500000', 4)).toBe('$10.5');
      expect(formatUSD('10.000000', 4)).toBe('$10');
      expect(formatUSD('50.00', 2)).toBe('$50');
    });

    it('mempertahankan tanda negatif pada angka desimal', () => {
      expect(formatUSD(-25.5, 2)).toBe('$-25.5');
      expect(formatUSD('-100.25', 2)).toBe('$-100.25');
    });
  });

  describe('percentageOfDecimal', () => {
    it('menghitung persentase pada batas normal', () => {
      expect(percentageOfDecimal('25', '100')).toBe(25);
      expect(percentageOfDecimal('45', '50')).toBe(90);
      expect(percentageOfDecimal('50', '200')).toBe(25);
      expect(percentageOfDecimal('1', '3')).toBe(33); // Pembulatan (33.333% -> 33)
      expect(percentageOfDecimal('2', '3')).toBe(67); // Pembulatan (66.666% -> 67)
    });

    it('membatasi persentase maksimum 100% saat pemakaian melebihi limit', () => {
      expect(percentageOfDecimal('150', '100')).toBe(100);
      expect(percentageOfDecimal('200.50', '50.00')).toBe(100);
      expect(percentageOfDecimal('999', '1')).toBe(100);
    });

    it('mengembalikan 0 saat pemakaian nol atau limit bernilai nol/kosong', () => {
      expect(percentageOfDecimal('0', '100')).toBe(0);
      expect(percentageOfDecimal('', '100')).toBe(0);
      expect(percentageOfDecimal('50', '0')).toBe(0);
      expect(percentageOfDecimal('50', '')).toBe(0);
    });

    it('menangani input desimal berpresisi tinggi', () => {
      expect(percentageOfDecimal('0.00250000', '0.01000000')).toBe(25);
      expect(percentageOfDecimal('0.0075', '0.0100')).toBe(75);
    });

    it('mengembalikan 0 jika format numerik tidak valid atau bernilai negatif', () => {
      expect(percentageOfDecimal('abc', '100')).toBe(0);
      expect(percentageOfDecimal('100', 'xyz')).toBe(0);
      expect(percentageOfDecimal('-10', '100')).toBe(0);
    });
  });

  // Audit3 V-FF-07: Number("10,5") = NaN dulu membuat anggaran hilang
  // diam-diam di balik toast sukses.
  describe('parseJumlahUsd', () => {
    it('menerima titik dan koma sebagai pemisah desimal', () => {
      expect(parseJumlahUsd('10.5')).toEqual({ ok: true, nilai: 10.5 });
      expect(parseJumlahUsd('10,5')).toEqual({ ok: true, nilai: 10.5 });
      expect(parseJumlahUsd(' 25 ')).toEqual({ ok: true, nilai: 25 });
      expect(parseJumlahUsd('0.01')).toEqual({ ok: true, nilai: 0.01 });
    });

    it('menolak masukan tak sah dengan pesan, bukan NaN senyap', () => {
      expect(parseJumlahUsd('').ok).toBe(false);
      expect(parseJumlahUsd('abc').ok).toBe(false);
      expect(parseJumlahUsd('1e3').ok).toBe(false);
      expect(parseJumlahUsd('1.000.000').ok).toBe(false);
      expect(parseJumlahUsd('10,5,2').ok).toBe(false);
      expect(parseJumlahUsd('-5').ok).toBe(false);
    });

    it('menolak nol dan negatif sebagai anggaran', () => {
      expect(parseJumlahUsd('0').ok).toBe(false);
      expect(parseJumlahUsd('0.0').ok).toBe(false);
    });
  });
});
