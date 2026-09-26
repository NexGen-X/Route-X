import { describe, it, expect } from 'vitest';
import { formatUSD, percentageOfDecimal } from './money';

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

      // fractionDigits = 0
      expect(formatUSD(val, 0)).toBe('$1,234');

      // fractionDigits = 2
      expect(formatUSD(val, 2)).toBe('$1,234.56');

      // fractionDigits = 4
      expect(formatUSD(val, 4)).toBe('$1,234.5678');

      // fractionDigits = 6
      expect(formatUSD(val, 6)).toBe('$1,234.56789');

      // fractionDigits = 8
      expect(formatUSD(val, 8)).toBe('$1,234.56789012');
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
});
