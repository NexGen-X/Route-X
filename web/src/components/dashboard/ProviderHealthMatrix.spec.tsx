import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProviderHealthMatrix } from './ProviderHealthMatrix';

// B-375: matriks kesehatan provider ikut gerbang kejujuran hero
// (B-346). Daftar kosong yang BELUM pernah terukur — muatan
// pertama yang masih berjalan atau gagal total — tidak boleh
// diklaim definitif "belum ada provider" dan tidak boleh
// menawarkan CTA menambahkan provider.
describe('ProviderHealthMatrix (B-375 gerbang muat)', () => {
  it('belum terukur + sedang memuat: teks memuat netral, tanpa klaim dan CTA', () => {
    render(<ProviderHealthMatrix providers={[]} onNavigate={vi.fn()} memuat />);
    expect(screen.getByText('Memuat status provider…')).toBeInTheDocument();
    expect(screen.queryByText('Belum ada provider upstream yang terdaftar.')).toBeNull();
    expect(screen.queryByText('Tambahkan provider sekarang')).toBeNull();
  });

  it('belum terukur + tidak memuat: status tidak diketahui, tanpa klaim dan CTA', () => {
    render(<ProviderHealthMatrix providers={[]} onNavigate={vi.fn()} />);
    expect(screen.getByText('Status provider tidak diketahui.')).toBeInTheDocument();
    expect(screen.queryByText('Belum ada provider upstream yang terdaftar.')).toBeNull();
    expect(screen.queryByText('Tambahkan provider sekarang')).toBeNull();
  });

  it('sudah terukur kosong: klaim definitif dan CTA tetap tampil', () => {
    render(<ProviderHealthMatrix providers={[]} onNavigate={vi.fn()} providersTerukur />);
    expect(screen.getByText('Belum ada provider upstream yang terdaftar.')).toBeInTheDocument();
    expect(screen.getByText('Tambahkan provider sekarang')).toBeInTheDocument();
  });
});
