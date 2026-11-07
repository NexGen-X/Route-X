import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { UsersPage } from './Users';
import { ToastProvider } from '../context/ToastContext';
import type { User } from '../types';

// B-450: drawer detail pengguna — kegagalan users.get wajib menjadi
// keadaan galat + coba lagi di drawer, bukan "Memuat detail..."
// permanen yang hanya disertai toast sesaat.

const PENGGUNA: User = {
  id: 'user-1',
  email: 'satu@routex.internal',
  display_name: 'Pengguna Satu',
  status: 'active',
  must_change_password: false,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
} as User;

const mockList = vi.fn(async () => ({ items: [PENGGUNA] as User[] }));
const mockGet = vi.fn();

vi.mock('../api/client', () => ({
  api: {
    users: {
      list: () => mockList(),
      get: (id: string) => mockGet(id),
      create: async () => ({}),
      update: async () => ({}),
      delete: async () => ({}),
      forceResetPassword: async () => ({}),
      revokeSessions: async () => ({ status: 'revoked', count: 0 }),
      unlock: async () => ({}),
    },
  },
}));

describe('UsersPage — drawer detail gagal dimuat (B-450)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockList.mockResolvedValue({ items: [PENGGUNA] });
    mockGet.mockRejectedValue(new Error('server menolak detail'));
  });

  it('users.get gagal → drawer menampilkan galat + Coba Lagi, bukan memuat permanen', async () => {
    render(
      <ToastProvider>
        <UsersPage />
      </ToastProvider>
    );

    const kelola = await screen.findAllByRole('button', { name: 'Kelola' });
    fireEvent.click(kelola[0]);

    expect(
      await screen.findByText(/Detail pengguna belum dapat dimuat: server menolak detail/)
    ).toBeInTheDocument();
    expect(screen.queryByText('Memuat detail...')).toBeNull();
    expect(screen.getByRole('button', { name: 'Coba Lagi' })).toBeInTheDocument();
  });
});
