import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { TrafficChart } from './TrafficChart';

// B-376: seri kosong selama muatan pertama belum berarti "belum
// ada sampel" — grafik menampilkan keadaan memuat.
describe('TrafficChart (B-376 gerbang muat)', () => {
  const props = {
    series: [],
    metricType: 'requests' as const,
    setMetricType: vi.fn(),
    timeWindow: '24h' as const,
    setTimeWindow: vi.fn(),
  };

  it('memuat: teks memuat, bukan klaim belum ada sampel', () => {
    render(<TrafficChart {...props} memuat />);
    expect(screen.getByText('Memuat sampel metrik…')).toBeInTheDocument();
    expect(screen.queryByText(/Belum ada sampel metrik/)).toBeNull();
  });

  it('tidak memuat: klaim belum ada sampel tetap tampil', () => {
    render(<TrafficChart {...props} />);
    expect(screen.getByText(/Belum ada sampel metrik untuk rentang waktu 24h/)).toBeInTheDocument();
  });
});
