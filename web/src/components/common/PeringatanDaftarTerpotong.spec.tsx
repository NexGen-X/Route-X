import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { PeringatanDaftarTerpotong } from './PeringatanDaftarTerpotong';

describe('PeringatanDaftarTerpotong (sisa audit3 V-FF-28)', () => {
  it('menyebut sumber daftar dan menyatakan daftar mungkin tidak lengkap', () => {
    render(<PeringatanDaftarTerpotong sumber="provider" />);
    expect(screen.getByText(/Daftar provider mungkin tidak lengkap/)).toBeInTheDocument();
  });

  it('mendukung frasa sumber majemuk', () => {
    render(<PeringatanDaftarTerpotong sumber="aturan routing, model" />);
    expect(screen.getByText(/Daftar aturan routing, model mungkin tidak lengkap/)).toBeInTheDocument();
  });
});
