import { describe, it, expect } from 'vitest';
import { slugNamaProvider, validasiProviderBaru } from './providerMatching';
import type { Provider } from '../../types';

// Audit3 V-FF-09: slug dulu diterapkan senyap saat submit; validasi nama &
// base_url tidak ada sama sekali di modal buat provider.
describe('slugNamaProvider', () => {
  it('menormalisasi sama seperti jalur simpan', () => {
    expect(slugNamaProvider('OpenAI Saya!')).toBe('openai-saya-');
    expect(slugNamaProvider('  DeepSeek  ')).toBe('deepseek');
    expect(slugNamaProvider('my_provider-2')).toBe('my_provider-2');
  });
});

describe('validasiProviderBaru', () => {
  const ada = [{ name: 'openai', display_name: 'OpenAI' } as Provider];

  it('menerima form yang sah', () => {
    expect(
      validasiProviderBaru(
        { name: 'deepseek', display_name: 'DeepSeek', base_url: 'https://api.deepseek.com/v1' },
        ada
      )
    ).toBeNull();
  });

  it('menolak nama dan nama tampilan kosong', () => {
    expect(
      validasiProviderBaru({ name: '  ', display_name: 'X', base_url: 'https://a.b/v1' }, ada)
    ).toContain('Nama teknis');
    expect(
      validasiProviderBaru({ name: 'x', display_name: '', base_url: 'https://a.b/v1' }, ada)
    ).toContain('Nama tampilan');
  });

  it('menolak base_url kosong, bukan URL, dan non-http(s)', () => {
    expect(
      validasiProviderBaru({ name: 'x', display_name: 'X', base_url: '' }, ada)
    ).toContain('wajib diisi');
    expect(
      validasiProviderBaru({ name: 'x', display_name: 'X', base_url: 'bukan url' }, ada)
    ).toContain('bukan URL yang valid');
    expect(
      validasiProviderBaru({ name: 'x', display_name: 'X', base_url: 'ftp://a.b/v1' }, ada)
    ).toContain('http://');
  });

  it('menolak slug yang menabrak provider yang sudah ada', () => {
    expect(
      validasiProviderBaru({ name: 'OpenAI', display_name: 'Lain', base_url: 'https://a.b/v1' }, ada)
    ).toContain('sudah dipakai');
  });

  it('menolak nama yang slug-nya kosong/tanda hubung saja', () => {
    expect(
      validasiProviderBaru({ name: '!!!', display_name: 'X', base_url: 'https://a.b/v1' }, ada)
    ).toContain('tidak menghasilkan nama yang sah');
  });
});
