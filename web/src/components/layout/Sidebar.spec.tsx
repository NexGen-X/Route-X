import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Sidebar } from './Sidebar';

const mockLogout = vi.fn();
const mockUseAuth = vi.fn();

vi.mock('../../context/AuthContext', () => ({
  useAuth: () => mockUseAuth(),
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

  it('merender branding Route-X, grup menu navigasi, dan user info', () => {
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

    expect(screen.getByRole('button', { name: /route-x beranda/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Providers' })).toBeInTheDocument();
    expect(screen.getByText('Architect Bob')).toBeInTheDocument();
    expect(screen.getByText('SuperAdmin')).toBeInTheDocument();
  });

  it('menavigasi saat salah satu item menu diklik', () => {
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

    const providersBtn = screen.getByRole('button', { name: 'Providers' });
    fireEvent.click(providersBtn);

    expect(handleNavigate).toHaveBeenCalledWith('/upstreams/providers');
    expect(handleSetMobile).toHaveBeenCalledWith(false);
  });

  it('menandai item aktif dengan aria-current="page"', () => {
    render(
      <Sidebar
        currentPath="/gateway/routing"
        onNavigate={vi.fn()}
        isMobileOpen={false}
        setIsMobileOpen={vi.fn()}
      />
    );

    const routingBtn = screen.getByRole('button', { name: 'Routing & Failover' });
    expect(routingBtn).toHaveAttribute('aria-current', 'page');
  });

  it('memanggil fungsi logout saat tombol keluar ditekan', () => {
    render(
      <Sidebar
        currentPath="/"
        onNavigate={vi.fn()}
        isMobileOpen={false}
        setIsMobileOpen={vi.fn()}
      />
    );

    const logoutBtn = screen.getByRole('button', { name: /keluar dari akun/i });
    fireEvent.click(logoutBtn);
    expect(mockLogout).toHaveBeenCalledTimes(1);
  });

  it('dapat menciutkan dan memperluas sidebar', () => {
    render(
      <Sidebar
        currentPath="/"
        onNavigate={vi.fn()}
        isMobileOpen={false}
        setIsMobileOpen={vi.fn()}
      />
    );

    const collapseBtn = screen.getByLabelText(/ciutkan sidebar/i);
    fireEvent.click(collapseBtn);

    // Sekarang tombol berubah jadi perluas sidebar
    expect(screen.getByLabelText(/perluas sidebar/i)).toBeInTheDocument();
  });
});
