import { describe, it, expect } from 'vitest';
import { validasiDomain } from './domain';

// Audit3 V-FF-19: regex lama menolak semua label ber-"--" termasuk
// punycode yang sah. Spec ini mengunci aturan per label yang benar.
describe('validasiDomain', () => {
  it('menerima domain biasa', () => {
    expect(validasiDomain('api.contoh.com')).toBeNull();
    expect(validasiDomain('contoh.co.id')).toBeNull();
    expect(validasiDomain('  API.Contoh.COM ')).toBeNull();
  });

  it('menerima domain punycode (label xn--)', () => {
    expect(validasiDomain('xn--mnchen-3ya.de')).toBeNull();
    expect(validasiDomain('api.xn--mnchen-3ya.de')).toBeNull();
    expect(validasiDomain('xn--nxasmq6b.example.com')).toBeNull();
  });

  it('menolak label diawali/diakhiri tanda hubung', () => {
    expect(validasiDomain('-api.contoh.com')).toContain('tidak boleh diawali atau diakhiri');
    expect(validasiDomain('api-.contoh.com')).toContain('tidak boleh diawali atau diakhiri');
  });

  it('menolak bentuk cacat lain dengan pesan jelas', () => {
    expect(validasiDomain('')).toContain('wajib diisi');
    expect(validasiDomain('tanpatitik')).toContain('minimal satu titik');
    expect(validasiDomain('api..contoh.com')).toContain('label kosong');
    expect(validasiDomain('api.contoh.com.')).toContain('label kosong');
    expect(validasiDomain('api_contoh.com')).toContain('karakter tak sah');
    expect(validasiDomain('contoh.c')).toContain('minimal 2 karakter');
    expect(validasiDomain(`${'a'.repeat(64)}.com`)).toContain('63 karakter');
  });
});
