import { describe, expect, it } from 'vitest';
import { keadaanMuat, teksHitungKeadaan } from './keadaanMuat';

// Pengunci gerbang keadaan muat bersama (Fase 18): urutan
// keputusannya adalah kontraknya — data yang ada menang atas
// galat (data lama saat refresh gagal tetap data), dan 'kosong'
// hanya lahir dari muatan yang BERHASIL.
describe('keadaanMuat', () => {
  it('data ada menang atas memuat dan gagal', () => {
    expect(keadaanMuat({ memuat: true, gagal: true, jumlah: 3 })).toBe('ada');
    expect(keadaanMuat({ memuat: false, gagal: true, jumlah: 1 })).toBe('ada');
  });

  it('memuat tanpa data adalah memuat, bukan kosong', () => {
    expect(keadaanMuat({ memuat: true, gagal: false, jumlah: 0 })).toBe('memuat');
  });

  it('gagal tanpa data adalah gagal, BUKAN kosong', () => {
    expect(keadaanMuat({ memuat: false, gagal: true, jumlah: 0 })).toBe('gagal');
  });

  it('berhasil tanpa data adalah satu-satunya kosong', () => {
    expect(keadaanMuat({ memuat: false, gagal: false, jumlah: 0 })).toBe('kosong');
  });

  it('memuat menang atas gagal saat keduanya terpasang tanpa data', () => {
    expect(keadaanMuat({ memuat: true, gagal: true, jumlah: 0 })).toBe('memuat');
  });
});

describe('teksHitungKeadaan', () => {
  it('angka hanya pada keadaan terukur (ada/kosong)', () => {
    expect(teksHitungKeadaan('ada', 3)).toBe('3');
    expect(teksHitungKeadaan('kosong', 0)).toBe('0');
  });

  it('belum terukur (memuat/gagal) adalah penanda netral, bukan 0', () => {
    expect(teksHitungKeadaan('memuat', 0)).toBe('—');
    expect(teksHitungKeadaan('gagal', 0)).toBe('—');
  });
});
