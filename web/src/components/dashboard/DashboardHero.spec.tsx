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
  providersTerukur: false,
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
        providersTerukur={true}
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

// Spec B-346: muatan PARSIAL — Dashboard hanya menyetel loadError
// untuk kegagalan ringkasan atau kegagalan total, jadi sumber
// provider/overview yang gagal sendirian mencapai hero dengan
// loadError null dan isLoading false. Hero versi lama mengklaim
// "Beroperasi" + "0/0 upstream sehat" + in-flight 0 dari data yang
// tidak pernah tiba. Kini klaim digerbangi keberadaan datanya:
// hitungan upstream pada providersTerukur, status + in-flight pada
// overview.
describe('DashboardHero — kejujuran muatan parsial (B-346)', () => {
  it('provider belum pernah tiba (overview ada) → hitungan "—", status tetap dari overview', () => {
    render(
      <DashboardHero
        {...propsDasar}
        isLoading={false}
        providersTerukur={false}
        healthyProvidersCount={0}
        totalProvidersCount={0}
        overview={{ in_flight_requests: 7 } as SystemOverview}
      />,
    );

    expect(screen.getByText('Beroperasi')).toBeInTheDocument();
    expect(screen.queryByText('0/0')).not.toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
  });

  it('overview tidak tiba (bukan loadError, bukan loading) → "Status Tidak Diketahui", in-flight "—"', () => {
    render(
      <DashboardHero
        {...propsDasar}
        isLoading={false}
        providersTerukur={true}
        healthyProvidersCount={1}
        totalProvidersCount={2}
        overview={null}
      />,
    );

    expect(screen.getByText('Status Tidak Diketahui')).toBeInTheDocument();
    expect(screen.queryByText('Beroperasi')).not.toBeInTheDocument();
    expect(screen.getByText('1/2')).toBeInTheDocument();
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1);
  });
});
