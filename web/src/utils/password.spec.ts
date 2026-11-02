import { describe, it, expect } from 'vitest';
import { validasiPassword, validasiSpasiTepiKataSandi } from './password';

// Spec mencerminkan kebijakan server internal/security/password.go persis
// (audit3 V-FF-12/V-FF-13): min 12 rune, maks 1024 byte, >= 2 dari 3 kelas,
// dan daftar kata sandi umum case-insensitive.
describe('validasiPassword (cermin kebijakan server)', () => {
  it('menolak kurang dari 12 karakter', () => {
    expect(validasiPassword('Abc123!xyz')).toBe('Kata sandi minimal 12 karakter.');
    expect(validasiPassword('')).toBe('Kata sandi minimal 12 karakter.');
  });

  it('menghitung karakter sebagai rune, bukan byte', () => {
    // 11 rune multibyte tetap di bawah batas walau byte-nya > 12.
    expect(validasiPassword('éééééééééé1')).toBe('Kata sandi minimal 12 karakter.');
    // 12 rune campuran huruf non-ASCII + digit = 2 kelas -> sah.
    expect(validasiPassword('ééééééééééé1')).toBeNull();
  });

  it('menolak satu kelas karakter saja', () => {
    expect(validasiPassword('abcdefghijkl')).toContain('minimal 2 dari 3');
    expect(validasiPassword('12345678901!')).toBeNull(); // digit + simbol = 2 kelas
    expect(validasiPassword('abcdefghijk1')).toBeNull(); // huruf + digit = 2 kelas
    expect(validasiPassword('abcdefghijk!')).toBeNull(); // huruf + simbol = 2 kelas
  });

  it('menolak kata sandi umum tanpa peduli kapitalisasi', () => {
    expect(validasiPassword('Password1234')).toContain('kata sandi umum');
    expect(validasiPassword('Password12345')).toContain('kata sandi umum');
    expect(validasiPassword('WELCOME12345')).toContain('kata sandi umum');
  });

  it('menolak lebih dari 1024 byte', () => {
    const panjang = 'a1'.repeat(600); // 1200 karakter ASCII = 1200 byte
    expect(validasiPassword(panjang)).toContain('1024 byte');
  });

  it('menerima kata sandi yang memenuhi kebijakan', () => {
    expect(validasiPassword('KucingTerbang42!')).toBeNull();
    expect(validasiPassword('rahasia-super-aman')).toBeNull();
  });
});

describe('validasiSpasiTepiKataSandi', () => {
  it('menolak spasi di awal atau akhir dengan pesan eksplisit', () => {
    expect(validasiSpasiTepiKataSandi(' KataSandi123')).toContain('tidak boleh diawali atau diakhiri spasi');
    expect(validasiSpasiTepiKataSandi('KataSandi123 ')).toContain('tidak boleh diawali atau diakhiri spasi');
  });

  it('menerima spasi di tengah dan tanpa spasi tepi', () => {
    expect(validasiSpasiTepiKataSandi('kata sandi frasa 42')).toBeNull();
    expect(validasiSpasiTepiKataSandi('KataSandi123')).toBeNull();
  });
});
