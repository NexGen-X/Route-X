import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
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

// B-486: penjaga urutan muat detail — membuka pengguna A lalu B
// dengan cepat tidak boleh membiarkan respons A yang tiba terakhir
// menimpa drawer B (peran yang tampil adalah peran B, karena
// "Simpan Peran" menulis ke detailUserId yang sedang terbuka).
describe('UsersPage — penjaga urutan detail (B-486)', () => {
  const PENGGUNA_B: User = {
    ...PENGGUNA,
    id: 'user-2',
    email: 'dua@routex.internal',
    display_name: 'Pengguna Dua',
  } as User;

  function tunda<T>() {
    let resolve!: (v: T) => void;
    const promise = new Promise<T>((r) => {
      resolve = r;
    });
    return { promise, resolve };
  }

  beforeEach(() => {
    vi.clearAllMocks();
    mockList.mockResolvedValue({ items: [PENGGUNA, PENGGUNA_B] });
  });

  it('respons detail basi tidak menimpa drawer pengguna yang dibuka terakhir', async () => {
    const tundaA = tunda<{ roles: string[]; permissions: string[] }>();
    const tundaB = tunda<{ roles: string[]; permissions: string[] }>();
    mockGet.mockImplementation((id: string) =>
      id === 'user-1' ? tundaA.promise : tundaB.promise
    );

    render(
      <ToastProvider>
        <UsersPage />
      </ToastProvider>
    );

    const kelola = await screen.findAllByRole('button', { name: 'Kelola' });
    fireEvent.click(kelola[0]); // buka A (responsnya tertahan)
    fireEvent.click(kelola[1]); // segera buka B

    // B tiba lebih dulu: drawer B menampilkan peran B (user).
    await act(async () => {
      tundaB.resolve({ roles: ['User'], permissions: [] });
    });
    const pemilih = (await screen.findByLabelText('Peran pengguna')) as HTMLSelectElement;
    expect(pemilih.value).toBe('user');

    // Respons A (Admin) tiba TERAKHIR — wajib diabaikan.
    await act(async () => {
      tundaA.resolve({ roles: ['Admin'], permissions: ['*'] });
    });
    expect(pemilih.value).toBe('user');
  });
});
