import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { UsersPage } from './Users';
import { ToastProvider } from '../context/ToastContext';
import type { User } from '../types';

// Spec regresi B-076: tombol "Buka Kunci" di halaman Pengguna hanya tampil
// untuk akun terkunci — status locked ATAU locked_until brute force yang
// masih di masa depan — dan memanggil endpoint unlock sesudah konfirmasi.

const dasar: Omit<User, 'id' | 'email' | 'status'> = {
  display_name: 'Pengguna Uji',
  must_change_password: false,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

const TERKUNCI_STATUS: User = {
  ...dasar,
  id: 'user-terkunci-status',
  email: 'terkunci-status@routex.internal',
  status: 'locked',
};

const TERKUNCI_SEMENTARA: User = {
  ...dasar,
  id: 'user-terkunci-sementara',
  email: 'terkunci-sementara@routex.internal',
  status: 'active',
  failed_login_attempts: 5,
  locked_until: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
};

const AKTIF_BIASA: User = {
  ...dasar,
  id: 'user-aktif',
  email: 'aktif@routex.internal',
  status: 'active',
  locked_until: null,
};

const mockList = vi.fn(async () => ({
  items: [TERKUNCI_STATUS, TERKUNCI_SEMENTARA, AKTIF_BIASA] as User[],
}));
const mockUnlock = vi.fn(async (_id: string) => TERKUNCI_STATUS as User);

vi.mock('../api/client', () => ({
  api: {
    users: {
      list: () => mockList(),
      get: async () => ({ roles: ['Admin'], permissions: ['*'] }),
      create: async () => ({}),
      update: async () => ({}),
      delete: async () => ({}),
      forceResetPassword: async () => ({}),
      revokeSessions: async () => ({ status: 'revoked', count: 0 }),
      unlock: (id: string) => mockUnlock(id),
    },
  },
}));

const renderHalaman = () =>
  render(
    <ToastProvider>
      <UsersPage />
    </ToastProvider>
  );

describe('UsersPage — Buka Kunci (B-076)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('menampilkan tombol Buka Kunci hanya untuk akun terkunci', async () => {
    renderHalaman();

    // Ketiga pengguna termuat (tabel desktop + kartu mobile sama-sama
    // dirender di jsdom, jadi tiap akun terkunci punya 2 tombol).
    await screen.findAllByText('terkunci-status@routex.internal');
    const tombol = screen.getAllByRole('button', { name: /Buka Kunci/ });
    expect(tombol.length).toBe(4);

    // Akun aktif biasa tidak punya tombol Buka Kunci di barisnya: jumlah
    // tombol persis 2 akun terkunci × 2 tampilan. Catatan kunci sementara
    // (akun active ber-locked_until) juga tampil di kedua tampilan.
    expect(screen.getAllByText('Terkunci sementara (gagal login)').length).toBe(2);
  });

  it('mengklik Buka Kunci meminta konfirmasi lalu memanggil endpoint unlock', async () => {
    renderHalaman();

    const tombol = await screen.findAllByRole('button', { name: /Buka Kunci/ });
    fireEvent.click(tombol[0]);

    const konfirmasi = await screen.findByRole('button', { name: 'Ya, Buka Kunci' });
    fireEvent.click(konfirmasi);

    await waitFor(() => {
      expect(mockUnlock).toHaveBeenCalledWith('user-terkunci-status');
    });
  });
});
