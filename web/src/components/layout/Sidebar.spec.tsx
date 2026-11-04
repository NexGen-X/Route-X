import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { Sidebar } from './Sidebar';

const mockLogout = vi.fn();
const mockUseAuth = vi.fn();

vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockUseAuth(),
}));

// B-142: Sidebar memuat jumlah tool CLI lewat efek (muatJumlahToolCli →
// api.cli.detected → fetch nyata). Tanpa mock ini, fetch relatif itu
// berdasar URL happy-dom (http://localhost:3000) dan benar-benar mencoba
// membuka koneksi jaringan — kegagalannya dicetak happy-dom sendiri ke
// stderr sebagai blok `AggregateError: ECONNREFUSED ::1:3000` berlabel
// "unknown test" sesudah berkas selesai. Nilai null adalah jalur sah
// "gagal muat → badge disembunyikan" dari lib/cliCount (lihat
// cliCount.spec.ts untuk perilaku muatnya sendiri). Karena mock ini
// menyelesaikan promise-nya, tiap test mengosongkan antrean promise di
// dalam act(async) sesudah render (pola B-138) agar penyelesaian efek
// badge itu tidak mendarat sebagai pembaruan state di luar act.
vi.mock('../../lib/cliCount', () => ({
  muatJumlahToolCli: () => Promise.resolve(null),
}));

describe('Sidebar Component — Obsidian Glass SOTA 2026', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseAuth.mockReturnValue({
      user: { id: 'usr-admin', display_name: 'Architect Bob', email: 'bob@routex.io' },
      principal: { roles: ['SuperAdmin'] },
      logout: mockLogout,
    });
  });

  it('merender branding Route-X, grup menu navigasi, dan user info', async () => {
    const handleNavigate = vi.fn();
    const handleSetMobile = vi.fn();

    render(
      <Sidebar
        currentPath="/"
        onNavigate={handleNavigate}
        isMobileOpen={false}
        setIsMobileOpen={handleSetMobile}
      />
    );
    await act(async () => {});

    expect(screen.getByRole('button', { name: /route-x beranda/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Penyedia' })).toBeInTheDocument();
    expect(screen.getByText('Architect Bob')).toBeInTheDocument();
    expect(screen.getByText('SuperAdmin')).toBeInTheDocument();
  });

  it('menavigasi saat salah satu item menu diklik', async () => {
    const handleNavigate = vi.fn();
    const handleSetMobile = vi.fn();

    render(
      <Sidebar
        currentPath="/"
        onNavigate={handleNavigate}
        isMobileOpen={true}
        setIsMobileOpen={handleSetMobile}
      />
    );
    await act(async () => {});

    const providersBtn = screen.getByRole('button', { name: 'Penyedia' });
    fireEvent.click(providersBtn);

    expect(handleNavigate).toHaveBeenCalledWith('/upstreams/providers');
    expect(handleSetMobile).toHaveBeenCalledWith(false);
  });

  it('menandai item aktif dengan aria-current="page"', async () => {
    render(
      <Sidebar
        currentPath="/gateway/routing"
        onNavigate={vi.fn()}
        isMobileOpen={false}
        setIsMobileOpen={vi.fn()}
      />
    );
    await act(async () => {});

    const routingBtn = screen.getByRole('button', { name: 'Perutean & Failover' });
    expect(routingBtn).toHaveAttribute('aria-current', 'page');
  });

  it('memanggil fungsi logout saat tombol keluar ditekan', async () => {
    render(
      <Sidebar
        currentPath="/"
        onNavigate={vi.fn()}
        isMobileOpen={false}
        setIsMobileOpen={vi.fn()}
      />
    );
    await act(async () => {});

    const logoutBtn = screen.getByRole('button', { name: /keluar dari akun/i });
    fireEvent.click(logoutBtn);
    expect(mockLogout).toHaveBeenCalledTimes(1);
  });

  it('dapat menciutkan dan memperluas sidebar', async () => {
    render(
      <Sidebar
        currentPath="/"
        onNavigate={vi.fn()}
        isMobileOpen={false}
        setIsMobileOpen={vi.fn()}
      />
    );
    await act(async () => {});

    const collapseBtn = screen.getByLabelText(/ciutkan sidebar/i);
    fireEvent.click(collapseBtn);

    // Sekarang tombol berubah jadi perluas sidebar
    expect(screen.getByLabelText(/perluas sidebar/i)).toBeInTheDocument();
  });
});
