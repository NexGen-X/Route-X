import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { DashboardHero } from './DashboardHero';
import type { SystemOverview } from '../../types';

// Spec B-285: DashboardHero tidak boleh mengklaim "Beroperasi"
// dengan titik emerald — apalagi "0/0 upstream sehat" — selama
// muatan pertama, sebelum satu pun data tiba. Klaim sehat hanya
// sah sesudah data benar-benar dimuat.

const propsDasar = {
  loadError: null,
  healthyProvidersCount: 0,
  totalProvidersCount: 0,
  overview: null,
  isRefreshing: false,
  onRefresh: vi.fn(),
  onNavigate: vi.fn(),
};

describe('DashboardHero — kejujuran status selama muatan pertama (B-285)', () => {
  it('isLoading → "Memuat Status…", BUKAN "Beroperasi", dan telemetri belum terukur', () => {
    render(<DashboardHero {...propsDasar} isLoading={true} />);

    expect(screen.getByText('Memuat Status…')).toBeInTheDocument();
    expect(screen.queryByText('Beroperasi')).not.toBeInTheDocument();
    expect(screen.queryByText('0/0')).not.toBeInTheDocument();
  });

  it('muatan selesai → "Beroperasi" + hitungan upstream nyata tampil', () => {
    render(
      <DashboardHero
        {...propsDasar}
        isLoading={false}
        healthyProvidersCount={2}
        totalProvidersCount={3}
        overview={{ in_flight_requests: 4 } as SystemOverview}
      />,
    );

    expect(screen.getByText('Beroperasi')).toBeInTheDocument();
    expect(screen.getByText('2/3')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
  });

  it('loadError → "Gateway Tidak Tersedia" (mengalahkan cabang muat)', () => {
    render(<DashboardHero {...propsDasar} isLoading={false} loadError="jaringan putus" />);

    expect(screen.getByText('Gateway Tidak Tersedia')).toBeInTheDocument();
    expect(screen.queryByText('Beroperasi')).not.toBeInTheDocument();
  });
});
