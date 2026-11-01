import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { CacheTab } from './CacheTab';
import { ApiError } from '../../api/client';

// CacheTab (Fase 2 R3): kontrol cache respons tinggal di Settings.
// Uji ini mengunci perilaku jujurnya: tidak ada tombol aktifkan yang
// pasti gagal, TTL tersimpan dengan enabled apa adanya, dan penolakan
// cache_policy_unsafe tampil sebagai penjelasan — bukan toast gagal.

const mockStatsFn = vi.fn();
const mockUpdateFn = vi.fn();
const mockFlushFn = vi.fn();
const mockToastSuccess = vi.fn();
const mockToastError = vi.fn();
const mockToastInfo = vi.fn();

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client');
  return {
    ...actual,
    api: {
      system: {
        cacheStats: () => mockStatsFn(),
        updateCacheSettings: (enabled: boolean, ttl: number) => mockUpdateFn(enabled, ttl),
        flushCache: () => mockFlushFn(),
      },
    },
  };
});

vi.mock('../../context/ToastContext', () => ({
  useToast: () => ({
    toast: {
      success: mockToastSuccess,
      error: mockToastError,
      info: mockToastInfo,
      warn: vi.fn(),
    },
    confirmModal: vi.fn(async () => true),
  }),
}));

const STATS_NONAKTIF = {
  enabled: false,
  ttl_seconds: 3600,
  hits: 12,
  misses: 8,
  total_entries: 5,
};

describe('CacheTab — Performa & Cache (Settings)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStatsFn.mockResolvedValue(STATS_NONAKTIF);
    mockUpdateFn.mockResolvedValue(undefined);
    mockFlushFn.mockResolvedValue({ deleted: 5, message: 'Berhasil menghapus 5 entri' });
  });

  it('menampilkan statistik dan penjelasan kebijakan tanpa tombol aktifkan', async () => {
    render(<CacheTab />);

    await waitFor(() => {
      expect(screen.getByText('Cache Respons Redis')).toBeInTheDocument();
    });
    expect(screen.getByText('NONAKTIF')).toBeInTheDocument();
    expect(screen.getByText(/belum dapat diaktifkan dari panel ini/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /aktifkan cache/i })).not.toBeInTheDocument();
    // Rasio hit: 12/(12+8) = 60.0%
    expect(screen.getByText('60.0%')).toBeInTheDocument();
  });

  it('menyimpan TTL dengan enabled apa adanya (nonaktif) dalam detik', async () => {
    render(<CacheTab />);

    const input = await screen.findByLabelText(/durasi simpan cache dalam menit/i);
    fireEvent.change(input, { target: { value: '30' } });
    fireEvent.click(screen.getByRole('button', { name: 'Simpan TTL' }));

    await waitFor(() => {
      expect(mockUpdateFn).toHaveBeenCalledWith(false, 1800);
    });
    expect(mockToastSuccess).toHaveBeenCalled();
  });

  it('menampilkan penolakan cache_policy_unsafe sebagai penjelasan menetap', async () => {
    mockStatsFn.mockResolvedValue({ ...STATS_NONAKTIF, enabled: true });
    mockUpdateFn.mockRejectedValue(
      new ApiError(
        400,
        'cache_policy_unsafe',
        'Response cache dinonaktifkan sampai fingerprint tenant, routing, dan policy diterapkan',
      ),
    );

    render(<CacheTab />);

    fireEvent.click(await screen.findByRole('button', { name: 'Simpan TTL' }));

    await waitFor(() => {
      expect(screen.getByText(/Server menolak perubahan ini/i)).toBeInTheDocument();
    });
    expect(mockToastError).not.toHaveBeenCalled();
  });
});
