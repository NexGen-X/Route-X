import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Header } from './Header';

const mockUseAuth = vi.fn();
vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockUseAuth(),
}));

// Status gateway kini diturunkan dari endpoint overview nyata (B-224);
// mock-nya dikendalikan per test.
const overviewMock = vi.fn();
vi.mock('../../api/client', () => ({
  api: {
    system: {
      overview: () => overviewMock(),
    },
  },
}));

describe('Header Component — Glassmorphic SOTA 2026', () => {
  beforeEach(() => {
    overviewMock.mockReset();
    overviewMock.mockResolvedValue({ pid: 1 });
    mockUseAuth.mockReturnValue({
      principal: {
        user: { id: 'usr-1', email: 'admin@routex.local', display_name: 'SuperAdmin' },
        roles: ['Administrator'],
      },
    });
  });

  it('merender title, subtitle breadcrumb, dan telemetry radar online', async () => {
    const handleOpenMobile = vi.fn();
    render(
      <Header
        title="Aturan Perutean"
        subtitle="Gateway Pipeline"
        onOpenMobileMenu={handleOpenMobile}
      />
    );

    expect(screen.getByRole('heading', { level: 1, name: 'Aturan Perutean' })).toBeInTheDocument();
    expect(screen.getByText('Gateway Pipeline')).toBeInTheDocument();
    expect(screen.getByRole('status', { name: /status telemetri gateway/i })).toBeInTheDocument();
    // "Gateway Online" hanya muncul SESUDAH overview benar-benar
    // menjawab (B-224) — bukan teks statis sejak render pertama.
    expect(await screen.findByText('Gateway Online')).toBeInTheDocument();
  });

  it('overview gagal → status "Gateway Tidak Tersedia", bukan online karangan (B-224)', async () => {
    overviewMock.mockRejectedValue(new Error('api tidak terjangkau'));
    render(<Header title="Dashboard" onOpenMobileMenu={vi.fn()} />);
    expect(await screen.findByText('Gateway Tidak Tersedia')).toBeInTheDocument();
    expect(screen.queryByText('Gateway Online')).not.toBeInTheDocument();
  });

  it('membuka mobile menu saat hamburger ditekan', () => {
    const handleOpenMobile = vi.fn();
    render(
      <Header
        title="Dashboard"
        onOpenMobileMenu={handleOpenMobile}
      />
    );

    const menuBtn = screen.getByLabelText(/buka menu navigasi/i);
    fireEvent.click(menuBtn);
    expect(handleOpenMobile).toHaveBeenCalledTimes(1);
  });

  it('menampilkan tombol command palette dan memicu callback saat ditekan', () => {
    const handlePalette = vi.fn();
    render(
      <Header
        title="Dashboard"
        onOpenMobileMenu={vi.fn()}
        onOpenCommandPalette={handlePalette}
      />
    );

    const paletteBtn = screen.getByLabelText(/buka command palette \(ctrl\+k atau ⌘k\)/i);
    expect(paletteBtn).toBeInTheDocument();
    fireEvent.click(paletteBtn);
    expect(handlePalette).toHaveBeenCalledTimes(1);
  });

  it('merender badge role admin saat principal aktif', () => {
    render(
      <Header
        title="Dashboard"
        onOpenMobileMenu={vi.fn()}
      />
    );

    expect(screen.getByText('Administrator')).toBeInTheDocument();
  });
});
