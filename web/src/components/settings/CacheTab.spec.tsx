import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { CacheTab } from './CacheTab';
import { ApiError } from '../../api/client';

// CacheTab (Fase 2 R3): kontrol cache respons tinggal di Settings.
// Uji ini mengunci perilaku sesudah Batch C1: aktivasi DITERIMA server
// (kebijakan sidik kunci sudah diterapkan), jadi tombol Aktifkan ada dan
// berfungsi; TTL tersimpan dengan enabled apa adanya; menonaktifkan
// memanggil update dengan enabled=false.

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

  it('menampilkan statistik, banner batas tahap awal, dan tombol aktifkan saat nonaktif', async () => {
    render(<CacheTab />);

    await waitFor(() => {
      expect(screen.getByText('Cache Respons Redis')).toBeInTheDocument();
    });
    expect(screen.getByText('NONAKTIF')).toBeInTheDocument();
    expect(screen.getByText(/Cache nonaktif — aktifkan/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /aktifkan cache/i })).toBeInTheDocument();
    // Rasio hit: 12/(12+8) = 60.0%
    expect(screen.getByText('60.0%')).toBeInTheDocument();
  });

  it('muatan statistik GAGAL → badge dan angka netral, bukan NONAKTIF/0.0% (B-405)', async () => {
    mockStatsFn.mockRejectedValue(new Error('jaringan putus'));
    render(<CacheTab />);

    expect(await screen.findByText('BELUM TERUKUR')).toBeInTheDocument();
    expect(screen.queryByText('NONAKTIF')).not.toBeInTheDocument();
    expect(screen.queryByText('0.0%')).not.toBeInTheDocument();
    expect(screen.getByText(/Statistik cache belum dapat dimuat/)).toBeInTheDocument();
    expect(screen.getByText(/Gagal memuat statistik cache/)).toBeInTheDocument();
  });

  it('mengaktifkan cache dengan TTL dari kolom (menit -> detik)', async () => {
    render(<CacheTab />);

    fireEvent.click(await screen.findByRole('button', { name: /aktifkan cache/i }));

    await waitFor(() => {
      expect(mockUpdateFn).toHaveBeenCalledWith(true, 3600);
    });
    expect(mockToastSuccess).toHaveBeenCalled();
  });

  it('menampilkan tombol nonaktifkan dan banner aktif saat cache menyala', async () => {
    mockStatsFn.mockResolvedValue({ ...STATS_NONAKTIF, enabled: true });
    render(<CacheTab />);

    fireEvent.click(await screen.findByRole('button', { name: /nonaktifkan cache/i }));

    await waitFor(() => {
      expect(mockUpdateFn).toHaveBeenCalledWith(false, 3600);
    });
    expect(screen.getByText(/Cache aktif — hanya respons sukses non-streaming/i)).toBeInTheDocument();
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

  // Audit3 V-FF-18: TTL di luar rentang dulu dijepit diam-diam saat simpan
  // (ketik 2000 -> tersimpan 1440). Kini ditolak dengan pesan dan API tidak
  // dipanggil sama sekali.
  it('menolak TTL di atas 1440 menit tanpa memanggil API', async () => {
    render(<CacheTab />);

    const input = await screen.findByLabelText(/durasi simpan cache dalam menit/i);
    fireEvent.change(input, { target: { value: '2000' } });
    fireEvent.click(screen.getByRole('button', { name: 'Simpan TTL' }));

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/antara 1 dan 1440 menit/);
    });
    expect(mockUpdateFn).not.toHaveBeenCalled();
    expect(mockToastError).toHaveBeenCalled();
  });

  it('kegagalan simpan tampil sebagai toast, bukan penjelasan kebijakan menetap', async () => {
    mockUpdateFn.mockRejectedValue(new ApiError(500, 'internal_error', 'server bermasalah'));

    render(<CacheTab />);

    fireEvent.click(await screen.findByRole('button', { name: 'Simpan TTL' }));

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalled();
    });
    expect(screen.queryByText(/Server menolak perubahan ini/i)).not.toBeInTheDocument();
  });
});
