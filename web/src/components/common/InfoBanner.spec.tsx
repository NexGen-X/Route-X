import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { InfoBanner } from './InfoBanner';

describe('InfoBanner (Fase 3 P5)', () => {
  it('ringkasan selalu tampil; detail terlipat sampai tombol Detail diklik', () => {
    render(
      <InfoBanner variant="warning" summary="Ringkasan singkat.">
        Penjelasan panjang yang terlipat.
      </InfoBanner>,
    );
    expect(screen.getByText('Ringkasan singkat.')).toBeInTheDocument();
    const tombol = screen.getByRole('button', { name: /Detail/ });
    expect(tombol).toHaveAttribute('aria-expanded', 'false');
    // Detail tetap di DOM (tersembunyi) agar teksnya tidak hilang.
    expect(screen.getByText('Penjelasan panjang yang terlipat.')).toBeInTheDocument();
    fireEvent.click(tombol);
    expect(tombol).toHaveAttribute('aria-expanded', 'true');
  });

  it('tanpa detail tidak ada tombol lipat', () => {
    render(<InfoBanner variant="danger" summary="Hanya ringkasan." />);
    expect(screen.getByText('Hanya ringkasan.')).toBeInTheDocument();
    expect(screen.queryByRole('button')).toBeNull();
  });
});
